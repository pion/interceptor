package nack

import (
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ResponderInterceptor responds to nack feedback messages
type ResponderInterceptor struct {
	interceptor.NoOp
	size     uint16
	log      logging.LeveledLogger
	sessions map[interceptor.SessionID]map[uint32]*localStream
	mu       sync.Mutex
}

type localStream struct {
	sendBuffer *sendBuffer
	rtpWriter  interceptor.RTPWriter
}

// NewResponderInterceptor returns a new GeneratorInterceptor interceptor
func NewResponderInterceptor(opts ...ResponderOption) (*ResponderInterceptor, error) {
	r := &ResponderInterceptor{
		size:     8192,
		log:      logging.NewDefaultLoggerFactory().NewLogger("nack_responder"),
		sessions: map[interceptor.SessionID]map[uint32]*localStream{},
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	if _, err := newSendBuffer(r.size); err != nil {
		return nil, err
	}

	return r, nil
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (n *ResponderInterceptor) BindRTCPReader(sessionID interceptor.SessionID, reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		pkts, err := rtcp.Unmarshal(b[:i])
		if err != nil {
			return 0, nil, err
		}
		for _, rtcpPacket := range pkts {
			nack, ok := rtcpPacket.(*rtcp.TransportLayerNack)
			if !ok {
				continue
			}

			go n.resendPackets(sessionID, nack)
		}

		return i, attr, err
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (n *ResponderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	if !streamSupportNack(info) {
		return writer
	}

	// error is already checked in NewGeneratorInterceptor
	sendBuffer, _ := newSendBuffer(n.size)
	n.mu.Lock()
	ssrcs, ok := n.sessions[info.SessionID]
	if !ok {
		ssrcs = make(map[uint32]*localStream, 1)
		n.sessions[info.SessionID] = ssrcs
	}
	ssrcs[info.SSRC] = &localStream{sendBuffer: sendBuffer, rtpWriter: writer}
	n.mu.Unlock()

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		sendBuffer.add(&rtp.Packet{Header: *header, Payload: payload})
		return writer.Write(header, payload, attributes)
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (n *ResponderInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	n.mu.Lock()
	delete(n.sessions[info.SessionID], info.SSRC)
	n.mu.Unlock()
}

func (n *ResponderInterceptor) resendPackets(sessionID interceptor.SessionID, nack *rtcp.TransportLayerNack) {
	n.mu.Lock()
	stream, ok := n.sessions[sessionID][nack.MediaSSRC]
	n.mu.Unlock()
	if !ok {
		return
	}

	for i := range nack.Nacks {
		nack.Nacks[i].Range(func(seq uint16) bool {
			if p := stream.sendBuffer.get(seq); p != nil {
				if _, err := stream.rtpWriter.Write(&p.Header, p.Payload, interceptor.Attributes{}); err != nil {
					n.log.Warnf("failed resending nacked packet: %+v", err)
				}
			}

			return true
		})
	}
}
