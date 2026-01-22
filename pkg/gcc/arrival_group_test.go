// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
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
				departure: time.Time{},
			},
		},
		{
			name: "departure time of group is the departure time of the first packet in the group",
			acks: []cc.Acknowledgment{{
				SequenceNumber: 0,
				Size:           0,
				Departure:      time.Time{}.Add(27 * time.Millisecond),
				Arrival:        time.Time{},
			}, {
				SequenceNumber: 1,
				Size:           1,
				Departure:      time.Time{}.Add(32 * time.Millisecond),
				Arrival:        time.Time{}.Add(37 * time.Millisecond),
			}, {
				SequenceNumber: 2,
				Size:           2,
				Departure:      time.Time{}.Add(50 * time.Millisecond),
				Arrival:        time.Time{}.Add(56 * time.Millisecond),
			}},
			expected: arrivalGroup{
				packets: []cc.Acknowledgment{{
					SequenceNumber: 0,
					Size:           0,
					Departure:      time.Time{}.Add(27 * time.Millisecond),
					Arrival:        time.Time{},
				}, {
					SequenceNumber: 1,
					Size:           1,
					Departure:      time.Time{}.Add(32 * time.Millisecond),
					Arrival:        time.Time{}.Add(37 * time.Millisecond),
				}, {
					SequenceNumber: 2,
					Size:           2,
					Departure:      time.Time{}.Add(50 * time.Millisecond),
					Arrival:        time.Time{}.Add(56 * time.Millisecond),
				}},
				arrival:   time.Time{}.Add(56 * time.Millisecond),
				departure: time.Time{}.Add(27 * time.Millisecond),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ag := arrivalGroup{}
			for i, ack := range tc.acks {
				if i == 0 {
					ag = newArrivalGroup(ack)
				} else {
					ag.add(ack)
				}
			}
			assert.Equal(t, tc.expected, ag)
		})
	}
}
