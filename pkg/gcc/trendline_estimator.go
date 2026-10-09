// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"math"
	"time"
)

// Trendline delay-based overuse detection as in libwebrtc's
// TrendlineEstimator (and pion/bwe): the slope of the smoothed accumulated
// inter-group delay over a window of groups, scaled by the number of
// deltas and a gain, against an adaptive threshold. It replaces a Kalman
// filter whose offset estimate was scaled the same way, but without
// libwebrtc's adaptive measurement noise, which made a single
// grouping-sized delay sample read as overuse.
const (
	trendlineWindowSize      = 20
	trendlineSmoothingCoeff  = 0.9
	trendlineThresholdGain   = 4.0
	trendlineMinNumDeltas    = 60
	trendlineDeltaCounterMax = 1000
	initialDelayThreshold    = 12.5 // ms
	minDelayThreshold        = 6.0  // ms
	maxDelayThreshold        = 600.0
	maxAdaptOffset           = 15.0 // ms
	thresholdKUp             = 0.0087
	thresholdKDown           = 0.039
	overusingTimeThreshold   = 10 * time.Millisecond
	maxThresholdUpdateDelta  = 100 * time.Millisecond
)

type trendlinePoint struct {
	arrivalMS       float64
	smoothedDelayMS float64
}

type trendlineEstimator struct {
	dsWriter func(DelayStats)

	init      bool
	prevGroup arrivalGroup

	numDeltas        int
	firstArrival     time.Time
	accumulatedDelay float64 // ms
	smoothedDelay    float64 // ms
	history          []trendlinePoint

	prevTrend      float64
	overusing      bool
	timeOverusing  time.Duration
	overuseCounter int
	usage          usage

	threshold        float64 // ms
	lastThresholdAdj time.Time
}

func newTrendlineEstimator(dsw func(DelayStats)) *trendlineEstimator {
	return &trendlineEstimator{
		dsWriter:  dsw,
		history:   make([]trendlinePoint, 0, trendlineWindowSize+1),
		usage:     usageNormal,
		threshold: initialDelayThreshold,
	}
}

func (e *trendlineEstimator) onArrivalGroup(ag arrivalGroup) {
	if !e.init {
		e.prevGroup = ag
		e.init = true

		return
	}
	sendDelta := ag.departure.Sub(e.prevGroup.departure)
	recvDelta := ag.arrival.Sub(e.prevGroup.arrival)
	e.prevGroup = ag
	if recvDelta < 0 {
		// The groups were reordered after their arrival was stamped.
		return
	}
	delay := recvDelta - sendDelta
	modifiedTrend := e.update(durationToMs(delay), ag.arrival, sendDelta)

	e.dsWriter(DelayStats{
		Measurement:      delay,
		Estimate:         msToDuration(modifiedTrend),
		Threshold:        msToDuration(e.threshold),
		LastReceiveDelta: recvDelta,
		Usage:            e.usage,
		State:            0,
		TargetBitrate:    0,
	})
}

// update feeds one inter-group delay (ms) and returns the modified trend
// the detector compared against its threshold.
func (e *trendlineEstimator) update(delayMS float64, arrival time.Time, sendDelta time.Duration) float64 {
	e.numDeltas = min(e.numDeltas+1, trendlineDeltaCounterMax)
	if e.firstArrival.IsZero() {
		e.firstArrival = arrival
	}
	e.accumulatedDelay += delayMS
	e.smoothedDelay = trendlineSmoothingCoeff*e.smoothedDelay +
		(1-trendlineSmoothingCoeff)*e.accumulatedDelay

	e.history = append(e.history, trendlinePoint{
		arrivalMS:       durationToMs(arrival.Sub(e.firstArrival)),
		smoothedDelayMS: e.smoothedDelay,
	})
	if len(e.history) > trendlineWindowSize {
		e.history = e.history[1:]
	}

	trend := e.prevTrend
	if len(e.history) == trendlineWindowSize {
		if slope, ok := linearFitSlope(e.history); ok {
			trend = slope
		}
	}

	return e.detect(trend, sendDelta, arrival)
}

func (e *trendlineEstimator) detect(trend float64, sendDelta time.Duration, arrival time.Time) float64 {
	if e.numDeltas < 2 {
		e.usage = usageNormal

		return 0
	}
	modifiedTrend := float64(min(e.numDeltas, trendlineMinNumDeltas)) * trend * trendlineThresholdGain

	switch {
	case modifiedTrend > e.threshold:
		if e.overusing {
			e.timeOverusing += sendDelta
		} else {
			// Initialize with half a delta: the overuse began somewhere
			// within it.
			e.timeOverusing = sendDelta / 2
			e.overusing = true
		}
		e.overuseCounter++
		if e.timeOverusing > overusingTimeThreshold && e.overuseCounter > 1 && trend >= e.prevTrend {
			e.timeOverusing = 0
			e.overuseCounter = 0
			e.usage = usageOver
		}
	case modifiedTrend < -e.threshold:
		e.overusing = false
		e.overuseCounter = 0
		e.usage = usageUnder
	default:
		e.overusing = false
		e.overuseCounter = 0
		e.usage = usageNormal
	}
	e.prevTrend = trend
	e.updateThreshold(modifiedTrend, arrival)

	return modifiedTrend
}

func (e *trendlineEstimator) updateThreshold(modifiedTrend float64, arrival time.Time) {
	if e.lastThresholdAdj.IsZero() {
		e.lastThresholdAdj = arrival
	}
	if math.Abs(modifiedTrend) > e.threshold+maxAdaptOffset {
		// Do not adapt to big latency spikes, e.g. a sudden capacity drop.
		e.lastThresholdAdj = arrival

		return
	}
	k := thresholdKUp
	if math.Abs(modifiedTrend) < e.threshold {
		k = thresholdKDown
	}
	delta := min(arrival.Sub(e.lastThresholdAdj), maxThresholdUpdateDelta)
	e.threshold += k * (math.Abs(modifiedTrend) - e.threshold) * durationToMs(delta)
	e.threshold = min(max(e.threshold, minDelayThreshold), maxDelayThreshold)
	e.lastThresholdAdj = arrival
}

// linearFitSlope is the least-squares slope of smoothed delay over arrival
// time; false when the arrivals do not span any time.
func linearFitSlope(points []trendlinePoint) (float64, bool) {
	if len(points) < 2 {
		return 0, false
	}
	var sumX, sumY float64
	for _, p := range points {
		sumX += p.arrivalMS
		sumY += p.smoothedDelayMS
	}
	avgX := sumX / float64(len(points))
	avgY := sumY / float64(len(points))
	var numerator, denominator float64
	for _, p := range points {
		numerator += (p.arrivalMS - avgX) * (p.smoothedDelayMS - avgY)
		denominator += (p.arrivalMS - avgX) * (p.arrivalMS - avgX)
	}
	if denominator == 0 {
		return 0, false
	}

	return numerator / denominator, true
}

func durationToMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func msToDuration(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}
