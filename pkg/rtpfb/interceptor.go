// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package rtpfb implements feedback aggregation for CCFB and TWCC packets.
package rtpfb

import (
	"math"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"

type ccfbAttributesKeyType uint32

// CCFBAttributesKey is the key which can be used to retrieve the Report objects
// from the interceptor.Attributes.
const CCFBAttributesKey ccfbAttributesKeyType = iota

type packetLog interface {
	addOutgoing(
		ssrc uint32,
		rtpSequenceNumber uint16,
		isTWCC bool,
		twccSequenceNumber uint16,
		size int,
		departure time.Time,
	)
	onTWCCFeedback(ts time.Time, ack acknowledgement) (time.Duration, bool)
	onCCFBFeedback(ts time.Time, ssrc uint32, ack acknowledgement) (time.Duration, bool)
	buildReport() []PacketReport
}

// Option can be used to set initial options on CCFB interceptors.
type Option func(*Interceptor) error

func WithLoggerFactory(lf logging.LoggerFactory) Option {
	return func(i *Interceptor) error {
		i.logFactory = lf

		return nil
	}
}

func timeFactory(f func() time.Time) Option {
	return func(i *Interceptor) error {
		i.timestamp = f

		return nil
	}
}

func setHistory(pl packetLog) Option {
	return func(i *Interceptor) error {
		i.history = pl

		return nil
	}
}

// InterceptorFactory is a factory for CCFB interceptors.
type InterceptorFactory struct {
	opts []Option
}

// NewInterceptor returns a new CCFB InterceptorFactory.
func NewInterceptor(opts ...Option) (*InterceptorFactory, error) {
	return &InterceptorFactory{
		opts: opts,
	}, nil
}

// NewInterceptor returns a new ccfb.Interceptor.
func (f *InterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	in := &Interceptor{
		NoOp:       interceptor.NoOp{},
		logFactory: logging.NewDefaultLoggerFactory(),
		log:        nil,
		timestamp:  time.Now,
		history:    newHistory(),
	}
	for _, opt := range f.opts {
		if err := opt(in); err != nil {
			return nil, err
		}
	}
	in.log = in.logFactory.NewLogger("ccfb_interceptor")

	return in, nil
}

// Interceptor implements a congestion control feedback receiver. It keeps track
// of outgoing packets and reads incoming feedback reports (CCFB or TWCC). For
// each incoming feedback report, it will add an entry to the interceptor
// attributes, which can be read from the `RTCPReader`
// (`webrtc.RTPSender.Read`). For each acknowledgement included in the feedback
// report, a PacketReport will be added to the ccfb.Report.
type Interceptor struct {
	interceptor.NoOp
	logFactory logging.LoggerFactory
	log        logging.LeveledLogger
	timestamp  func() time.Time

	history packetLog
}

func (i *Interceptor) bindTWCCStream(twccHdrExtID uint8, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(
		header *rtp.Header,
		payload []byte,
		attributes interceptor.Attributes,
	) (int, error) {
		ts := i.timestamp()

		var twccHdrExt rtp.TransportCCExtension
		if err := twccHdrExt.Unmarshal(header.GetExtension(twccHdrExtID)); err != nil {
			i.log.Warnf(
				"CCFB configured for TWCC, but failed to get TWCC header extension from outgoing packet."+
					"Falling back to saving history for CCFB feedback reports. err: %v",
				err,
			)
			i.history.addOutgoing(header.SSRC, header.SequenceNumber, false, 0, header.MarshalSize()+len(payload), ts)

			return writer.Write(header, payload, attributes)
		}

		i.history.addOutgoing(
			header.SSRC,
			header.SequenceNumber,
			true,
			twccHdrExt.TransportSequence,
			header.MarshalSize()+len(payload),
			ts,
		)

		return writer.Write(header, payload, attributes)
	})
}

func (i *Interceptor) bindCCFBStream(writer interceptor.RTPWriter) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(
		header *rtp.Header,
		payload []byte,
		attributes interceptor.Attributes,
	) (int, error) {
		ts := i.timestamp()

		i.history.addOutgoing(
			header.SSRC,
			header.SequenceNumber,
			false,
			0,
			header.MarshalSize()+len(payload),
			ts,
		)

		return writer.Write(header, payload, attributes)
	})
}

// BindLocalStream implements interceptor.Interceptor.
func (i *Interceptor) BindLocalStream(
	info *interceptor.StreamInfo,
	writer interceptor.RTPWriter,
) interceptor.RTPWriter {
	var twccHdrExtID uint8
	var useTWCC bool
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			twccHdrExtID = uint8(e.ID) // nolint:gosec
			useTWCC = true

			break
		}
	}
	if useTWCC {
		return i.bindTWCCStream(twccHdrExtID, writer)
	}

	return i.bindCCFBStream(writer)
}

// BindRTCPReader implements interceptor.Interceptor.
func (i *Interceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		n, attr, err := reader.Read(b, a)
		if err != nil {
			return n, attr, err
		}
		ts := i.timestamp()

		if attr == nil {
			attr = make(interceptor.Attributes)
		}

		pkts, err := attr.GetRTCPPackets(b[:n])
		if err != nil {
			return n, attr, err
		}
		rtt, prs := i.processFeedback(ts, pkts)

		if len(prs) > 0 {
			report := Report{
				Arrival:       ts,
				RTT:           rtt,
				PacketReports: prs,
			}
			attr.Set(CCFBAttributesKey, report)
		}

		return n, attr, err
	})
}

//nolint:cyclop
func (i *Interceptor) processFeedback(ts time.Time, pkts []rtcp.Packet) (time.Duration, []PacketReport) {
	shortestRTT := time.Duration(math.MaxInt64)
	var ackDelay time.Duration

	for _, pkt := range pkts {
		switch fb := pkt.(type) {
		case *rtcp.CCFeedbackReport:
			var acksPerSSRC map[uint32][]acknowledgement
			ackDelay, acksPerSSRC = convertCCFB(ts, fb)
			for ssrc, acks := range acksPerSSRC {
				for _, ack := range acks {
					rtt, ok := i.history.onCCFBFeedback(ts, ssrc, ack)
					if ok && rtt < shortestRTT {
						shortestRTT = rtt
					}
				}
			}
		case *rtcp.TransportLayerCC:
			for _, ack := range convertTWCC(fb) {
				rtt, ok := i.history.onTWCCFeedback(ts, ack)
				if ok && rtt < shortestRTT {
					shortestRTT = rtt
				}
			}
		}
	}

	return shortestRTT - ackDelay, i.history.buildReport()
}
