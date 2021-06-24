// Package pli provides interceptors to implement Picture Loss Indication mechanism.
package pli

import "github.com/pion/interceptor"

func streamSupportPli(info *interceptor.StreamInfo) bool {
	for _, fb := range info.RTCPFeedback {
		if fb.Type == "nack" && fb.Parameter == "pli" {
			return true
		}
	}

	return false
}
