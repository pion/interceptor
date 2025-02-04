package bwe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOveruseDetectorUpdate(t *testing.T) {
	type estimate struct {
		ts        time.Time
		estimate  float64
		numDeltas int
	}
	cases := []struct {
		name     string
		adaptive bool
		values   []estimate
		expected []usage
	}{
		{
			name:     "noEstimateNoUsageStatic",
			adaptive: false,
			values:   []estimate{},
			expected: []usage{},
		},
		{
			name:     "overuseStatic",
			adaptive: false,
			values: []estimate{
				{time.Time{}, 1.0, 1},
				{time.Time{}.Add(5 * time.Millisecond), 20, 2},
				{time.Time{}.Add(20 * time.Millisecond), 30, 3},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name:     "normaluseStatic",
			adaptive: false,
			values:   []estimate{{estimate: 0}},
			expected: []usage{usageNormal},
		},
		{
			name:     "underuseStatic",
			adaptive: false,
			values:   []estimate{{time.Time{}, -20, 2}},
			expected: []usage{usageUnder},
		},
		{
			name:     "noOverUseBeforeDelayStatic",
			adaptive: false,
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 20, 1},
				{time.Time{}.Add(2 * time.Millisecond), 30, 2},
				{time.Time{}.Add(30 * time.Millisecond), 50, 3},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name:     "noOverUseIfEstimateDecreasedStatic",
			adaptive: false,
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 20, 1},
				{time.Time{}.Add(10 * time.Millisecond), 40, 2},
				{time.Time{}.Add(30 * time.Millisecond), 50, 3},
				{time.Time{}.Add(35 * time.Millisecond), 3, 4},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
		},
		{
			name:     "noEstimateNoUsageAdaptive",
			adaptive: true,
			values:   []estimate{},
			expected: []usage{},
		},
		{
			name:     "overuseAdaptive",
			adaptive: true,
			values: []estimate{
				{time.Time{}, 1, 1},
				{time.Time{}.Add(5 * time.Millisecond), 20, 2},
				{time.Time{}.Add(20 * time.Millisecond), 30, 3},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name:     "normaluseAdaptive",
			adaptive: true,
			values:   []estimate{{estimate: 0}},
			expected: []usage{usageNormal},
		},
		{
			name:     "underuseAdaptive",
			adaptive: true,
			values:   []estimate{{time.Time{}, -20, 2}},
			expected: []usage{usageUnder},
		},
		{
			name:     "noOverUseBeforeDelayAdaptive",
			adaptive: true,
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 20, 1},
				{time.Time{}.Add(2 * time.Millisecond), 30, 2},
				{time.Time{}.Add(30 * time.Millisecond), 50, 3},
			},
			expected: []usage{usageNormal, usageNormal, usageOver},
		},
		{
			name:     "noOverUseIfEstimateDecreasedAdaptive",
			adaptive: true,
			values: []estimate{
				{time.Time{}.Add(time.Millisecond), 20, 1},
				{time.Time{}.Add(10 * time.Millisecond), 40, 2},
				{time.Time{}.Add(30 * time.Millisecond), 50, 3},
				{time.Time{}.Add(35 * time.Millisecond), 3, 4},
			},
			expected: []usage{usageNormal, usageNormal, usageOver, usageNormal},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			od := newOveruseDetector(tc.adaptive)
			received := []usage{}
			for _, e := range tc.values {
				usage := od.update(e.ts, e.estimate, e.numDeltas)
				received = append(received, usage)
			}
			assert.Equal(t, tc.expected, received)
		})
	}
}

func TestOveruseDetectorAdaptThreshold(t *testing.T) {
	cases := []struct {
		name              string
		od                *overuseDetector
		ts                time.Time
		estimate          float64
		expectedThreshold float64
	}{
		{
			name:              "minThreshold",
			od:                &overuseDetector{},
			ts:                time.Time{},
			estimate:          0,
			expectedThreshold: 6,
		},
		{
			name: "increase",
			od: &overuseDetector{
				delayThreshold: 12.5,
				lastUpdate:     time.Time{}.Add(time.Second),
			},
			ts:                time.Time{}.Add(2 * time.Second),
			estimate:          25,
			expectedThreshold: 25,
		},
		{
			name: "maxThreshold",
			od: &overuseDetector{
				delayThreshold: 6,
				lastUpdate:     time.Time{},
			},
			ts:                time.Time{}.Add(time.Second),
			estimate:          6.1,
			expectedThreshold: 6,
		},
		{
			name: "decrease",
			od: &overuseDetector{
				delayThreshold: 12.5,
				lastUpdate:     time.Time{},
			},
			ts:                time.Time{}.Add(time.Second),
			estimate:          1,
			expectedThreshold: 12.5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.od.adaptThreshold(tc.ts, tc.estimate)
			assert.Equal(t, tc.expectedThreshold, tc.od.delayThreshold)
		})
	}
}
