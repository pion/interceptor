package packetdump

import (
	"fmt"
	"io"
	"os"

	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// SenderInterceptor responds to nack feedback messages
type SenderInterceptor struct {
	interceptor.NoOp

	log logging.LeveledLogger

	stream     io.Writer
	rtpFilter  RTPFilterCallback
	rtcpFilter RTCPFilterCallback
}

// NewSenderInterceptor returns a new SenderInterceptor interceptor
func NewSenderInterceptor(opts ...SenderOption) (*SenderInterceptor, error) {
	r := &SenderInterceptor{
		stream:     os.Stdout,
		rtpFilter:  func(packet *rtp.Packet) bool { return true },
		rtcpFilter: func(pkt *rtcp.Packet) bool { return true },
		log:        logging.NewDefaultLoggerFactory().NewLogger("packetdump_sender"),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (r *SenderInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	return interceptor.RTCPWriterFunc(func(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
		for ndx := range pkts {
			if r.rtcpFilter(&pkts[ndx]) {
				if _, err := fmt.Fprintf(r.stream, "out: %s\n", pkts[ndx]); err != nil {
					r.log.Errorf("could not dump RTCP packet %v", err)
				}
			}
		}

		return writer.Write(pkts, attributes)
	})
}

// BindLocalStream lets you modify any outgoing RTP packets. It is called once for per LocalStream. The returned method
// will be called once per rtp packet.
func (r *SenderInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		pkt := rtp.Packet{
			Header:  *header,
			Payload: payload,
		}

		if r.rtpFilter(&pkt) {
			if _, err := fmt.Fprintf(r.stream, "out: %s\n", pkt); err != nil {
				r.log.Errorf("could not dump RTP packet %v", err)
			}
		}

		return writer.Write(header, payload, attributes)
	})
}
