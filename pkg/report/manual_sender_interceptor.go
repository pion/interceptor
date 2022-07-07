package report

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ManualSenderInterceptorFactory is a interceptor.Factory for a ManualSenderInterceptor
type ManualSenderInterceptorFactory struct {
	opts []ManualSenderOption
}

// NewInterceptor constructs a new ManualSenderInterceptor
func (s *ManualSenderInterceptorFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	i := &ManualSenderInterceptor{
		interval: 1 * time.Second,
		now:      time.Now,
		log:      logging.NewDefaultLoggerFactory().NewLogger("manual_sender_interceptor"),
		close:    make(chan struct{}),
	}

	for _, opt := range s.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	return i, nil
}

// NewManualSenderInterceptor returns a new ManualSenderInterceptorFactory
func NewManualSenderInterceptor(opts ...ManualSenderOption) (*ManualSenderInterceptorFactory, error) {
	return &ManualSenderInterceptorFactory{opts}, nil
}

// ManualSenderInterceptor interceptor allows the developer to publish sender reports.
// This is useful when there is an external clock (such as the track being forwarded) for synchronization.
// This interceptor adjusts the SSRC on the SenderReport but other fields must be manually populated.
type ManualSenderInterceptor struct {
	interceptor.NoOp
	interval time.Duration
	now      func() time.Time
	ssrcs    []uint32
	writers  []interceptor.RTCPWriter
	log      logging.LeveledLogger
	m        sync.Mutex
	wg       sync.WaitGroup
	close    chan struct{}
}

func (s *ManualSenderInterceptor) isClosed() bool {
	select {
	case <-s.close:
		return true
	default:
		return false
	}
}

// Close closes the interceptor.
func (s *ManualSenderInterceptor) Close() error {
	defer s.wg.Wait()
	s.m.Lock()
	defer s.m.Unlock()

	if !s.isClosed() {
		close(s.close)
	}

	return nil
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (s *ManualSenderInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	s.m.Lock()
	defer s.m.Unlock()

	if s.isClosed() {
		return writer
	}

	s.wg.Add(1)

	s.writers = append(s.writers, writer)

	return writer
}

func (s *ManualSenderInterceptor) WriteSenderReport(report *rtcp.SenderReport) {
	defer s.wg.Done()

	for _, ssrc := range s.ssrcs {
		report.SSRC = ssrc

		for _, writer := range s.writers {
			if _, err := writer.Write([]rtcp.Packet{report}, interceptor.Attributes{}); err != nil {
				s.log.Warnf("failed sending: %+v", err)
			}
		}
	}
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (s *ManualSenderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	s.m.Lock()
	defer s.m.Unlock()

	s.ssrcs = append(s.ssrcs, info.SSRC)

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, a interceptor.Attributes) (int, error) {
		return writer.Write(header, payload, a)
	})
}
