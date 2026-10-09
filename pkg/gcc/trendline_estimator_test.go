// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

// feedGroups sends one single-packet group every 20 ms; extraDelay(i) is
// group i's queueing delay in ms. Returns every DelayStats.
func feedGroups(n int, extraDelay func(i int) float64) []DelayStats {
	var stats []DelayStats
	e := newTrendlineEstimator(func(ds DelayStats) { stats = append(stats, ds) })
	for i := range n {
		dep := groupEpoch().Add(time.Duration(i) * 20 * time.Millisecond)
		arr := dep.Add(20*time.Millisecond + msToDuration(extraDelay(i)))
		e.onArrivalGroup(newArrivalGroup(cc.Acknowledgment{Departure: dep, Arrival: arr}))
	}

	return stats
}

func usages(stats []DelayStats) map[usage]int {
	m := map[usage]int{}
	for _, ds := range stats {
		m[ds.Usage]++
	}

	return m
}

func TestLinearFitSlope(t *testing.T) {
	slope, ok := linearFitSlope([]trendlinePoint{{0, 1}, {10, 2}, {20, 3}})
	assert.True(t, ok)
	assert.InDelta(t, 0.1, slope, 1e-9)

	_, ok = linearFitSlope([]trendlinePoint{{5, 1}, {5, 2}})
	assert.False(t, ok, "no arrival spread")
	_, ok = linearFitSlope([]trendlinePoint{{5, 1}})
	assert.False(t, ok, "a single point")
}

func TestTrendlineConstantDelayIsNormal(t *testing.T) {
	stats := feedGroups(200, func(int) float64 { return 0 })

	assert.Len(t, stats, 199, "one sample per group after the first")
	assert.Equal(t, map[usage]int{usageNormal: 199}, usages(stats))
}

// A queue that grows 2 ms per 20 ms group (sending 10% over capacity) is
// overuse.
func TestTrendlineGrowingDelayIsOveruse(t *testing.T) {
	stats := feedGroups(100, func(i int) float64 {
		return float64(max(0, i-40)) * 2
	})

	assert.Positive(t, usages(stats)[usageOver])
	for _, ds := range stats[:40] {
		assert.Equal(t, usageNormal, ds.Usage, "before the queue builds")
	}
}

// A draining queue is underuse.
func TestTrendlineShrinkingDelayIsUnderuse(t *testing.T) {
	stats := feedGroups(100, func(i int) float64 {
		return float64(max(0, 100-i)) * 2
	})

	assert.Positive(t, usages(stats)[usageUnder])
	assert.Zero(t, usages(stats)[usageOver])
}

// One delayed group (a 10 ms hiccup that recovers at once) is jitter, not
// congestion: the old Kalman detector cut the target on exactly this.
func TestTrendlineSingleSpikeIsNotOveruse(t *testing.T) {
	stats := feedGroups(200, func(i int) float64 {
		if i == 120 {
			return 10
		}

		return 0
	})

	assert.Zero(t, usages(stats)[usageOver])
}

// A one-off step in the delay level (e.g. a route change) is not sustained
// growth either.
func TestTrendlineDelayStepIsNotOveruse(t *testing.T) {
	stats := feedGroups(200, func(i int) float64 {
		if i >= 120 {
			return 10
		}

		return 0
	})

	assert.Zero(t, usages(stats)[usageOver])
}

// Before the window holds trendlineWindowSize deltas there is no trend.
func TestTrendlineNeedsAFullWindow(t *testing.T) {
	stats := feedGroups(trendlineWindowSize, func(i int) float64 {
		return float64(i) * 5
	})

	for _, ds := range stats {
		assert.Equal(t, usageNormal, ds.Usage)
		assert.Zero(t, ds.Estimate)
	}
}

func TestTrendlineThresholdStaysClamped(t *testing.T) {
	calm := feedGroups(2000, func(int) float64 { return 0 })
	assert.InDelta(t, minDelayThreshold, durationToMs(calm[len(calm)-1].Threshold), 1e-9,
		"a calm path lowers the threshold to its floor")

	noisy := feedGroups(2000, func(i int) float64 { return float64(i%5) * 6 })
	for _, ds := range noisy {
		ms := durationToMs(ds.Threshold)
		assert.GreaterOrEqual(t, ms, minDelayThreshold)
		assert.LessOrEqual(t, ms, maxDelayThreshold)
	}
}

// Groups reordered after their arrival was stamped yield no sample.
func TestTrendlineSkipsReorderedGroups(t *testing.T) {
	var stats []DelayStats
	e := newTrendlineEstimator(func(ds DelayStats) { stats = append(stats, ds) })
	e.onArrivalGroup(newArrivalGroup(ackAt(1, 0, 50)))
	e.onArrivalGroup(newArrivalGroup(ackAt(2, 20, 40)))
	e.onArrivalGroup(newArrivalGroup(ackAt(3, 40, 60)))

	assert.Len(t, stats, 1)
	assert.Equal(t, 20*time.Millisecond, stats[0].LastReceiveDelta)
}

func BenchmarkTrendlineEstimator(b *testing.B) {
	est := newTrendlineEstimator(func(DelayStats) {})
	groups := make([]arrivalGroup, 1024)
	for i := range groups {
		dep := groupEpoch().Add(time.Duration(i) * 20 * time.Millisecond)
		groups[i] = newArrivalGroup(cc.Acknowledgment{
			Departure: dep,
			Arrival:   dep.Add(20*time.Millisecond + time.Duration(i%7)*100*time.Microsecond),
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := groups[i%len(groups)]
		// keep time moving forward across wraps
		shift := time.Duration(i/len(groups)) * time.Duration(len(groups)) * 20 * time.Millisecond
		g.departure = g.departure.Add(shift)
		g.arrival = g.arrival.Add(shift)
		est.onArrivalGroup(g)
	}
}
