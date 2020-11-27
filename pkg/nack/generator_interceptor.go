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
	size        uint16
	skipLastN   uint16
	interval    time.Duration
	receiveLogs *sync.Map
	m           sync.Mutex
	wg          sync.WaitGroup
	close       chan struct{}
	log         logging.LeveledLogger

	remoteStreamBuf rtp.Packet
}

// NewGeneratorInterceptor returns a new GeneratorInterceptor interceptor
func NewGeneratorInterceptor(opts ...GeneratorOption) (*GeneratorInterceptor, error) {
	r := &GeneratorInterceptor{
		size:        8192,
		skipLastN:   0,
		interval:    time.Millisecond * 100,
		receiveLogs: &sync.Map{},
		close:       make(chan struct{}),
		log:         logging.NewDefaultLoggerFactory().NewLogger("nack_generator"),
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
func (n *GeneratorInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	n.m.Lock()
	defer n.m.Unlock()

	if n.isClosed() {
		return writer
	}

	n.wg.Add(1)

	go n.loop(writer)

	return writer
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (n *GeneratorInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	if !streamSupportNack(info) {
		return reader
	}

	// error is already checked in NewGeneratorInterceptor
	receiveLog, _ := newReceiveLog(n.size)
	n.receiveLogs.Store(info.SSRC, receiveLog)

	return interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		if err = n.remoteStreamBuf.Unmarshal(b[:i]); err != nil {
			return 0, nil, err
		}
		receiveLog.add(n.remoteStreamBuf.Header.SequenceNumber)

		return i, attr, nil
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (n *GeneratorInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	n.receiveLogs.Delete(info.SSRC)
}

// Close closes the interceptor
func (n *GeneratorInterceptor) Close() error {
	defer n.wg.Wait()
	n.m.Lock()
	defer n.m.Unlock()

	if !n.isClosed() {
		close(n.close)
	}

	return nil
}

func (n *GeneratorInterceptor) loop(rtcpWriter interceptor.RTCPWriter) {
	defer n.wg.Done()

	senderSSRC := rand.Uint32() // #nosec

	ticker := time.NewTicker(n.interval)
	for {
		select {
		case <-ticker.C:
			n.receiveLogs.Range(func(key, value interface{}) bool {
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

		case <-n.close:
			return
		}
	}
}

func (n *GeneratorInterceptor) isClosed() bool {
	select {
	case <-n.close:
		return true
	default:
		return false
	}
}
