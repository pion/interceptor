package bwe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLossRateController(t *testing.T) {
	cases := []struct {
		init, min, max int
		acked          int
		lost           int
		deliveredRate  int
		expectedRate   int
	}{
		{}, // all zeros
		{
			init:          100_000,
			min:           100_000,
			max:           1_000_000,
			acked:         0,
			lost:          0,
			deliveredRate: 0,
			expectedRate:  100_000,
		},
		{
			init:          100_000,
			min:           100_000,
			max:           1_000_000,
			acked:         99,
			lost:          1,
			deliveredRate: 100_000,
			expectedRate:  105_000,
		},
		{
			init:          100_000,
			min:           100_000,
			max:           1_000_000,
			acked:         99,
			lost:          1,
			deliveredRate: 90_000,
			expectedRate:  105_000,
		},
		{
			init:          100_000,
			min:           100_000,
			max:           1_000_000,
			acked:         95,
			lost:          5,
			deliveredRate: 99_000,
			expectedRate:  100_000,
		},
		{
			init:          100_000,
			min:           50_000,
			max:           1_000_000,
			acked:         89,
			lost:          11,
			deliveredRate: 90_000,
			expectedRate:  94_500,
		},
		{
			init:          100_000,
			min:           100_000,
			max:           1_000_000,
			acked:         89,
			lost:          11,
			deliveredRate: 90_000,
			expectedRate:  100_000,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			lrc := NewLossRateController(tc.init, tc.min, tc.max)
			for i := 0; i < tc.acked; i++ {
				lrc.OnPacketAcked()
			}
			for i := 0; i < tc.lost; i++ {
				lrc.OnPacketLost()
			}
			assert.Equal(t, tc.expectedRate, lrc.Update(tc.deliveredRate))
		})
	}
}
