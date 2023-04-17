package playoutdelay

import (
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

// HeaderExtensionInterceptorFactory is a interceptor.Factory for a HeaderExtensionInterceptor
type HeaderExtensionInterceptorFactory struct{}

const (
	playoutDelayMaxMs = 40950
)

// NewInterceptor constructs a new HeaderExtensionInterceptor
func (h *HeaderExtensionInterceptorFactory) NewInterceptor(id string, minDelay, maxDelay time.Duration) (interceptor.Interceptor, error) {
	if minDelay.Milliseconds() < 0 || minDelay.Milliseconds() > playoutDelayMaxMs || maxDelay.Milliseconds() < 0 || maxDelay.Milliseconds() > playoutDelayMaxMs {
		return nil, errPlayoutDelayInvalidValue
	}
	return &HeaderExtensionInterceptor{minDelay: uint16(minDelay.Milliseconds() / 10), maxDelay: uint16(maxDelay.Milliseconds() / 10)}, nil
}

// NewHeaderExtensionInterceptor returns a HeaderExtensionInterceptorFactory
func NewHeaderExtensionInterceptor() (*HeaderExtensionInterceptorFactory, error) {
	return &HeaderExtensionInterceptorFactory{}, nil
}

// HeaderExtensionInterceptor adds transport wide sequence numbers as header extension to each RTP packet
type HeaderExtensionInterceptor struct {
	interceptor.NoOp
	minDelay, maxDelay uint16
}

const playoutDelayURI = "http://www.webrtc.org/experiments/rtp-hdrext/playout-delay"

// BindLocalStream returns a writer that adds a rtp.PlayoutDelayExtension
// header with increasing sequence numbers to each outgoing packet.
func (h *HeaderExtensionInterceptor) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var hdrExtID uint8
	for _, e := range info.RTPHeaderExtensions {
		if e.URI == playoutDelayURI {
			hdrExtID = uint8(e.ID)
			break
		}
	}
	if hdrExtID == 0 { // Don't add header extension if ID is 0, because 0 is an invalid extension ID
		return writer
	}
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		ext, err := (&rtp.PlayoutDelayExtension{
			minDelay: h.minDelay,
			maxDelay: h.maxDelay,
		}).Marshal()
		if err != nil {
			return 0, err
		}
		err = header.SetExtension(hdrExtID, ext)
		if err != nil {
			return 0, err
		}
		return writer.Write(header, payload, attributes)
	})
}
