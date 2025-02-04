package bwe

import (
	"math"
	"time"
)

type rateController struct {
	s    state
	rate int

	decreaseFactor float64 // (beta)
	lastUpdate     time.Time
	lastDecrease   *exponentialMovingAverage
}

func newRateController(initialRate int) *rateController {
	return &rateController{
		s:              stateIncrease,
		rate:           initialRate,
		decreaseFactor: 0.85,
		lastUpdate:     time.Time{},
		lastDecrease:   &exponentialMovingAverage{},
	}
}

func (c *rateController) update(ts time.Time, u usage, deliveredRate int, rtt time.Duration) int {
	nextState := c.s.transition(u)
	c.s = nextState

	if c.s == stateIncrease {
		var target float64
		if c.canIncreaseMultiplicatively(float64(deliveredRate)) {
			window := ts.Sub(c.lastUpdate)
			target = c.multiplicativeIncrease(float64(c.rate), window)
		} else {
			bitsPerFrame := float64(c.rate) / 30.0
			packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
			expectedPacketSizeBits := bitsPerFrame / packetsPerFrame
			target = c.additiveIncrease(float64(c.rate), int(expectedPacketSizeBits), rtt)
		}
		c.rate = int(max(min(target, 1.5*float64(deliveredRate)), float64(c.rate)))
	}

	if c.s == stateDecrease {
		c.rate = int(c.decreaseFactor * float64(deliveredRate))
		c.lastDecrease.update(float64(c.rate))
	}

	c.lastUpdate = ts

	return c.rate
}

func (c *rateController) canIncreaseMultiplicatively(deliveredRate float64) bool {
	if c.lastDecrease.average == 0 {
		return true
	}
	stdDev := math.Sqrt(c.lastDecrease.variance)
	lower := c.lastDecrease.average - 3*stdDev
	upper := c.lastDecrease.average + 3*stdDev
	return deliveredRate < lower || deliveredRate > upper
}

func (c *rateController) multiplicativeIncrease(rate float64, window time.Duration) float64 {
	exponent := min(window.Seconds(), 1.0)
	eta := math.Pow(1.08, exponent)
	target := eta * rate
	return target
}

func (c *rateController) additiveIncrease(rate float64, expectedPacketSizeBits int, window time.Duration) float64 {
	alpha := 0.5 * min(window.Seconds(), 1.0)
	target := rate + max(1000, alpha*float64(expectedPacketSizeBits))
	return target
}
