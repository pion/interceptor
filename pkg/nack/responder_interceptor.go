package nack

import (
	"fmt"
	"sync"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ResponderInterceptor responds to nack feedback messages
type ResponderInterceptor struct {
	size uint16
	log  logging.LeveledLogger

	sendBuffers   map[uint32]*sendBuffer
	sendBuffersMu sync.Mutex
}

// NewResponderInterceptor constructs a new ResponderInterceptor
func NewResponderInterceptor(opts ...ResponderOption) (*ResponderInterceptor, error) {
	r := &ResponderInterceptor{
		size:        8192,
		log:         logging.NewDefaultLoggerFactory().NewLogger("nack_responder"),
		sendBuffers: map[uint32]*sendBuffer{},
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	allowedSizes := make([]uint16, 0)
	correctSize := false
	for i := 0; i < 16; i++ {
		if r.size == 1<<i {
			correctSize = true
			break
		}
		allowedSizes = append(allowedSizes, 1<<i)
	}

	if !correctSize {
		return nil, fmt.Errorf("%w: %d is not a valid size, allowed sizes: %v", ErrInvalidSize, r.size, allowedSizes)
	}

	return r, nil
}

// Transform transforms a given set of sender interceptor pipes.
func (n *ResponderInterceptor) Transform(rtpSink rtpio.RTPWriter, rtcpSink rtpio.RTCPWriter, rtcpSrc rtpio.RTCPReader) rtpio.RTPWriter {
	go func() {
		for {
			pkts := make([]rtcp.Packet, 16)
			ct, err := rtcpSrc.ReadRTCP(pkts)
			if err != nil {
				return
			}

			for _, pkt := range pkts[:ct] {
				nack, ok := pkt.(*rtcp.TransportLayerNack)
				if !ok {
					continue
				}
				go n.resendPackets(rtpSink, nack)
			}
		}
	}()

	r := &responderRTPWriter{
		interceptor: n,
		rtpSink:     rtpSink,
	}
	return r
}

type responderRTPWriter struct {
	interceptor *ResponderInterceptor
	rtpSink     rtpio.RTPWriter
}

func (n *responderRTPWriter) WriteRTP(pkt *rtp.Packet) (int, error) {
	n.interceptor.sendBuffersMu.Lock()
	buf, ok := n.interceptor.sendBuffers[pkt.SSRC]
	if !ok {
		buf = newSendBuffer(n.interceptor.size)
		n.interceptor.sendBuffers[pkt.SSRC] = buf
	}
	n.interceptor.sendBuffersMu.Unlock()

	buf.add(pkt)
	if n.rtpSink == nil {
		return 0, nil
	}
	return n.rtpSink.WriteRTP(pkt)
}

func (n *ResponderInterceptor) resendPackets(rtpSink rtpio.RTPWriter, nack *rtcp.TransportLayerNack) {
	if rtpSink == nil {
		return
	}

	n.sendBuffersMu.Lock()
	stream, ok := n.sendBuffers[nack.MediaSSRC]
	n.sendBuffersMu.Unlock()
	if !ok {
		return
	}

	for i := range nack.Nacks {
		nack.Nacks[i].Range(func(seq uint16) bool {
			if p := stream.get(seq); p != nil {
				if _, err := rtpSink.WriteRTP(p); err != nil {
					n.log.Warnf("failed resending nacked packet: %+v", err)
				}
			}

			return true
		})
	}
}

// Close does nothing.
func (n *ResponderInterceptor) Close() error {
	return nil
}
