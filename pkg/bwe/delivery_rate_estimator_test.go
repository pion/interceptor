package bwe

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeliveryRateEstimator(t *testing.T) {
	type ack struct {
		arrival time.Time
		size    int
	}
	cases := []struct {
		window       time.Duration
		acks         []ack
		expectedRate int
	}{
		{
			window:       0,
			acks:         []ack{},
			expectedRate: 0,
		},
		{
			window:       time.Second,
			acks:         []ack{},
			expectedRate: 0,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}, 1200},
			},
			expectedRate: 0,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(time.Millisecond), 1200},
			},
			expectedRate: 0,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(time.Second), 1200},
				{time.Time{}.Add(1500 * time.Millisecond), 1200},
				{time.Time{}.Add(2 * time.Second), 1200},
			},
			expectedRate: 28800,
		},
		{
			window: time.Second,
			acks: []ack{
				{time.Time{}.Add(500 * time.Millisecond), 1200},
				{time.Time{}.Add(time.Second), 1200},
				{time.Time{}.Add(1500 * time.Millisecond), 1200},
				{time.Time{}.Add(2 * time.Second), 1200},
			},
			expectedRate: 28800,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			e := newDeliveryRateEstimator(tc.window)
			for _, ack := range tc.acks {
				e.OnPacketAcked(ack.arrival, ack.size)
			}
			assert.Equal(t, tc.expectedRate, e.GetRate())
		})
	}
}
