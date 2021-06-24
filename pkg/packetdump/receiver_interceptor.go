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

// ReceiverInterceptor interceptor dumps outgoing RTP packets.
type ReceiverInterceptor struct {
	interceptor.NoOp

	log logging.LeveledLogger

	stream     io.Writer
	rtpFilter  RTPFilterCallback
	rtcpFilter RTCPFilterCallback
}

// NewReceiverInterceptor returns a new ReceiverInterceptor interceptor.
func NewReceiverInterceptor(opts ...ReceiverOption) (*ReceiverInterceptor, error) {
	r := &ReceiverInterceptor{
		stream:     os.Stdout,
		rtpFilter:  func(packet *rtp.Packet) bool { return true },
		rtcpFilter: func(pkt *rtcp.Packet) bool { return true },
		log:        logging.NewDefaultLoggerFactory().NewLogger("packetdump_receiver"),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// BindRemoteStream lets you modify any incoming RTP packets. It is called once for per RemoteStream. The returned method
// will be called once per rtp packet.
func (r *ReceiverInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	return interceptor.RTPReaderFunc(func(bytes []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(bytes, attributes)
		if err != nil {
			return 0, nil, err
		}

		pkt := rtp.Packet{}
		if err = pkt.Unmarshal(bytes[:i]); err != nil {
			return 0, nil, err
		}

		if r.rtpFilter(&pkt) {
			if _, err := fmt.Fprintf(r.stream, "in:  %s\n", pkt); err != nil {
				r.log.Errorf("could not dump RTP packet %v", err)
			}
		}

		return i, attr, nil
	})
}

// BindRTCPReader lets you modify any incoming RTCP packets. It is called once per sender/receiver, however this might
// change in the future. The returned method will be called once per packet batch.
func (r *ReceiverInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return interceptor.RTCPReaderFunc(func(bytes []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
		i, attr, err := reader.Read(bytes, attributes)
		if err != nil {
			return 0, nil, err
		}

		pkts, err := rtcp.Unmarshal(bytes[:i])
		if err != nil {
			return 0, nil, err
		}

		for ndx := range pkts {
			if r.rtcpFilter(&pkts[ndx]) {
				if _, printErr := fmt.Fprintf(r.stream, "in:  %s\n", pkts[ndx]); printErr != nil {
					r.log.Errorf("could not dump RTCP packet %v", printErr)
				}
			}
		}

		return i, attr, err
	})
}
