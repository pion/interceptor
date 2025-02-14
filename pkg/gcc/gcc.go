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

func clampInt(b, minVal, maxVal int) int {
	return maxInt(minVal, minInt(maxVal, b))
}

func clampDuration(d, minVal, maxVal time.Duration) time.Duration {
	return time.Duration(clampInt(int(d), int(minVal), int(maxVal)))
}
