// Package gcc implements Google Congestion Control for bandwidth estimation
package gcc

import "time"

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(b, min, max int) int {
	if min < b && b < max {
		return b
	}
	if b < min {
		return min
	}
	return max
}

func clampDuration(d, min, max time.Duration) time.Duration {
	if min <= d && d <= max {
		return d
	}
	if d <= min {
		return min
	}
	return max
}
