// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The additive increase derives its step from the expected packet size. Deriving
// that size as bitsPerFrame/ceil(bitsPerFrame/MTU) gives the mean packet size,
// which falls discontinuously whenever the packet count increments, so a higher
// target can be raised in smaller steps than a lower one. A packet is at most one
// MTU, so above one MTU per frame the step should simply be constant.
func TestRateControllerAdditiveIncreaseStepIsMonotonicInTarget(t *testing.T) {
	// At 30 fps with a 1200-byte MTU, 288 kbps is exactly one packet per frame and
	// 290 kbps spills into a second; 576 and 580 kbps straddle the next boundary.
	// alpha is 0.5 below, so the step is half the expected packet size in bits.
	for _, tc := range []struct {
		target   int
		wantStep int
	}{
		{240_000, 4_000}, // under one MTU per frame: half of 8000 bits
		{288_000, 4_800}, // exactly one MTU per frame
		{290_000, 4_800}, // second packet begins; step must not fall
		{300_000, 4_800},
		{480_000, 4_800},
		{576_000, 4_800},
		{580_000, 4_800}, // third packet begins
		{600_000, 4_800},
	} {
		c := newRateController(
			func() time.Time { return time.Time{} },
			tc.target, 1_000, 50_000_000, func(DelayStats) {},
		)
		// Select the near-a-previous-decrease regime that uses the additive branch.
		// The received rate must sit inside average +/- 3*stdDeviation.
		c.latestDecreaseRate.average = float64(tc.target)
		c.latestDecreaseRate.stdDeviation = float64(tc.target) * 0.05
		c.latestReceivedRate = tc.target
		c.latestRTT = 50 * time.Millisecond

		// responseTime is 100ms+RTT, so a 150ms gap gives alpha = 0.5.
		got := c.increase(time.Time{}.Add(150 * time.Millisecond))

		assert.Equal(t, tc.target+tc.wantStep, got,
			"target %d: additive step was %d, want %d", tc.target, got-tc.target, tc.wantStep)
	}
}
