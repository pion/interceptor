// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"sync"
	"time"
)

const (
	decreaseEMAAlpha = 0.95
	beta             = 0.85
)

type rateController struct {
	now                  now
	initialTargetBitrate int
	minBitrate           int
	maxBitrate           int

	dsWriter func(DelayStats)

	lock               sync.Mutex
	init               bool
	delayStats         DelayStats
	target             int
	lastUpdate         time.Time
	lastState          state
	latestRTT          time.Duration
	latestReceivedRate int
	latestDecreaseRate *exponentialMovingAverage
}

type exponentialMovingAverage struct {
	average      float64
	variance     float64
	stdDeviation float64
}

func (a *exponentialMovingAverage) update(value float64) {
	if a.average == 0.0 {
		a.average = value
	} else {
		x := value - a.average
		a.average += decreaseEMAAlpha * x
		a.variance = (1 - decreaseEMAAlpha) * (a.variance + decreaseEMAAlpha*x*x)
		a.stdDeviation = math.Sqrt(a.variance)
	}
}

func newRateController(now now, initialTargetBitrate, minBitrate, maxBitrate int, dsw func(DelayStats)) *rateController {
	return &rateController{
		now:                  now,
		initialTargetBitrate: initialTargetBitrate,
		minBitrate:           minBitrate,
		maxBitrate:           maxBitrate,
		dsWriter:             dsw,
		init:                 false,
		delayStats:           DelayStats{},
		target:               initialTargetBitrate,
		lastUpdate:           time.Time{},
		lastState:            stateIncrease,
		latestRTT:            0,
		latestReceivedRate:   0,
		latestDecreaseRate:   &exponentialMovingAverage{},
	}
}

func (c *rateController) onReceivedRate(rate int) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.latestReceivedRate = rate
}

func (c *rateController) updateRTT(rtt time.Duration) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.latestRTT = rtt
}

func (c *rateController) onDelayStats(ds DelayStats) {
	now := time.Now()

	if !c.init {
		c.delayStats = ds
		c.delayStats.State = stateIncrease
		c.init = true
		return
	}
	c.delayStats = ds
	c.delayStats.State = c.delayStats.State.transition(ds.Usage)

	if c.delayStats.State == stateHold {
		return
	}

	var next DelayStats

	c.lock.Lock()

	switch c.delayStats.State {
	case stateHold:
		// should never occur due to check above, but makes the linter happy
	case stateIncrease:
		c.target = clampInt(c.increase(now), c.minBitrate, c.maxBitrate)
		next = DelayStats{
			Measurement:      c.delayStats.Measurement,
			Estimate:         c.delayStats.Estimate,
			Threshold:        c.delayStats.Threshold,
			LastReceiveDelta: c.delayStats.LastReceiveDelta,
			Usage:            c.delayStats.Usage,
			State:            c.delayStats.State,
			TargetBitrate:    c.target,
		}

	case stateDecrease:
		c.target = clampInt(c.decrease(), c.minBitrate, c.maxBitrate)
		next = DelayStats{
			Measurement:      c.delayStats.Measurement,
			Estimate:         c.delayStats.Estimate,
			Threshold:        c.delayStats.Threshold,
			LastReceiveDelta: c.delayStats.LastReceiveDelta,
			Usage:            c.delayStats.Usage,
			State:            c.delayStats.State,
			TargetBitrate:    c.target,
		}
	}

	c.lock.Unlock()

	c.dsWriter(next)
}

func (c *rateController) increase(now time.Time) int {
	if c.latestDecreaseRate.average > 0 && float64(c.latestReceivedRate) > c.latestDecreaseRate.average-3*c.latestDecreaseRate.stdDeviation &&
		float64(c.latestReceivedRate) < c.latestDecreaseRate.average+3*c.latestDecreaseRate.stdDeviation {
		bitsPerFrame := float64(c.target) / 30.0
		packetsPerFrame := math.Ceil(bitsPerFrame / (1200 * 8))
		expectedPacketSizeBits := bitsPerFrame / packetsPerFrame

		responseTime := 100*time.Millisecond + c.latestRTT
		alpha := 0.5 * math.Min(float64(now.Sub(c.lastUpdate).Milliseconds())/float64(responseTime.Milliseconds()), 1.0)
		increase := int(math.Max(1000.0, alpha*expectedPacketSizeBits))
		c.lastUpdate = now
		return int(math.Min(float64(c.target+increase), 1.5*float64(c.latestReceivedRate)))
	}
	eta := math.Pow(1.08, math.Min(float64(now.Sub(c.lastUpdate).Milliseconds())/1000, 1.0))
	c.lastUpdate = now

	rate := int(eta * float64(c.target))

	// maximum increase to 1.5 * received rate
	received := int(1.5 * float64(c.latestReceivedRate))
	if rate > received && received > c.target {
		return received
	}

	if rate < c.target {
		return c.target
	}
	return rate
}

func (c *rateController) decrease() int {
	target := int(beta * float64(c.latestReceivedRate))
	c.latestDecreaseRate.update(float64(c.latestReceivedRate))
	c.lastUpdate = c.now()
	return target
}
