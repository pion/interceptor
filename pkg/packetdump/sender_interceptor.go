// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

import (
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// SenderInterceptorFactory is a interceptor.Factory for a SenderInterceptor
type SenderInterceptorFactory struct {
	opts []PacketDumperOption
}

// NewSenderInterceptor returns a new SenderInterceptorFactory
func NewSenderInterceptor(opts ...PacketDumperOption) (*SenderInterceptorFactory, error) {
	return &SenderInterceptorFactory{
		opts: opts,
	}, nil
}

// NewInterceptor returns a new SenderInterceptor interceptor
func (s *SenderInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	dumper, err := NewPacketDumper(s.opts...)
	if err != nil {
		return nil, err
	}
	i := &SenderInterceptor{
		PacketDumper: dumper,
	}
	return i, nil
}

// SenderInterceptor responds to nack feedback messages
type SenderInterceptor struct {
	interceptor.NoOp
	*PacketDumper
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (s *SenderInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	return interceptor.RTCPWriterFunc(func(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
		s.logRTCPPackets(pkts, attributes)
		return writer.Write(pkts, attributes)
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (s *SenderInterceptor) BindLocalStream(_ *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		s.logRTPPacket(header, payload, attributes)
		return writer.Write(header, payload, attributes)
	})
}

// Close closes the interceptor
func (s *SenderInterceptor) Close() error {
	return s.PacketDumper.Close()
}
