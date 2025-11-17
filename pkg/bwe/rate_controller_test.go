package bwe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateController(t *testing.T) {
	cases := []struct {
		name         string
		rc           rateController
		ts           time.Time
		u            usage
		delivered    int
		rtt          time.Duration
		expectedRate int
	}{
		{
			name: "zero",
			rc: rateController{
				s:              0,
				rate:           0,
				decreaseFactor: 0,
				lastUpdate:     time.Time{},
				lastDecrease:   &exponentialMovingAverage{},
			},
			ts:           time.Time{},
			u:            0,
			delivered:    0,
			rtt:          0,
			expectedRate: 0,
		},
		{
			name: "multiplicativeIncrease",
			rc: rateController{
				s:              stateIncrease,
				rate:           100,
				decreaseFactor: 0.9,
				lastUpdate:     time.Time{},
				lastDecrease:   &exponentialMovingAverage{},
			},
			ts:           time.Time{}.Add(time.Second),
			u:            usageNormal,
			delivered:    100,
			rtt:          0,
			expectedRate: 108,
		},
		{
			name: "minimumAdditiveIncrease",
			rc: rateController{
				s:              stateIncrease,
				rate:           100_000,
				decreaseFactor: 0.9,
				lastUpdate:     time.Time{},
				lastDecrease: &exponentialMovingAverage{
					average: 100_000,
				},
			},
			ts:           time.Time{}.Add(time.Second),
			u:            usageNormal,
			delivered:    100_000,
			rtt:          20 * time.Millisecond,
			expectedRate: 101_000,
		},
		{
			name: "additiveIncrease",
			rc: rateController{
				s:              stateIncrease,
				rate:           1_000_000,
				decreaseFactor: 0.9,
				lastUpdate:     time.Time{},
				lastDecrease: &exponentialMovingAverage{
					average: 1_000_000,
				},
			},
			ts:           time.Time{}.Add(time.Second),
			u:            usageNormal,
			delivered:    1_000_000,
			rtt:          2000 * time.Millisecond,
			expectedRate: 1_004166,
		},
		{
			name: "minimumAdditiveIncreaseAppLimited",
			rc: rateController{
				s:              stateIncrease,
				rate:           100_000,
				decreaseFactor: 0.9,
				lastUpdate:     time.Time{},
				lastDecrease: &exponentialMovingAverage{
					average: 100_000,
				},
			},
			ts:           time.Time{}.Add(time.Second),
			u:            usageNormal,
			delivered:    50_000,
			rtt:          20 * time.Millisecond,
			expectedRate: 100_000,
		},
		{
			name: "additiveIncreaseAppLimited",
			rc: rateController{
				s:              stateIncrease,
				rate:           1_000_000,
				decreaseFactor: 0.9,
				lastUpdate:     time.Time{},
				lastDecrease: &exponentialMovingAverage{
					average: 1_000_000,
				},
			},
			ts:           time.Time{}.Add(time.Second),
			u:            usageNormal,
			delivered:    100_000,
			rtt:          2000 * time.Millisecond,
			expectedRate: 1_000_000,
		},
		{
			name: "decrease",
			rc: rateController{
				s:              stateDecrease,
				rate:           1_000_000,
				decreaseFactor: 0.9,
				lastUpdate:     time.Time{},
				lastDecrease: &exponentialMovingAverage{
					average: 1_000_000,
				},
			},
			ts:           time.Time{}.Add(time.Second),
			u:            usageOver,
			delivered:    1_000_000,
			rtt:          2000 * time.Millisecond,
			expectedRate: 900_000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.rc.update(tc.ts, tc.u, tc.delivered, tc.rtt)
			assert.Equal(t, tc.expectedRate, res)
		})
	}
}
