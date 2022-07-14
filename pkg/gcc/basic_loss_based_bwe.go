package gcc

import (
	"math"
	"time"
)

const (
	limitNumPackets     = 20
	bweDecreaseInterval = 300 * time.Millisecond
	bweIncreaseInterval = 1000 * time.Millisecond
	startPhase          = 2 * time.Second
)

type minHistoryEntry struct {
	ts   time.Time
	rate int
}

type basicLossBasedBWE struct {
	// config
	maxRTCPFeedbackInterval time.Duration
	bitrateThreshold        int
	lowLossThreshold        float64
	highLossThreshold       float64
	maxBitrate              int
	minBitrate              int

	// state
	currentTarget               int
	delayBasedLimit             int
	minBitrateHistory           []minHistoryEntry
	lastLossReport              time.Time
	firstReportTime             time.Time
	expectedSinceLastUpdate     int
	lostSinceLastUpdate         int
	hasDecreasedSinceLastUpdate bool
	lastFractionLoss            int
	lastDecrease                time.Time
	lastRoundTripTime           time.Duration
}

func newBasicLossBasedBWE(minBitrate, maxBitrate int) *basicLossBasedBWE {
	return &basicLossBasedBWE{
		maxRTCPFeedbackInterval:     time.Second,
		bitrateThreshold:            100_000,
		lowLossThreshold:            0.02,
		highLossThreshold:           0.1,
		maxBitrate:                  maxBitrate,
		minBitrate:                  minBitrate,
		currentTarget:               0,
		delayBasedLimit:             math.MaxInt,
		minBitrateHistory:           []minHistoryEntry{},
		lastLossReport:              time.Time{},
		firstReportTime:             time.Time{},
		expectedSinceLastUpdate:     0,
		lostSinceLastUpdate:         0,
		hasDecreasedSinceLastUpdate: false,
		lastFractionLoss:            0,
		lastDecrease:                time.Time{},
		lastRoundTripTime:           0,
	}
}

func (e *basicLossBasedBWE) updateRTT(rtt time.Duration) {
	e.lastRoundTripTime = rtt
}

func (e *basicLossBasedBWE) updateLoss(now time.Time, packetsLost int, numPackets int) {
	if e.firstReportTime.IsZero() {
		e.firstReportTime = now
	}

	if numPackets > 0 {
		expected := e.expectedSinceLastUpdate + numPackets
		if expected < limitNumPackets {
			e.expectedSinceLastUpdate = expected
			e.lostSinceLastUpdate += packetsLost
			return
		}

		e.hasDecreasedSinceLastUpdate = false
		lostQ8 := (e.lostSinceLastUpdate + packetsLost) << 8
		e.lastFractionLoss = minInt(lostQ8/expected, 255)

		e.lostSinceLastUpdate = 0
		e.expectedSinceLastUpdate = 0
		e.lastLossReport = now

		e.updateEstimate(now)
	}
}

func (e *basicLossBasedBWE) isInStartPhase(now time.Time) bool {
	return e.firstReportTime.IsZero() || now.Sub(e.firstReportTime) < startPhase
}

func (e *basicLossBasedBWE) updateDelayBasedLimit(limit int) {
	e.delayBasedLimit = limit
}

func (e *basicLossBasedBWE) updateEstimate(now time.Time) {
	if e.lastFractionLoss == 0 && e.isInStartPhase(now) {
		newBitrate := e.currentTarget
		if e.delayBasedLimit < math.MaxInt {
			newBitrate = maxInt(e.delayBasedLimit, newBitrate)
		}

		if newBitrate != e.currentTarget {
			e.minBitrateHistory = []minHistoryEntry{}
			e.updateTargetBitrate(newBitrate)
			return
		}
	}

	e.updateMinHistory(now)

	if e.lastLossReport.IsZero() {
		return
	}

	timeSinceLastReport := now.Sub(e.lastLossReport)
	if timeSinceLastReport < time.Duration(1.2*float64(e.maxRTCPFeedbackInterval)) {
		loss := float64(e.lastFractionLoss) / 256.0
		if e.currentTarget < e.bitrateThreshold || loss < e.lowLossThreshold {
			newBitrate := int(float64(e.minBitrateHistory[0].rate)*1.08 + 0.5)
			newBitrate += 1000
			e.updateTargetBitrate(newBitrate)
			return
		} else if e.currentTarget > e.bitrateThreshold {
			if loss > e.highLossThreshold {
				if !e.hasDecreasedSinceLastUpdate &&
					now.Sub(e.lastDecrease) >= bweDecreaseInterval+e.lastRoundTripTime {
					e.lastDecrease = now
					newBitrate := int(float64(e.currentTarget) * (float64(512-e.lastFractionLoss) / 512.0))
					e.hasDecreasedSinceLastUpdate = true
					e.updateTargetBitrate(newBitrate)
					return
				}
			}
		}
	}
}

func (e *basicLossBasedBWE) updateTargetBitrate(newBitrate int) {
	newBitrate = minInt(newBitrate, e.delayBasedLimit)
	if newBitrate < e.minBitrate {
		newBitrate = e.minBitrate
	}
	e.currentTarget = newBitrate
}

func (e *basicLossBasedBWE) updateMinHistory(now time.Time) {
	for len(e.minBitrateHistory) > 0 && now.Sub(e.minBitrateHistory[0].ts)+time.Millisecond > bweIncreaseInterval {
		e.minBitrateHistory = e.minBitrateHistory[1:]
	}
	for len(e.minBitrateHistory) > 0 && e.currentTarget <= e.minBitrateHistory[len(e.minBitrateHistory)-1].rate {
		e.minBitrateHistory = e.minBitrateHistory[:len(e.minBitrateHistory)-1]
	}
	e.minBitrateHistory = append(e.minBitrateHistory, minHistoryEntry{
		ts:   now,
		rate: e.currentTarget,
	})
}
