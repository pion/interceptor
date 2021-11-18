package packetdump

import (
	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// NewSenderInterceptor returns a new SenderInterceptor interceptor
func NewSenderInterceptor(opts ...PacketDumperOption) (*SenderInterceptor, error) {
	dumper, err := NewPacketDumper(opts...)
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
	*PacketDumper
}

// Transform transforms a given set of sender interceptor pipes.
func (s *SenderInterceptor) Transform(rtpSink rtpio.RTPWriter, rtcpSink rtpio.RTCPWriter, rtcpSrc rtpio.RTCPReader) rtpio.RTPWriter {
	go func() {
		for {
			pkts := make([]rtcp.Packet, 16)
			_, err := rtcpSrc.ReadRTCP(pkts)
			if err != nil {
				return
			}
			s.logRTCPPackets(pkts)
		}
	}()
	return &senderRTPWriter{
		interceptor: s,
		rtpSink:     rtpSink,
	}
}

type senderRTPWriter struct {
	interceptor *SenderInterceptor
	rtpSink     rtpio.RTPWriter
}

func (s *senderRTPWriter) WriteRTP(pkt *rtp.Packet) (int, error) {
	s.interceptor.logRTPPacket(&pkt.Header, pkt.Payload)
	return s.rtpSink.WriteRTP(pkt)
}

// Close closes the interceptor
func (s *SenderInterceptor) Close() error {
	return s.PacketDumper.Close()
}
