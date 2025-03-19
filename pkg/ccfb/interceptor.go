// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package ccfb implements feedback aggregation for CCFB and TWCC packets.
package ccfb

import (
	"sync"
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

// A Report contains Arrival and Departure (from the remote end) times of a RTCP
// feedback packet (CCFB or TWCC) and a list of PacketReport for all
// acknowledged packets that were still in the history.
type Report struct {
	Arrival             time.Time
	Departure           time.Time
	SSRCToPacketReports map[uint32][]PacketReport
}

type history interface {
	add(seqNr uint16, size int, departure time.Time) error
	getReportForAck([]acknowledgement) []PacketReport
}

// Option can be used to set initial options on CCFB interceptors.
type Option func(*Interceptor) error

// HistorySize sets the size of the history of outgoing packets.
func HistorySize(size int) Option {
	return func(i *Interceptor) error {
		i.historySize = size

		return nil
	}
}

func timeFactory(f func() time.Time) Option {
	return func(i *Interceptor) error {
		i.timestamp = f

		return nil
	}
}

func historyFactory(f func(int) history) Option {
	return func(i *Interceptor) error {
		i.historyFactory = f

		return nil
	}
}

// nolint
func ccfbConverterFactory(f func(ts time.Time, feedback *rtcp.CCFeedbackReport) (time.Time, map[uint32][]acknowledgement)) Option {
	return func(i *Interceptor) error {
		i.convertCCFB = f

		return nil
	}
}

func twccConverterFactory(f func(feedback *rtcp.TransportLayerCC) (time.Time, map[uint32][]acknowledgement)) Option {
	return func(i *Interceptor) error {
		i.convertTWCC = f

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
		NoOp:          interceptor.NoOp{},
		lock:          sync.Mutex{},
		log:           logging.NewDefaultLoggerFactory().NewLogger("ccfb_interceptor"),
		timestamp:     time.Now,
		convertCCFB:   convertCCFB,
		convertTWCC:   convertTWCC,
		ssrcToHistory: make(map[uint32]history),
		historySize:   200,
		historyFactory: func(size int) history {
			return newHistoryList(size)
		},
	}
	for _, opt := range f.opts {
		if err := opt(in); err != nil {
			return nil, err
		}
	}

	return in, nil
}

// Interceptor implements a congestion control feedback receiver. It keeps track
// of outgoing packets and reads incoming feedback reports (CCFB or TWCC). For
// each incoming feedback report, it will add an entry to the interceptor
// attributes, which can be read from the `RTCPReader`
// (`webrtc.RTPSender.Read`). For each acknowledgement included in the feedback
// report and for which there still is an entry in the history of outgoing
// packets, a PacketReport will be added to the ccfb.Report map. The map
// contains a list of packets for each outgoing SSRC if CCFB is used. The map
// contains a single entry with SSRC=0 if TWCC is used.
type Interceptor struct {
	interceptor.NoOp
	lock           sync.Mutex
	log            logging.LeveledLogger
	timestamp      func() time.Time
	convertCCFB    func(ts time.Time, feedback *rtcp.CCFeedbackReport) (time.Time, map[uint32][]acknowledgement)
	convertTWCC    func(feedback *rtcp.TransportLayerCC) (time.Time, map[uint32][]acknowledgement)
	ssrcToHistory  map[uint32]history
	historySize    int
	historyFactory func(int) history
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

	i.lock.Lock()
	defer i.lock.Unlock()

	ssrc := info.SSRC
	if useTWCC {
		ssrc = 0
	}
	i.ssrcToHistory[ssrc] = i.historyFactory(i.historySize)

	// nolint
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		i.lock.Lock()
		defer i.lock.Unlock()

		// If we are using TWCC, we use the sequence number from the TWCC header
		// extension and save all TWCC sequence numbers with the same SSRC (0).
		// If we are not using TWCC, we save a history per SSRC and use the
		// normal RTP sequence numbers.
		ssrc := header.SSRC
		seqNr := header.SequenceNumber
		if useTWCC {
			var twccHdrExt rtp.TransportCCExtension
			if err := twccHdrExt.Unmarshal(header.GetExtension(twccHdrExtID)); err != nil {
				i.log.Warnf(
					"CCFB configured for TWCC, but failed to get TWCC header extension from outgoing packet."+
						"Falling back to saving history for CCFB feedback reports. err: %v",
					err,
				)
				if _, ok := i.ssrcToHistory[ssrc]; !ok {
					i.ssrcToHistory[ssrc] = i.historyFactory(i.historySize)
				}
			} else {
				seqNr = twccHdrExt.TransportSequence
				ssrc = 0
			}
		}
		if err := i.ssrcToHistory[ssrc].add(seqNr, header.MarshalSize()+len(payload), i.timestamp()); err != nil {
			return 0, err
		}

		return writer.Write(header, payload, attributes)
	})
}

// BindRTCPReader implements interceptor.Interceptor.
func (i *Interceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		n, attr, err := reader.Read(b, a)
		if err != nil {
			return n, attr, err
		}
		now := i.timestamp()

		buf := make([]byte, n)
		copy(buf, b[:n])

		if attr == nil {
			attr = make(interceptor.Attributes)
		}

		res := []Report{}

		pkts, err := attr.GetRTCPPackets(buf)
		if err != nil {
			return n, attr, err
		}
		for _, pkt := range pkts {
			var reportLists map[uint32][]acknowledgement
			var reportDeparture time.Time
			switch fb := pkt.(type) {
			case *rtcp.CCFeedbackReport:
				reportDeparture, reportLists = i.convertCCFB(now, fb)
			case *rtcp.TransportLayerCC:
				reportDeparture, reportLists = i.convertTWCC(fb)
			default:
			}
			ssrcToPrl := map[uint32][]PacketReport{}
			for ssrc, reportList := range reportLists {
				prl := i.ssrcToHistory[ssrc].getReportForAck(reportList)
				if _, ok := ssrcToPrl[ssrc]; !ok {
					ssrcToPrl[ssrc] = prl
				} else {
					ssrcToPrl[ssrc] = append(ssrcToPrl[ssrc], prl...)
				}
			}
			res = append(res, Report{
				Arrival:             now,
				Departure:           reportDeparture,
				SSRCToPacketReports: ssrcToPrl,
			})
		}
		attr.Set(CCFBAttributesKey, res)

		return n, attr, err
	})
}
