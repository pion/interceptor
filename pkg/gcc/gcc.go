// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package gcc implements Google Congestion Control for bandwidth estimation
package gcc

import "time"

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(b, min, max int) int {
	return maxInt(min, minInt(max, b))
}

func clampDuration(d, min, max time.Duration) time.Duration {
	return time.Duration(clampInt(int(d), int(min), int(max)))
}
