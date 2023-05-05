// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package intervalpli is an interceptor that requests PLI on a static interval. Useful when bridging protocols that don't have receiver feedback
package intervalpli

import "github.com/pion/interceptor"

func streamSupportPli(info *interceptor.StreamInfo) bool {
	for _, fb := range info.RTCPFeedback {
		if fb.Type == "nack" && fb.Parameter == "pli" {
			return true
		}
	}

	return false
}
