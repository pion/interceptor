package rtpio

import (
	"io"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type rtpReader struct {
	src chan []byte
}

// ReadRTP reads an RTP packet from the underlying `io.Reader`.
func (r rtpReader) ReadRTP(pkt *rtp.Packet) (int, error) {
	buf := <-r.src
	if err := pkt.Unmarshal(buf); err != nil {
		return 0, err
	}
	return len(buf), nil
}

type rtcpReader struct {
	src chan []byte
}

// ReadRTCP reads an RTCP packet slice from the underlying `io.Reader`.
func (r rtcpReader) ReadRTCP(pkts []rtcp.Packet) (int, error) {
	buf := <-r.src
	p, err := rtcp.Unmarshal(buf)
	if err != nil {
		return 0, err
	}
	copy(pkts, p)
	return len(buf), nil
}

// NewRTPRTCPDemultiplexer creates a new RFC 5761 demultiplexer.
func NewRTPRTCPDemultiplexer(r io.Reader, mtu int) (RTPReader, RTCPReader) {
	// it's ok that these are unbuffered since our API is pull-based.
	rtpCh := make(chan []byte)
	rtcpCh := make(chan []byte)
	go func() {
		defer close(rtpCh)
		defer close(rtcpCh)
		for {
			buf := make([]byte, mtu)
			n, err := r.Read(buf)
			if err != nil {
				return
			}
			h := &rtcp.Header{}
			if err := h.Unmarshal(buf[:n]); err != nil {
				// not a valid rtp/rtcp packet.
				continue
			}
			if h.Type >= 200 && h.Type <= 207 {
				// it's an rtcp packet.
				rtcpCh <- buf[:n]
			} else {
				// it's an rtp packet.
				rtpCh <- buf[:n]
			}
		}
	}()
	return rtpReader{src: rtpCh}, rtcpReader{src: rtcpCh}
}
