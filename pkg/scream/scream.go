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

func getNTPT0() float64 {
	now := time.Now()
	secs := now.Unix()
	usecs := now.UnixMicro() - secs*1e6
	return (float64(secs) + float64(usecs)*1e-6) - 1e-3
}

func getTimeBetweenNTP(t0 float64, tx time.Time) uint64 {
	secs := tx.Unix()
	usecs := tx.UnixMicro() - secs*1e6
	tt := (float64(secs) + float64(usecs)*1e-6) - t0
	ntp64 := uint64(tt * 65536.0)
	ntp := 0xFFFFFFFF & ntp64
	return ntp
}
