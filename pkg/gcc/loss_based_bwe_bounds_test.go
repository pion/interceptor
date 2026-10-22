// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

// The loss-based controller must respect a caller-supplied minimum
// bitrate under sustained loss instead of decaying to a hardcoded
// 100 kbps floor. Since the combined SendSideBWE estimate is
// min(delayEstimate, lossEstimate), a floor violation here drags the
// whole estimate below the configured SendSideBWEMinBitrate.
func TestLossBasedBWERespectsConfiguredMinBitrate(t *testing.T) {
	const (
		initialBitrate = 2_500_000
		minimumBitrate = 500_000
		maximumBitrate = 100_000_000
	)
	estimator := newLossBasedBWE(
		initialBitrate, minimumBitrate, maximumBitrate, logging.NewDefaultLoggerFactory(),
	)

	// Acknowledgments with a zero Arrival time count as lost packets.
	lost := make([]cc.Acknowledgment, 10)

	for i := 0; i < 200; i++ {
		// Rewind the rate-limit windows so every iteration is allowed
		// to apply a decrease, simulating a long stretch of loss.
		estimator.lock.Lock()
		estimator.lastDecrease = time.Now().Add(-time.Second)
		estimator.lastLossUpdate = time.Now().Add(-time.Second)
		estimator.lock.Unlock()
		estimator.updateLossEstimate(lost)
	}

	got := estimator.getEstimate(maximumBitrate).TargetBitrate
	assert.GreaterOrEqual(t, got, minimumBitrate,
		"loss controller decayed below the configured minimum")
	assert.Less(t, got, initialBitrate,
		"sustained loss should have decreased the estimate")
}

// The configured maximum must bound the increase path the same way.
func TestLossBasedBWERespectsConfiguredMaxBitrate(t *testing.T) {
	const (
		initialBitrate = 9_000_000
		minimumBitrate = 500_000
		maximumBitrate = 10_000_000
	)
	estimator := newLossBasedBWE(
		initialBitrate, minimumBitrate, maximumBitrate, logging.NewDefaultLoggerFactory(),
	)

	clean := make([]cc.Acknowledgment, 10)
	for i := range clean {
		clean[i].Arrival = time.Now()
	}

	for i := 0; i < 200; i++ {
		estimator.lock.Lock()
		estimator.lastIncrease = time.Now().Add(-time.Second)
		estimator.lastLossUpdate = time.Now().Add(-time.Second)
		estimator.lock.Unlock()
		estimator.updateLossEstimate(clean)
	}

	got := estimator.getEstimate(maximumBitrate * 2).TargetBitrate
	assert.LessOrEqual(t, got, maximumBitrate,
		"loss controller grew above the configured maximum")
	assert.Greater(t, got, initialBitrate,
		"clean feedback should have increased the estimate")
}
