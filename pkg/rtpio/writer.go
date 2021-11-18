package rtpio

import (
	"io"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPWriter is used by Interceptor.BindLocalStream.
type RTPWriter interface {
	WriteRTP(pkt *rtp.Packet) (int, error)
}

// RTCPWriter is used by Interceptor.BindRTCPWriter.
type RTCPWriter interface {
	WriteRTCP(pkts []rtcp.Packet) (int, error)
}

// RawRTPWriter is a RTP packet writer that writes to an io.Writer`.`
type RawRTPWriter struct {
	dst io.Writer
}

// WriteRTP writes a RTP packet to the underlying writer.
func (w *RawRTPWriter) WriteRTP(pkt *rtp.Packet) (int, error) {
	b, err := pkt.Marshal()
	if err != nil {
		return 0, err
	}
	return w.dst.Write(b)
}

// NewRTPWriter creates a new RTP packet writer.
func NewRTPWriter(w io.Writer) RTPWriter {
	return &RawRTPWriter{
		dst: w,
	}
}

// RawRTCPWriter is a writer that writes RTCP packets to an `io.Writer``.
type RawRTCPWriter struct {
	dst io.Writer
}

// WriteRTCP writes a slice of RTCP packets to the underlying writer.
func (w *RawRTCPWriter) WriteRTCP(pkts []rtcp.Packet) (int, error) {
	b, err := rtcp.Marshal(pkts)
	if err != nil {
		return 0, err
	}
	return w.dst.Write(b)
}

// NewRTCPWriter creates a new RTCP packet writer.
func NewRTCPWriter(w io.Writer) RTCPWriter {
	return &RawRTCPWriter{
		dst: w,
	}
}
