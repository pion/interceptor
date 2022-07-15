package gcc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func identity(d time.Duration) time.Duration {
	return d
}

func TestSlopeEstimator(t *testing.T) {
	cases := []struct {
		name     string
		ags      []arrivalGroup
		expected []DelayStats
	}{
		{
			name:     "emptyReturnsEmpty",
			ags:      []arrivalGroup{},
			expected: []DelayStats{},
		},
		{
			name: "simpleDeltaTest",
			ags: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(5 * time.Millisecond),
					departure: time.Time{}.Add(15 * time.Millisecond),
				},
				{
					arrival:   time.Time{}.Add(10 * time.Millisecond),
					departure: time.Time{}.Add(20 * time.Millisecond),
				},
			},
			expected: []DelayStats{
				{
					Measurement:      0,
					Estimate:         0,
					Threshold:        0,
					lastReceiveDelta: 5 * time.Millisecond,
					Usage:            0,
					State:            0,
					TargetBitrate:    0,
					RTT:              0,
				},
			},
		},
		{
			name: "twoMeasurements",
			ags: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(5 * time.Millisecond),
					departure: time.Time{}.Add(15 * time.Millisecond),
				},
				{
					arrival:   time.Time{}.Add(10 * time.Millisecond),
					departure: time.Time{}.Add(20 * time.Millisecond),
				},
				{
					arrival:   time.Time{}.Add(15 * time.Millisecond),
					departure: time.Time{}.Add(30 * time.Millisecond),
				},
			},
			expected: []DelayStats{
				{
					Measurement:      0,
					Estimate:         0,
					Threshold:        0,
					lastReceiveDelta: 5 * time.Millisecond,
					Usage:            0,
					State:            0,
					TargetBitrate:    0,
					RTT:              0,
				},
				{
					Measurement:      -5 * time.Millisecond,
					Estimate:         -5 * time.Millisecond,
					Threshold:        0,
					lastReceiveDelta: 5 * time.Millisecond,
					Usage:            0,
					State:            0,
					TargetBitrate:    0,
					RTT:              0,
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := make(chan DelayStats)
			se := newSlopeEstimator(estimatorFunc(identity), func(ds DelayStats) {
				out <- ds
			})
			input := []time.Duration{}
			go func() {
				defer close(out)
				for _, ag := range tc.ags {
					se.onArrivalGroup(ag)
				}
			}()
			received := []DelayStats{}
			for d := range out {
				received = append(received, d)
			}
			assert.Equal(t, tc.expected, received, "%v != %v", input, received)
		})
	}
}
