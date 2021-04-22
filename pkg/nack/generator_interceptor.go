package nack

import (
	"math/rand"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// GeneratorInterceptor interceptor generates nack feedback messages.
type GeneratorInterceptor struct {
	interceptor.NoOp
	size      uint16
	skipLastN uint16
	interval  time.Duration
	sessions  map[interceptor.SessionID]*sessionCtx
	mu        sync.Mutex
	log       logging.LeveledLogger
}

type sessionCtx struct {
	teardown    chan struct{}
	torndown    chan struct{}
	receiveLogs sync.Map
}

// NewGeneratorInterceptor returns a new GeneratorInterceptor interceptor
func NewGeneratorInterceptor(opts ...GeneratorOption) (*GeneratorInterceptor, error) {
	r := &GeneratorInterceptor{
		size:      8192,
		skipLastN: 0,
		interval:  time.Millisecond * 100,
		sessions:  map[interceptor.SessionID]*sessionCtx{},
		log:       logging.NewDefaultLoggerFactory().NewLogger("nack_generator"),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	if _, err := newReceiveLog(r.size); err != nil {
		return nil, err
	}

	return r, nil
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (n *GeneratorInterceptor) BindRTCPWriter(sessionID interceptor.SessionID, writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	n.mu.Lock()
	defer n.mu.Unlock()

	sCtx := n.newSession(sessionID)

	// TODO(jeremija) figure out if there's a way to run a single loop for all sessions.
	go n.loop(sCtx, writer)

	return writer
}

// newSession finds the current session by ID or creates a new one if it
// doesn't exist. The caller must hold the lock.
func (n *GeneratorInterceptor) newSession(sessionID interceptor.SessionID) *sessionCtx {
	sCtx := &sessionCtx{
		teardown:    make(chan struct{}),
		torndown:    make(chan struct{}),
		receiveLogs: sync.Map{},
	}
	// TODO(jeremija) what if there are conflicts? The caller should ensure that
	// sessionIDs are unique.
	n.sessions[sessionID] = sCtx

	return sCtx
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (n *GeneratorInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	if !streamSupportNack(info) {
		return reader
	}

	// error is already checked in NewGeneratorInterceptor
	rcvLog, _ := newReceiveLog(n.size)
	n.mu.Lock()
	// TODO(jeremija) what if BindRTCPWriter was not called before? Current code
	// would panic. I think we need to bail out either by returning an error or
	// just silently return the reader without intercepting (meh).
	sCtx := n.sessions[info.SessionID]
	sCtx.receiveLogs.Store(info.SSRC, rcvLog)
	n.mu.Unlock()

	return interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		pkt := rtp.Packet{}
		if err = pkt.Unmarshal(b[:i]); err != nil {
			return 0, nil, err
		}
		rcvLog.add(pkt.Header.SequenceNumber)

		return i, attr, nil
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (n *GeneratorInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	n.mu.Lock()
	if sCtx, ok := n.sessions[info.SessionID]; ok {
		sCtx.receiveLogs.Delete(info.SSRC)
	}
	n.mu.Unlock()
}

// Close closes the interceptor
func (n *GeneratorInterceptor) Close(sessionID interceptor.SessionID) error {
	n.mu.Lock()
	sCtx, ok := n.sessions[sessionID]
	delete(n.sessions, sessionID)
	n.mu.Unlock()

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

func (n *GeneratorInterceptor) loop(sCtx *sessionCtx, rtcpWriter interceptor.RTCPWriter) {
	defer close(sCtx.torndown)

	senderSSRC := rand.Uint32() // #nosec

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sCtx.receiveLogs.Range(func(key, value interface{}) bool {
				ssrc := key.(uint32)
				receiveLog := value.(*receiveLog)

				missing := receiveLog.missingSeqNumbers(n.skipLastN)
				if len(missing) == 0 {
					return true
				}

				nack := &rtcp.TransportLayerNack{
					SenderSSRC: senderSSRC,
					MediaSSRC:  ssrc,
					Nacks:      rtcp.NackPairsFromSequenceNumbers(missing),
				}

				if _, err := rtcpWriter.Write([]rtcp.Packet{nack}, interceptor.Attributes{}); err != nil {
					n.log.Warnf("failed sending nack: %+v", err)
				}

				return true
			})
		case <-sCtx.teardown:
			return
		}
	}
}
