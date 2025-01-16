package ccfb

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

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
	i.lock.Lock()
	defer i.lock.Unlock()
	i.ssrcToHistory[info.SSRC] = newHistory()

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		i.lock.Lock()
		defer i.lock.Unlock()
		i.ssrcToHistory[header.SSRC].add(header.SequenceNumber, uint16(header.MarshalSize()+len(payload)), i.timestamp())
		return writer.Write(header, payload, attributes)
	})
}

// BindRTCPReader implements interceptor.Interceptor.
func (i *Interceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		now := i.timestamp()

		n, attr, err := reader.Read(b, a)
		if err != nil {
			return n, attr, err
		}
		buf := make([]byte, n)
		copy(buf, b[:n])

		if attr == nil {
			attr = make(interceptor.Attributes)
		}

		pktReportLists := map[uint32]*PacketReportList{}

		pkts, err := attr.GetRTCPPackets(buf)
		for _, pkt := range pkts {
			switch fb := pkt.(type) {
			case *rtcp.CCFeedbackReport:
				reportLists := convertCCFB(now, fb)
				for ssrc, reportList := range reportLists {
					prl := i.ssrcToHistory[ssrc].getReportForAck(reportList)
					if l, ok := pktReportLists[ssrc]; !ok {
						pktReportLists[ssrc] = &prl
					} else {
						l.Reports = append(l.Reports, prl.Reports...)
					}
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
