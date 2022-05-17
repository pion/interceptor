// Package scream provides interceptors to implement SCReAM congestion control via cgo
package scream

import (
	"time"

	"github.com/pion/interceptor"
)

func streamSupportSCReAM(info *interceptor.StreamInfo) bool {
	for _, fb := range info.RTCPFeedback {
		if fb.Type == "ack" && fb.Parameter == "ccfb" {
			return true
		}
	}

	return false
}

func ntpTime32(t time.Time) uint32 {
	// seconds since 1st January 1900
	s := (float64(t.UnixNano()) / 1000000000.0) + 2208988800

	integerPart := uint32(s)
	fractionalPart := uint32((s - float64(integerPart)) * 0xFFFFFFFF)

	// higher 32 bits are the integer part, lower 32 bits are the fractional part
	return uint32(((uint64(integerPart)<<32 | uint64(fractionalPart)) >> 16) & 0xFFFFFFFF)
}
