package report

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

func ntpTime(t time.Time) uint64 {
	// seconds since 1st January 1900
	s := (float64(t.UnixNano()) / 1000000000) + 2208988800

	// higher 32 bits are the integer part, lower 32 bits are the fractional part
	integerPart := uint32(s)
	fractionalPart := uint32((s - float64(integerPart)) * 0xFFFFFFFF)
	return uint64(integerPart)<<32 | uint64(fractionalPart)
}

// SenderInterceptor interceptor generates sender reports.
type SenderInterceptor struct {
	interceptor.NoOp
	interval time.Duration
	now      func() time.Time
	sessions map[interceptor.SessionID]*senderSessionCtx
	log      logging.LeveledLogger
	mu       sync.Mutex
}

type senderSessionCtx struct {
	streams  sync.Map
	teardown chan struct{}
	torndown chan struct{}
}

// NewSenderInterceptor returns a new SenderInterceptor interceptor.
func NewSenderInterceptor(opts ...SenderOption) (*SenderInterceptor, error) {
	s := &SenderInterceptor{
		interval: 1 * time.Second,
		now:      time.Now,
		sessions: map[interceptor.SessionID]*senderSessionCtx{},
		log:      logging.NewDefaultLoggerFactory().NewLogger("sender_interceptor"),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// newSession finds the current session by ID or creates a new one if it
// doesn't exist. The caller must hold the lock.
func (s *SenderInterceptor) newSession(sessionID interceptor.SessionID) *senderSessionCtx {
	sCtx := &senderSessionCtx{
		streams:  sync.Map{},
		teardown: make(chan struct{}),
		torndown: make(chan struct{}),
	}
	s.sessions[sessionID] = sCtx

	return sCtx
}

// Close closes the interceptor.
func (s *SenderInterceptor) Close(sessionID interceptor.SessionID) error {
	s.mu.Lock()
	sCtx, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	if !ok {
		return nil
	}

	// Close loop goroutine.
	close(sCtx.teardown)

	// Wait for the goroutine to exit.
	<-sCtx.torndown

	return nil
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (s *SenderInterceptor) BindRTCPWriter(sessionID interceptor.SessionID, writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	s.mu.Lock()
	defer s.mu.Unlock()

	sCtx := s.newSession(sessionID)
	sCtx.torndown = make(chan struct{})

	// TODO(jeremija) figure out if there's a way to run a single loop for all
	// sessions.
	go s.loop(sCtx, writer)

	return writer
}

func (s *SenderInterceptor) loop(sCtx *senderSessionCtx, rtcpWriter interceptor.RTCPWriter) {
	defer close(sCtx.torndown)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := s.now()
			sCtx.streams.Range(func(key, value interface{}) bool {
				ssrc := key.(uint32)
				stream := value.(*senderStream)

				stream.m.Lock()
				defer stream.m.Unlock()

				sr := &rtcp.SenderReport{
					SSRC:        ssrc,
					NTPTime:     ntpTime(now),
					RTPTime:     stream.lastRTPTimeRTP + uint32(now.Sub(stream.lastRTPTimeTime).Seconds()*stream.clockRate),
					PacketCount: stream.packetCount,
					OctetCount:  stream.octetCount,
				}

				if _, err := rtcpWriter.Write([]rtcp.Packet{sr}, interceptor.Attributes{}); err != nil {
					s.log.Warnf("failed sending: %+v", err)
				}

				return true
			})

		case <-sCtx.teardown:
			return
		}
	}
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (s *SenderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	stream := newSenderStream(info.ClockRate)

	s.mu.Lock()
	// TODO(jeremija) what if BindRTCPWriter was not called before? Current code
	// would panic. I think we need to bail out either by returning an error or
	// just silently return the writer without intercepting (meh).
	sCtx := s.sessions[info.SessionID]
	s.mu.Unlock()

	sCtx.streams.Store(info.SSRC, stream)

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, a interceptor.Attributes) (int, error) {
		stream.processRTP(s.now(), header, payload)

		return writer.Write(header, payload, a)
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (s *SenderInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	s.mu.Lock()
	if sCtx, ok := s.sessions[info.SessionID]; ok {
		sCtx.streams.Delete(info.SSRC)
	}
	s.mu.Unlock()
}
