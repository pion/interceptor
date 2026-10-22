// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

func TestArrivalGroupAccumulator(t *testing.T) {
	triggerNewGroupElement := cc.Acknowledgment{
		Departure: time.Time{}.Add(time.Second),
		Arrival:   time.Time{}.Add(time.Second),
	}
	cases := []struct {
		name string
		log  []cc.Acknowledgment
		exp  []arrivalGroup
	}{
		{
			name: "emptyCreatesNoGroups",
			log:  []cc.Acknowledgment{},
			exp:  []arrivalGroup{},
		},
		{
			name: "createsSingleElementGroup",
			log: []cc.Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(time.Millisecond),
					departure: time.Time{},
				},
			},
		},
		{
			name: "createsTwoElementGroup",
			log: []cc.Acknowledgment{
				{
					Arrival: time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{{
				arrival:   time.Time{}.Add(20 * time.Millisecond),
				departure: time.Time{},
			}},
		},
		{
			name: "createsTwoArrivalGroups",
			log: []cc.Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(9 * time.Millisecond),
					Arrival:   time.Time{}.Add(30 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(20 * time.Millisecond),
					departure: time.Time{}.Add(0 * time.Millisecond),
				},
				{
					arrival:   time.Time{}.Add(30 * time.Millisecond),
					departure: time.Time{}.Add(9 * time.Millisecond),
				},
			},
		},
		{
			name: "ignoresOutOfOrderPackets",
			log: []cc.Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(6 * time.Millisecond),
					Arrival:   time.Time{}.Add(34 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(8 * time.Millisecond),
					Arrival:   time.Time{}.Add(30 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(15 * time.Millisecond),
					departure: time.Time{},
				},
				{
					arrival:   time.Time{}.Add(34 * time.Millisecond),
					departure: time.Time{}.Add(6 * time.Millisecond),
				},
			},
		},
		{
			name: "newGroupBecauseOfInterDepartureTime",
			log: []cc.Acknowledgment{
				{
					SequenceNumber: 0,
					Departure:      time.Time{},
					Arrival:        time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SequenceNumber: 1,
					Departure:      time.Time{}.Add(3 * time.Millisecond),
					Arrival:        time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SequenceNumber: 2,
					Departure:      time.Time{}.Add(6 * time.Millisecond),
					Arrival:        time.Time{}.Add(10 * time.Millisecond),
				},
				{
					SequenceNumber: 3,
					Departure:      time.Time{}.Add(9 * time.Millisecond),
					Arrival:        time.Time{}.Add(10 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					departure: time.Time{},
					arrival:   time.Time{}.Add(4 * time.Millisecond),
				},
				{
					departure: time.Time{}.Add(6 * time.Millisecond),
					arrival:   time.Time{}.Add(10 * time.Millisecond),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aga := newArrivalGroupAccumulator()
			in := make(chan []cc.Acknowledgment)
			out := make(chan arrivalGroup)
			go func() {
				defer close(out)
				aga.run(in, func(ag arrivalGroup) {
					out <- ag
				})
			}()
			go func() {
				in <- tc.log
				close(in)
			}()
			received := []arrivalGroup{}
			for g := range out {
				received = append(received, g)
			}
			assert.Equal(t, tc.exp, received)
		})
	}
}

func TestArrivalGroupAccumulator_ConstantArrival(t *testing.T) {
	start := time.Unix(1, 0)
	acks := make([]cc.Acknowledgment, 10000)
	for i := range acks {
		acks[i] = cc.Acknowledgment{
			Departure: start.Add(time.Duration(i) * time.Microsecond),
			Arrival:   start,
		}
	}
	last := []cc.Acknowledgment{{Departure: start.Add(time.Second), Arrival: start.Add(time.Second)}}
	var got arrivalGroup
	groups := 0
	allocations := testing.AllocsPerRun(1, func() {
		groups = 0
		in := make(chan []cc.Acknowledgment, 2)
		in <- acks
		in <- last
		close(in)
		newArrivalGroupAccumulator().run(in, func(group arrivalGroup) {
			got = group
			groups++
		})
	})
	assert.LessOrEqual(t, allocations, float64(4))
	assert.Equal(t, 1, groups)
	assert.Equal(t, start, got.departure)
	assert.Equal(t, start, got.arrival)
}
