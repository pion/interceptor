package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBasicLossBasedBWE(t *testing.T) {
	t.Run("minBitrate", func(t *testing.T) {
		now := time.Now()
		minBitrate, maxBitrate := 100_000, 1_000_000
		bwe := newBasicLossBasedBWE(minBitrate, maxBitrate)
		// Before any update
		assert.Equal(t, 0, bwe.currentTarget)

		// Update with 0 loss
		bwe.updateLoss(now, 0, 100)

		// After first update equals at least minBitrate
		assert.GreaterOrEqual(t, bwe.currentTarget, minBitrate)
	})

	t.Run("useDelayLimitDuringStartup", func(t *testing.T) {
		now := time.Now()
		minBitrate, maxBitrate := 100_000, 1_000_000
		bwe := newBasicLossBasedBWE(minBitrate, maxBitrate)
		// Before any update
		assert.Equal(t, 0, bwe.currentTarget)

		bwe.updateDelayBasedLimit(500_000)

		bwe.updateEstimate(now)

		assert.Equal(t, 500_000, bwe.currentTarget)
	})

	t.Run("nowEstimateUpdateBeforeLossReport", func(t *testing.T) {
		now := time.Now()
		minBitrate, maxBitrate := 100_000, 1_000_000
		bwe := newBasicLossBasedBWE(minBitrate, maxBitrate)
		// Before any update
		assert.Equal(t, 0, bwe.currentTarget)

		// Update after startup phase
		bwe.updateEstimate(now.Add(3 * time.Second))

		assert.Equal(t, 0, bwe.currentTarget)
	})

	t.Run("increaseOnLowLoss", func(t *testing.T) {
		now := time.Now()
		minBitrate, maxBitrate := 100_000, 1_000_000
		bwe := newBasicLossBasedBWE(minBitrate, maxBitrate)
		// Before any update
		assert.Equal(t, 0, bwe.currentTarget)

		bwe.updateDelayBasedLimit(100_000)
		bwe.updateEstimate(now)

		bwe.updateLoss(now, 0, 100)

		assert.Equal(t, 100_000, bwe.currentTarget)

		bwe.updateLoss(now.Add(3000*time.Millisecond), 1, 100)

		// increase delay based limit (upper bound of loss based limit)
		bwe.updateDelayBasedLimit(1_000_000)
		bwe.updateLoss(now.Add(3100*time.Millisecond), 1, 100)

		assert.Equal(t, 109_000, bwe.currentTarget)
	})

	t.Run("decreaseOnHighLoss", func(t *testing.T) {
		now := time.Now()
		minBitrate, maxBitrate := 100_000, 1_000_000
		bwe := newBasicLossBasedBWE(minBitrate, maxBitrate)
		// Before any update
		assert.Equal(t, 0, bwe.currentTarget)

		bwe.updateDelayBasedLimit(200_000)
		bwe.updateEstimate(now)

		bwe.updateLoss(now, 0, 100)

		assert.Equal(t, 200_000, bwe.currentTarget)

		bwe.updateLoss(now.Add(3000*time.Millisecond), 15, 100)

		// increase delay based limit (upper bound of loss based limit)
		bwe.updateDelayBasedLimit(1_000_000)
		bwe.updateLoss(now.Add(3100*time.Millisecond), 15, 100)

		// Allow range of 0.1 percent to account for rounding
		assert.InDelta(t, 185_000, bwe.currentTarget, 0.001*185_000)
	})
}
