//+build scream

// Package scream provides interceptors to implement SCReAM congestion control via cgo
package scream

import "github.com/pion/interceptor"

func streamSupportSCReAM(info *interceptor.StreamInfo) bool {
	for _, fb := range info.RTCPFeedback {
		if fb.Type == "ack" && fb.Parameter == "ccfb" {
			return true
		}
	}

	return false
}
