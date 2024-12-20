// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateControllerRun(t *testing.T) {
	cases := []struct {
		name           string
		initialBitrate int
		usage          []usage
		expected       []DelayStats
	}{
		{
			name:           "empty",
			initialBitrate: 100_000,
			usage:          []usage{},
			expected:       []DelayStats{},
		},
		{
			name:           "increasesMultiplicativelyBy8000",
			initialBitrate: 100_000,
			usage:          []usage{usageNormal, usageNormal},
			expected: []DelayStats{{
				Usage:         usageNormal,
				State:         stateIncrease,
				TargetBitrate: 108_000,
				Estimate:      0,
				Threshold:     0,
			}},
		},
	}

	t0 := time.Time{}
	mockNoFn := func() time.Time {
		t0 = t0.Add(100 * time.Millisecond)
		return t0
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := make(chan DelayStats)
			dc := newRateController(mockNoFn, 100_000, 1_000, 50_000_000, func(ds DelayStats) {
				out <- ds
			})
			in := make(chan DelayStats)
			dc.onReceivedRate(100_000)
			dc.updateRTT(300 * time.Millisecond)
			go func() {
				defer close(out)
				for _, state := range tc.usage {
					dc.onDelayStats(DelayStats{
						Measurement:   0,
						Estimate:      0,
						Threshold:     0,
						Usage:         state,
						State:         0,
						TargetBitrate: 0,
					})
				}
				close(in)
			}()
			received := []DelayStats{}
			for ds := range out {
				received = append(received, ds)
			}
			if len(tc.expected) > 0 {
				assert.Equal(t, tc.expected[0], received[0])
			}
		})
	}
}

func TestRateController_StateTransition(t *testing.T) {
	tcs := []struct {
		name       string
		delayStats []DelayStats
		wantStates []state
	}{
		{
			name:       "overuse-normal",
			delayStats: []DelayStats{{Usage: usageOver}, {Usage: usageNormal}},
			wantStates: []state{stateDecrease, stateHold},
		},
		{
			name:       "overuse-underuse",
			delayStats: []DelayStats{{Usage: usageOver}, {Usage: usageUnder}},
			wantStates: []state{stateDecrease, stateHold},
		},
		{
			name:       "normal",
			delayStats: []DelayStats{{Usage: usageNormal}},
			wantStates: []state{stateIncrease},
		},
		{
			name:       "under-over",
			delayStats: []DelayStats{{Usage: usageUnder}, {Usage: usageOver}},
			wantStates: []state{stateHold, stateDecrease},
		},
		{
			name:       "under-normal",
			delayStats: []DelayStats{{Usage: usageUnder}, {Usage: usageNormal}},
			wantStates: []state{stateHold, stateIncrease},
		},
		{
			name:       "under-under",
			delayStats: []DelayStats{{Usage: usageUnder}, {Usage: usageUnder}},
			wantStates: []state{stateHold, stateHold},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			rc := newRateController(time.Now, 500_000, 100_000, 1_000_000, func(DelayStats) {})
			// Call it once to initialize the rate controller
			rc.onDelayStats(DelayStats{})

			for i, ds := range tc.delayStats {
				rc.onDelayStats(ds)
				if rc.lastState != tc.wantStates[i] {
					t.Errorf("expected lastState to be %v but got %v", tc.wantStates[i], rc.lastState)
				}
			}
		})
	}
}
