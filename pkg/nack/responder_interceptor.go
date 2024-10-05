// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package nack

import (
	"encoding/binary"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// ResponderInterceptorFactory is a interceptor.Factory for a ResponderInterceptor
type ResponderInterceptorFactory struct {
	opts []ResponderOption
}

type packetFactory interface {
	NewPacket(header *rtp.Header, payload []byte) (*retainablePacket, error)
}

// NewInterceptor constructs a new ResponderInterceptor
func (r *ResponderInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	i := &ResponderInterceptor{
		streamsFilter: streamSupportNack,
		size:          1024,
		log:           logging.NewDefaultLoggerFactory().NewLogger("nack_responder"),
		streams:       map[uint32]*localStream{},
	}

	for _, opt := range r.opts {
		if err := opt(i); err != nil {
			return nil, err
		}
	}

	if i.packetFactory == nil {
		i.packetFactory = newPacketManager()
	}

	if _, err := newSendBuffer(i.size); err != nil {
		return nil, err
	}

	return i, nil
}

// ResponderInterceptor responds to nack feedback messages
type ResponderInterceptor struct {
	interceptor.NoOp
	streamsFilter func(info *interceptor.StreamInfo) bool
	size          uint16
	log           logging.LeveledLogger
	packetFactory packetFactory

	streams   map[uint32]*localStream
	streamsMu sync.Mutex
}

type localStream struct {
	sendBuffer *sendBuffer
	rtpWriter  interceptor.RTPWriter

	// Non-zero if Retransmissions should be sent on a distinct stream
	rtxSsrc        uint32
	rtxPayloadType uint8
	rtxSequencer   rtp.Sequencer
}

// NewResponderInterceptor returns a new ResponderInterceptorFactor
func NewResponderInterceptor(opts ...ResponderOption) (*ResponderInterceptorFactory, error) {
	return &ResponderInterceptorFactory{opts}, nil
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (n *ResponderInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(b, a)
		if err != nil {
			return 0, nil, err
		}

		if attr == nil {
			attr = make(interceptor.Attributes)
		}
		pkts, err := attr.GetRTCPPackets(b[:i])
		if err != nil {
			return 0, nil, err
		}
		for _, rtcpPacket := range pkts {
			nack, ok := rtcpPacket.(*rtcp.TransportLayerNack)
			if !ok {
				continue
			}

			go n.resendPackets(nack)
		}

		return i, attr, err
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (n *ResponderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	if !n.streamsFilter(info) {
		return writer
	}

	// error is already checked in NewGeneratorInterceptor
	sendBuffer, _ := newSendBuffer(n.size)
	n.streamsMu.Lock()
	n.streams[info.SSRC] = &localStream{
		sendBuffer:     sendBuffer,
		rtpWriter:      writer,
		rtxSsrc:        info.SSRCRetransmission,
		rtxPayloadType: info.PayloadTypeRetransmission,
		rtxSequencer:   rtp.NewRandomSequencer(),
	}
	n.streamsMu.Unlock()

	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		pkt, err := n.packetFactory.NewPacket(header, payload)
		if err != nil {
			return 0, err
		}
		sendBuffer.add(pkt)
		return writer.Write(header, payload, attributes)
	})
}

// UnbindLocalStream is called when the Stream is removed. It can be used to clean up any data related to that track.
func (n *ResponderInterceptor) UnbindLocalStream(info *interceptor.StreamInfo) {
	n.streamsMu.Lock()
	delete(n.streams, info.SSRC)
	n.streamsMu.Unlock()
}

func (n *ResponderInterceptor) resendPackets(nack *rtcp.TransportLayerNack) {
	n.streamsMu.Lock()
	stream, ok := n.streams[nack.MediaSSRC]
	n.streamsMu.Unlock()
	if !ok {
		return
	}

	for i := range nack.Nacks {
		nack.Nacks[i].Range(func(seq uint16) bool {
			if p := stream.sendBuffer.get(seq); p != nil {
				if stream.rtxSsrc != 0 {
					// Store the original sequence number and rewrite the sequence number.
					originalSequenceNumber := p.Header().SequenceNumber
					p.Header().SequenceNumber = stream.rtxSequencer.NextSequenceNumber()

					// Rewrite the SSRC.
					p.Header().SSRC = stream.rtxSsrc
					// Rewrite the payload type.
					p.Header().PayloadType = stream.rtxPayloadType

					// Remove padding if present.
					paddingLength := 0
					originPayload := p.Payload()
					if p.Header().Padding {
						paddingLength = int(originPayload[len(originPayload)-1])
						p.Header().Padding = false
					}

					// Write the original sequence number at the beginning of the payload.
					payload := make([]byte, 2)
					binary.BigEndian.PutUint16(payload, originalSequenceNumber)
					payload = append(payload, originPayload[:len(originPayload)-paddingLength]...)

					// Send RTX packet.
					if _, err := stream.rtpWriter.Write(p.Header(), payload, interceptor.Attributes{}); err != nil {
						n.log.Warnf("failed sending rtx packet: %+v", err)
					}

					// Resore the Padding and SSRC.
					if paddingLength > 0 {
						p.Header().Padding = true
					}
					p.Header().SequenceNumber = originalSequenceNumber
				} else {
					if _, err := stream.rtpWriter.Write(p.Header(), p.Payload(), interceptor.Attributes{}); err != nil {
						n.log.Warnf("failed resending nacked packet: %+v", err)
					}
				}
				p.Release()
			}

			return true
		})
	}
}
