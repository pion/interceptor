package ccfb

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const transportCCURI = "http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01"

type ccfbAttributesKeyType uint32

const CCFBAttributesKey ccfbAttributesKeyType = iota

type Option func(*Interceptor) error

type InterceptorFactory struct {
	opts []Option
}

func NewInterceptor(opts ...Option) (*InterceptorFactory, error) {
	return &InterceptorFactory{
		opts: opts,
	}, nil
}

func (f *InterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	i := &Interceptor{
		NoOp:          interceptor.NoOp{},
		timestamp:     time.Now,
		ssrcToHistory: make(map[uint32]*history),
	}
	for _, opt := range f.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}
	return i, nil
}

type Interceptor struct {
	interceptor.NoOp
	lock          sync.Mutex
	timestamp     func() time.Time
	ssrcToHistory map[uint32]*history
}

// BindLocalStream implements interceptor.Interceptor.
func (i *Interceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var twccHdrExtID uint8
	var useTWCC bool
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == transportCCURI {
			twccHdrExtID = uint8(e.ID)
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
	i.ssrcToHistory[ssrc] = newHistory(200)

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
			ssrc = 0
			var twccHdrExt rtp.TransportCCExtension
			twccHdrExt.Unmarshal(header.GetExtension(twccHdrExtID))
			seqNr = twccHdrExt.TransportSequence
		}
		i.ssrcToHistory[ssrc].add(seqNr, uint16(header.MarshalSize()+len(payload)), i.timestamp())
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

		pktReportLists := map[uint32]*PacketReportList{}

		pkts, err := attr.GetRTCPPackets(buf)
		for _, pkt := range pkts {
			var reportLists map[uint32]acknowledgementList
			var reportDeparture time.Time
			switch fb := pkt.(type) {
			case *rtcp.CCFeedbackReport:
				reportDeparture, reportLists = convertCCFB(now, fb)
			case *rtcp.TransportLayerCC:
				reportDeparture, reportLists = convertTWCC(now, fb)
			}
			for ssrc, reportList := range reportLists {
				prl := i.ssrcToHistory[ssrc].getReportForAck(reportList)
				prl.Departure = reportDeparture
				if l, ok := pktReportLists[ssrc]; !ok {
					pktReportLists[ssrc] = &prl
				} else {
					l.Reports = append(l.Reports, prl.Reports...)
				}
			}
		}
		attr.Set(CCFBAttributesKey, pktReportLists)
		return n, attr, err
	})
}

// Close implements interceptor.Interceptor.
func (i *Interceptor) Close() error {
	panic("unimplemented")
}

// UnbindLocalStream implements interceptor.Interceptor.
func (i *Interceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	panic("unimplemented")
}
