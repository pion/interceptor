package packetdump

import (
	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// NewReceiverInterceptor returns a new ReceiverInterceptor
func NewReceiverInterceptor(opts ...PacketDumperOption) (*ReceiverInterceptor, error) {
	dumper, err := NewPacketDumper(opts...)
	if err != nil {
		return nil, err
	}
	return &ReceiverInterceptor{
		PacketDumper: dumper,
	}, nil
}

// ReceiverInterceptor interceptor dumps outgoing RTP packets.
type ReceiverInterceptor struct {
	*PacketDumper
}

// Transform transforms a given set of receiver interceptor pipes.
func (r *ReceiverInterceptor) Transform(rtcpSink rtpio.RTCPWriter, rtpSrc rtpio.RTPReader, rtcpSrc rtpio.RTCPReader) rtpio.RTPReader {
	go func() {
		for {
			pkts := make([]rtcp.Packet, 16)
			_, err := rtcpSrc.ReadRTCP(pkts)
			if err != nil {
				return
			}

			r.logRTCPPackets(pkts)
		}
	}()

	return &receiverRTPReader{
		interceptor: r,
		rtpSrc:      rtpSrc,
	}
}

type receiverRTPReader struct {
	interceptor *ReceiverInterceptor
	rtpSrc      rtpio.RTPReader
}

func (r *receiverRTPReader) ReadRTP(pkt *rtp.Packet) (int, error) {
	i, err := r.rtpSrc.ReadRTP(pkt)
	if err != nil {
		return 0, err
	}

	r.interceptor.logRTPPacket(&pkt.Header, pkt.Payload)
	return i, nil
}

// Close closes the interceptor
func (r *ReceiverInterceptor) Close() error {
	return r.PacketDumper.Close()
}
