package rtpio

import (
	"io"
)

// RTPPipe creates a new RTPPipe and returns the reader and writer.
func RTPPipe() (RTPReader, RTPWriter) {
	r, w := io.Pipe()
	return NewRTPReader(r, 1500), NewRTPWriter(w)
}

// RTCPPipe creates a new RTCPPipe and returns the reader and writer.
func RTCPPipe() (RTCPReader, RTCPWriter) {
	r, w := io.Pipe()
	return NewRTCPReader(r, 1500), NewRTCPWriter(w)
}
