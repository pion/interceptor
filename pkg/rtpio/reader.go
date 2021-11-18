package rtpio

import (
	"io"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// RTPReader is used by Interceptor.BindRemoteStream.
type RTPReader interface {
	ReadRTP(pkt *rtp.Packet) (int, error)
}

// RTCPReader is used by Interceptor.BindRTCPReader.
type RTCPReader interface {
	// Read a batch of rtcp packets. This returns the number of packets read, not the number of bytes!
	ReadRTCP([]rtcp.Packet) (int, error)
}

// RawRTPReader is a RTPReader that reads from an `io.Reader`.
type RawRTPReader struct {
	src io.Reader
	mtu int
}

// ReadRTP reads a single RTP packet from the underlying reader.
func (r *RawRTPReader) ReadRTP(pkt *rtp.Packet) (int, error) {
	buf := make([]byte, r.mtu)
	n, err := r.src.Read(buf)
	if err != nil {
		return 0, err
	}
	if err := pkt.Unmarshal(buf[:n]); err != nil {
		return 0, err
	}
	return n, nil
}

// NewRTPReader creates a new RTP packet reader.
func NewRTPReader(r io.Reader, mtu int) RTPReader {
	return &RawRTPReader{src: r, mtu: mtu}
}

// RawRTCPReader is a RTCPReader that reads from an `io.Reader`.
type RawRTCPReader struct {
	src     io.Reader
	mtu     int
	backlog []rtcp.Packet
}

// ReadRTCP reads a batch of RTCP packets from the underlying reader.
func (r *RawRTCPReader) ReadRTCP(pkts []rtcp.Packet) (int, error) {
	// read from backlog first.
	if len(r.backlog) > 0 {
		n := copy(pkts, r.backlog)
		r.backlog = r.backlog[n:]
		if n == len(pkts) {
			// we filled up all the packets.
			return n, nil
		}
	}
	buf := make([]byte, r.mtu)
	n, err := r.src.Read(buf)
	if err != nil {
		return 0, err
	}
	p, err := rtcp.Unmarshal(buf[:n])
	if err != nil {
		return 0, err
	}
	ct := copy(pkts, p)

	if ct < len(p) {
		// we didn't fill up all the packets so mark some of them as backlogged for
		// the next read.
		r.backlog = append(r.backlog, p[ct:]...)
	}
	return ct, nil
}

// NewRTCPReader creates a new RTCP packet reader.
func NewRTCPReader(r io.Reader, mtu int) RTCPReader {
	return &RawRTCPReader{src: r, mtu: mtu}
}

// ConsumeRTCP reads all the data from an RTCPReader.
func ConsumeRTCP(r RTCPReader) {
	if r == nil {
		return
	}
	var pkts []rtcp.Packet
	for {
		if _, err := r.ReadRTCP(pkts); err != nil {
			return
		}
	}
}
