//+build scream

package scream

import (
	"sync"
	"time"

	"github.com/mengelbart/scream-go"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// TODO: This is copied from the report interceptor, maybe there's a better place for this?
func ntpTime(t time.Time) uint64 {
	// seconds since 1st January 1900
	s := (float64(t.UnixNano()) / 1000000000) + 2208988800

	// higher 32 bits are the integer part, lower 32 bits are the fractional part
	integerPart := uint32(s)
	fractionalPart := uint32((s - float64(integerPart)) * 0xFFFFFFFF)
	return uint64(integerPart)<<32 | uint64(fractionalPart)
}

// ReceiverInterceptor generates Feedback for SCReAM congestion control
type ReceiverInterceptor struct {
	interceptor.NoOp
	m     sync.Mutex
	wg    sync.WaitGroup
	close chan struct{}
	log   logging.LeveledLogger

	screamRx   map[uint32]*scream.Rx
	screamRxMu sync.Mutex
	interval   time.Duration
}

// NewReceiverInterceptor returns a new ReceiverInterceptor
func NewReceiverInterceptor() *ReceiverInterceptor {
	return &ReceiverInterceptor{
		interval: time.Millisecond * 10,
		close:    make(chan struct{}),
		log:      logging.NewDefaultLoggerFactory().NewLogger("scream_receiver"),
		screamRx: map[uint32]*scream.Rx{},
	}
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (r *ReceiverInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	r.m.Lock()
	defer r.m.Unlock()

	if r.isClosed() {
		return writer
	}

	r.wg.Add(1)

	go r.loop(writer)

	return writer
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (r *ReceiverInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	// TODO: Check if stream supports scream?

	rx := scream.NewRx(info.SSRC)
	r.screamRxMu.Lock()
	r.screamRx[info.SSRC] = rx
	r.screamRxMu.Unlock()

	return interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		pkt := rtp.Packet{}
		if err = pkt.Unmarshal(b[:i]); err != nil {
			return 0, nil, err
		}

		// TODO: Add support for ECN via ceBits?
		rx.Receive(ntpTime(time.Now()), pkt.SSRC, len(pkt.Raw), pkt.SequenceNumber, 0)
		return i, attr, nil
	})
}

// UnbindRemoteStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (r *ReceiverInterceptor) UnbindRemoteStream(info *interceptor.StreamInfo) {
	r.screamRxMu.Lock()
	delete(r.screamRx, info.SSRC)
	r.screamRxMu.Unlock()
}

// Close closes the interceptor.
func (r *ReceiverInterceptor) Close() error {
	defer r.wg.Wait()
	r.m.Lock()
	defer r.m.Unlock()

	if !r.isClosed() {
		close(r.close)
	}
	return nil
}

func (r *ReceiverInterceptor) loop(rtcpWriter interceptor.RTCPWriter) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			func() {
				r.screamRxMu.Lock()

				for _, rx := range r.screamRx {
					// TODO: Check meaning of isMark
					if ok, feedback := rx.CreateStandardizedFeedback(0, true); ok {
						fb := rtcp.RawPacket(feedback)
						if _, err := rtcpWriter.Write([]rtcp.Packet{&fb}, interceptor.Attributes{}); err != nil {
							r.log.Warnf("failed sending scream feedback report: %+v", err)
						}
					}
				}

				r.screamRxMu.Unlock()
			}()
		case <-r.close:
			return
		}
	}
}

func (r *ReceiverInterceptor) isClosed() bool {
	select {
	case <-r.close:
		return true
	default:
		return false
	}
}
