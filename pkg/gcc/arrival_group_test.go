package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

func TestArrivalGroup(t *testing.T) {
	cases := []struct {
		name     string
		acks     []cc.Acknowledgment
		expected arrivalGroup
	}{
		{
			name: "createsEmptyArrivalGroup",
			acks: []cc.Acknowledgment{},
			expected: arrivalGroup{
				packets:   nil,
				arrival:   time.Time{},
				departure: time.Time{},
				rtt:       0,
			},
		},
		{
			name: "createsArrivalGroupContainingSingleACK",
			acks: []cc.Acknowledgment{{
				TLCC:      0,
				Size:      0,
				Departure: time.Time{},
				Arrival:   time.Time{},
				RTT:       0,
			}},
			expected: arrivalGroup{
				packets: []cc.Acknowledgment{{
					TLCC:      0,
					Size:      0,
					Departure: time.Time{},
					Arrival:   time.Time{},
					RTT:       0,
				}},
				arrival:   time.Time{},
				departure: time.Time{},
				rtt:       0,
			},
		},
		{
			name: "setsTimesToLastACK",
			acks: []cc.Acknowledgment{{
				TLCC:      0,
				Size:      0,
				Departure: time.Time{},
				Arrival:   time.Time{},
				RTT:       0,
			}, {
				TLCC:      0,
				Size:      0,
				Departure: time.Time{}.Add(time.Second),
				Arrival:   time.Time{}.Add(time.Second),
				RTT:       time.Hour,
			}},
			expected: arrivalGroup{
				packets: []cc.Acknowledgment{{
					TLCC:      0,
					Size:      0,
					Departure: time.Time{},
					Arrival:   time.Time{},
					RTT:       0,
				}, {
					TLCC:      0,
					Size:      0,
					Departure: time.Time{}.Add(time.Second),
					Arrival:   time.Time{}.Add(time.Second),
					RTT:       time.Hour,
				}},
				arrival:   time.Time{}.Add(time.Second),
				departure: time.Time{}.Add(time.Second),
				rtt:       time.Hour,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ag := arrivalGroup{}
			for _, ack := range tc.acks {
				ag.add(ack)
			}
			assert.Equal(t, tc.expected, ag)
		})
	}
}
