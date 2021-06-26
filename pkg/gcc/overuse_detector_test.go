package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type staticThreshold time.Duration

func (t staticThreshold) compare(estimate, _ time.Duration) (usage, time.Duration, time.Duration) {
	if estimate > time.Duration(t) {
		return usageOver, estimate, time.Duration(t)
	}
	if estimate < -time.Duration(t) {
		return usageUnder, estimate, time.Duration(t)
	}
	return usageNormal, estimate, time.Duration(t)
}

func TestOveruseDetectorWithoutDelay(t *testing.T) {
	cases := []struct {
		name      string
		estimates []DelayStats
		expected  []usage
		thresh    threshold
		delay     time.Duration
	}{
		{
			name:      "noEstimateNoUsage",
			estimates: []DelayStats{},
			expected:  []usage{},
			thresh:    staticThreshold(time.Millisecond),
			delay:     0,
		},
		{
			name: "overuse",
			estimates: []DelayStats{
				{},
				{Estimate: 2 * time.Millisecond},
				{Estimate: 3 * time.Millisecond},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
			thresh:   staticThreshold(time.Millisecond),
			delay:    13 * time.Millisecond,
		},
		{
			name:      "normaluse",
			estimates: []DelayStats{{Estimate: 0}},
			expected:  []usage{usageNormal},
			thresh:    staticThreshold(time.Millisecond),
			delay:     0,
		},
		{
			name:      "underuse",
			estimates: []DelayStats{{Estimate: -2 * time.Millisecond}},
			expected:  []usage{usageUnder},
			thresh:    staticThreshold(time.Millisecond),
			delay:     0,
		},
		{
			name: "noOverUseBeforeDelay",
			estimates: []DelayStats{
				{},
				{Estimate: 3 * time.Millisecond},
				{Estimate: 5 * time.Millisecond},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
			thresh:   staticThreshold(1 * time.Millisecond),
			delay:    10 * time.Millisecond,
		},
		{
			name: "noOverUseIfEstimateDecreased",
			estimates: []DelayStats{
				{},
				{Estimate: 4 * time.Millisecond},
				{Estimate: 5 * time.Millisecond},
				{Estimate: 3 * time.Millisecond},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
			thresh:   staticThreshold(1 * time.Millisecond),
			delay:    0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			od := newOveruseDetector(tc.thresh, tc.delay)
			in := make(chan DelayStats)
			out := od.run(in)
			go func() {
				for _, e := range tc.estimates {
					in <- e
					time.Sleep(tc.delay)
				}
				close(in)
			}()
			received := []usage{}
			for s := range out {
				received = append(received, s.Usage)
			}
			assert.Equal(t, tc.expected, received, "%v != %v", tc.expected, received)
		})
	}
}
