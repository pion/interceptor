package report

import (
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ReceiverInterceptor interceptor generates receiver reports.
type ReceiverInterceptor struct {
	interceptor.NoOp
	interval time.Duration
	now      func() time.Time
	sessions map[interceptor.SessionID]*receiverSessionCtx
	log      logging.LeveledLogger
	mu       sync.Mutex
}

// NewReceiverInterceptor returns a new ReceiverInterceptor interceptor.
func NewReceiverInterceptor(opts ...ReceiverOption) (*ReceiverInterceptor, error) {
	r := &ReceiverInterceptor{
		interval: 1 * time.Second,
		now:      time.Now,
		sessions: map[interceptor.SessionID]*receiverSessionCtx{},
		log:      logging.NewDefaultLoggerFactory().NewLogger("receiver_interceptor"),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	return r, nil
}

type receiverSessionCtx struct {
	streams  sync.Map
	teardown chan struct{}
	torndown chan struct{}
}

// Close closes the interceptor.
func (r *ReceiverInterceptor) Close(sessionID interceptor.SessionID) error {
	r.mu.Lock()
	sCtx, ok := r.sessions[sessionID]
	delete(r.sessions, sessionID)
	r.mu.Unlock()

	if !ok {
		// TODO(jeremija) maybe return an error?
		return nil
	}

	// Close loop goroutine.
	close(sCtx.teardown)

	// Wait for the goroutine to exit.
	<-sCtx.torndown

	return nil
}

// newSession finds the current session by ID or creates a new one if it
// doesn't exist. The caller must hold the lock.
func (r *ReceiverInterceptor) newSession(sessionID interceptor.SessionID) *receiverSessionCtx {
	sCtx := &receiverSessionCtx{
		streams:  sync.Map{},
		teardown: make(chan struct{}),
		torndown: make(chan struct{}),
	}
	// TODO(jeremija) what if there are conflicts? The caller should ensure that
	// sessionIDs are unique.
	r.sessions[sessionID] = sCtx

	return sCtx
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (r *ReceiverInterceptor) BindRTCPWriter(sessionID interceptor.SessionID, writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	r.mu.Lock()
	defer r.mu.Unlock()

	sCtx := r.newSession(sessionID)

	// TODO(jeremija) figure out if there's a way to run a single loop for all sessions.
	go r.loop(sCtx, writer)

	return writer
}

func (r *ReceiverInterceptor) loop(sCtx *receiverSessionCtx, rtcpWriter interceptor.RTCPWriter) {
	defer close(sCtx.torndown)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := r.now()
			sCtx.streams.Range(func(key, value interface{}) bool {
				stream := value.(*receiverStream)

				var pkts []rtcp.Packet

				pkts = append(pkts, stream.generateReport(now))

				if _, err := rtcpWriter.Write(pkts, interceptor.Attributes{}); err != nil {
					r.log.Warnf("failed sending: %+v", err)
				}

				return true
			})

		case <-sCtx.teardown:
			return
		}
	}
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (r *ReceiverInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	stream := newReceiverStream(info.SSRC, info.ClockRate)

	r.mu.Lock()
	// TODO(jeremija) what if BindRTCPWriter was not called here? Current code
	// would panic. I think we need to bail out either by returning an error or
	// just silently return the writer without intercepting (meh).
	sCtx := r.sessions[info.SessionID]
	r.mu.Unlock()

	sCtx.streams.Store(info.SSRC, stream)

	return interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		pkt := rtp.Packet{}
		if err = pkt.Unmarshal(b[:i]); err != nil {
			return 0, nil, err
		}

		stream.processRTP(r.now(), &pkt)

		return i, attr, nil
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (r *ReceiverInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	r.mu.Lock()
	if sCtx, ok := r.sessions[info.SessionID]; ok {
		sCtx.streams.Delete(info.SSRC)
	}
	r.mu.Unlock()
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (r *ReceiverInterceptor) BindRTCPReader(sessionID interceptor.SessionID, reader interceptor.RTCPReader) interceptor.RTCPReader {
	r.mu.Lock()
	// TODO(jeremija) what if BindRTCPWriter was not called before? Current code
	// would panic. I think we need to bail out either by returning an error or
	// just silently return the writer without intercepting (meh).
	sCtx := r.sessions[sessionID]
	r.mu.Unlock()

	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		pkts, err := rtcp.Unmarshal(b[:i])
		if err != nil {
			return 0, nil, err
		}

		for _, pkt := range pkts {
			if sr, ok := (pkt).(*rtcp.SenderReport); ok {
				value, ok := sCtx.streams.Load(sr.SSRC)
				if !ok {
					continue
				}

				stream := value.(*receiverStream)
				stream.processSenderReport(r.now(), sr)
			}
		}

		return i, attr, nil
	})
}
