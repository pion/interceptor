// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

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
			},
		},
		{
			name: "createsArrivalGroupContainingSingleACK",
			acks: []cc.Acknowledgment{{
				SequenceNumber: 0,
				Size:           0,
				Departure:      time.Time{},
				Arrival:        time.Time{},
			}},
			expected: arrivalGroup{
				packets: []cc.Acknowledgment{{
					SequenceNumber: 0,
					Size:           0,
					Departure:      time.Time{},
					Arrival:        time.Time{},
				}},
				arrival:   time.Time{},
				departure: time.Time{},
			},
		},
		{
			name: "setsTimesToLastACK",
			acks: []cc.Acknowledgment{{
				SequenceNumber: 0,
				Size:           0,
				Departure:      time.Time{},
				Arrival:        time.Time{},
			}, {
				SequenceNumber: 0,
				Size:           0,
				Departure:      time.Time{}.Add(time.Second),
				Arrival:        time.Time{}.Add(time.Second),
			}},
			expected: arrivalGroup{
				packets: []cc.Acknowledgment{{
					SequenceNumber: 0,
					Size:           0,
					Departure:      time.Time{},
					Arrival:        time.Time{},
				}, {
					SequenceNumber: 0,
					Size:           0,
					Departure:      time.Time{}.Add(time.Second),
					Arrival:        time.Time{}.Add(time.Second),
				}},
				arrival:   time.Time{}.Add(time.Second),
				departure: time.Time{}.Add(time.Second),
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
