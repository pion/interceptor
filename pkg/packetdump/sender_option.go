package packetdump

import (
	"io"

	"github.com/pion/logging"
)

// SenderOption can be used to configure SenderInterceptor
type SenderOption func(s *SenderInterceptor) error

// SenderLog sets a logger for the interceptor
func SenderLog(log logging.LeveledLogger) SenderOption {
	return func(r *SenderInterceptor) error {
		r.log = log
		return nil
	}
}

// SenderWriter sets the io.Writer on which packets will be dumped.
func SenderWriter(w io.Writer) SenderOption {
	return func(r *SenderInterceptor) error {
		r.stream = w
		return nil
	}
}

// SenderRTPFilter sets the RTP filter.
func SenderRTPFilter(callback RTPFilterCallback) SenderOption {
	return func(r *SenderInterceptor) error {
		r.rtpFilter = callback
		return nil
	}
}

// SenderRTCPFilter sets the RTCP filter.
func SenderRTCPFilter(callback RTCPFilterCallback) SenderOption {
	return func(r *SenderInterceptor) error {
		r.rtcpFilter = callback
		return nil
	}
}
