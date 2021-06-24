package packetdump

import (
	"io"

	"github.com/pion/logging"
)

// ReceiverOption can be used to configure ReceiverInterceptor.
type ReceiverOption func(r *ReceiverInterceptor) error

// ReceiverLog sets a logger for the interceptor.
func ReceiverLog(log logging.LeveledLogger) ReceiverOption {
	return func(r *ReceiverInterceptor) error {
		r.log = log
		return nil
	}
}

// ReceiverWriter sets the io.Writer on which packets will be dumped.
func ReceiverWriter(w io.Writer) ReceiverOption {
	return func(r *ReceiverInterceptor) error {
		r.stream = w
		return nil
	}
}

// ReceiverRTPFilter sets the RTP filter.
func ReceiverRTPFilter(callback RTPFilterCallback) ReceiverOption {
	return func(r *ReceiverInterceptor) error {
		r.rtpFilter = callback
		return nil
	}
}

// ReceiverRTCPFilter sets the RTCP filter.
func ReceiverRTCPFilter(callback RTCPFilterCallback) ReceiverOption {
	return func(r *ReceiverInterceptor) error {
		r.rtcpFilter = callback
		return nil
	}
}
