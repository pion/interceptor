package twcc

import (
	"math/rand"
	"time"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtp"
)

// NewSenderInterceptor constructs a new SenderInterceptor
func NewSenderInterceptor(defaultHdrExtID uint8, opts ...Option) (*SenderInterceptor, error) {
	recorder := NewRecorder(rand.Uint32()) // #nosec
	i := &SenderInterceptor{
		defaultHdrExtID: defaultHdrExtID,
		log:             logging.NewDefaultLoggerFactory().NewLogger("twcc_sender_interceptor"),
		close:           make(chan struct{}),
		interval:        100 * time.Millisecond,
		startTime:       time.Now(),
		recorder:        recorder,
	}

	for _, opt := range opts {
		err := opt(i)
		if err != nil {
			return nil, err
		}
	}

	return i, nil
}

// SenderInterceptor sends transport wide congestion control reports as specified in:
// https://datatracker.ietf.org/doc/html/draft-holmer-rmcat-transport-wide-cc-extensions-01
type SenderInterceptor struct {
	defaultHdrExtID uint8
	log             logging.LeveledLogger

	close chan struct{}

	interval  time.Duration
	startTime time.Time

	recorder *Recorder
}

// An Option is a function that can be used to configure a SenderInterceptor
type Option func(*SenderInterceptor) error

// SendInterval sets the interval at which the interceptor
// will send new feedback reports.
func SendInterval(interval time.Duration) Option {
	return func(s *SenderInterceptor) error {
		s.interval = interval
		return nil
	}
}

// Transform transforms a given set of sender interceptor pipes.
func (s *SenderInterceptor) Transform(rtcpSink rtpio.RTCPWriter, rtpSrc rtpio.RTPReader, rtcpSrc rtpio.RTCPReader) rtpio.RTPReader {
	go s.loop(rtcpSink)

	return &senderRTPReader{
		interceptor: s,
		rtpSrc:      rtpSrc,
	}
}

type senderRTPReader struct {
	interceptor *SenderInterceptor
	rtpSrc      rtpio.RTPReader
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (s *senderRTPReader) ReadRTP(pkt *rtp.Packet) (int, error) {
	n, err := s.rtpSrc.ReadRTP(pkt)
	if err != nil {
		return n, err
	}

	var tccExt rtp.TransportCCExtension
	if ext := pkt.Header.GetExtension(s.interceptor.defaultHdrExtID); ext != nil {
		err = tccExt.Unmarshal(ext)
		if err != nil {
			return 0, err
		}
		s.interceptor.recorder.Record(pkt.SSRC, tccExt.TransportSequence, time.Since(s.interceptor.startTime).Microseconds())
	}

	return n, err
}

// Close closes the interceptor.
func (s *SenderInterceptor) Close() error {
	close(s.close)
	return nil
}

func (s *SenderInterceptor) loop(w rtpio.RTCPWriter) {
	ticker := time.NewTicker(s.interval)
	for {
		select {
		case <-s.close:
			ticker.Stop()
			return
		case <-ticker.C:
			// build and send twcc
			pkts := s.recorder.BuildFeedbackPacket()
			if _, err := w.WriteRTCP(pkts); err != nil {
				s.log.Error(err.Error())
			}
		}
	}
}
