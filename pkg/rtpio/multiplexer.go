package rtpio

import (
	"io"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type rtpWriter struct {
	dst io.Writer
}

// WriteRTP writes a RTP packet to the underlying `io.Writer`.
func (w rtpWriter) WriteRTP(pkt *rtp.Packet) (int, error) {
	b, err := pkt.Marshal()
	if err != nil {
		return 0, err
	}
	return w.dst.Write(b)
}

type rtcpWriter struct {
	dst io.Writer
}

// WriteRTCP writes a RTCP packet to the underlying `io.Writer`.
func (w rtcpWriter) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	b, err := rtcp.Marshal(pkts)
	if err != nil {
		return 0, err
	}
	return w.dst.Write(b)
}

// NewRTPRTCPMultiplexer creates a new RTP/RTCP multiplexer over an `io.Writer`.
func NewRTPRTCPMultiplexer(w io.Writer) (RTPWriter, RTCPWriter) {
	return rtpWriter{dst: w}, rtcpWriter{dst: w}
}
