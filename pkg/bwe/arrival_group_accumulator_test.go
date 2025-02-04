package bwe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestArrivalGroupAccumulator(t *testing.T) {
	triggerNewGroupElement := Acknowledgment{
		Departure: time.Time{}.Add(time.Second),
		Arrival:   time.Time{}.Add(time.Second),
	}
	cases := []struct {
		name string
		log  []Acknowledgment
		exp  []arrivalGroup
	}{
		{
			name: "emptyCreatesNoGroups",
			log:  []Acknowledgment{},
			exp:  []arrivalGroup{},
		},
		{
			name: "createsSingleElementGroup",
			log: []Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(time.Millisecond),
				},
			},
			},
		},
		{
			name: "createsTwoElementGroup",
			log: []Acknowledgment{
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
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
			}},
		},
		{
			name: "createsTwoArrivalGroups1",
			log: []Acknowledgment{
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
					Arrival:   time.Time{}.Add(24 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					{
						Arrival: time.Time{}.Add(15 * time.Millisecond),
					},
					{
						Departure: time.Time{}.Add(3 * time.Millisecond),
						Arrival:   time.Time{}.Add(20 * time.Millisecond),
					},
				},
				{
					{
						Departure: time.Time{}.Add(9 * time.Millisecond),
						Arrival:   time.Time{}.Add(24 * time.Millisecond),
					},
				},
			},
		},
		{
			name: "createsTwoArrivalGroups2",
			log: []Acknowledgment{
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
					{
						Arrival: time.Time{}.Add(15 * time.Millisecond),
					},
					{
						Departure: time.Time{}.Add(3 * time.Millisecond),
						Arrival:   time.Time{}.Add(20 * time.Millisecond),
					},
				},
				{
					{
						Departure: time.Time{}.Add(9 * time.Millisecond),
						Arrival:   time.Time{}.Add(30 * time.Millisecond),
					},
				},
			},
		},
		{
			name: "ignoresOutOfOrderPackets",
			log: []Acknowledgment{
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
					{
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(15 * time.Millisecond),
					},
				},
				{
					{
						Departure: time.Time{}.Add(6 * time.Millisecond),
						Arrival:   time.Time{}.Add(34 * time.Millisecond),
					},
					{
						Departure: time.Time{}.Add(8 * time.Millisecond),
						Arrival:   time.Time{}.Add(30 * time.Millisecond),
					},
				},
			},
		},
		{
			name: "newGroupBecauseOfInterDepartureTime",
			log: []Acknowledgment{
				{
					SeqNr:     0,
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SeqNr:     1,
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SeqNr:     2,
					Departure: time.Time{}.Add(6 * time.Millisecond),
					Arrival:   time.Time{}.Add(10 * time.Millisecond),
				},
				{
					SeqNr:     3,
					Departure: time.Time{}.Add(9 * time.Millisecond),
					Arrival:   time.Time{}.Add(10 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					{
						SeqNr:     0,
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(4 * time.Millisecond),
					},
					{
						SeqNr:     1,
						Departure: time.Time{}.Add(3 * time.Millisecond),
						Arrival:   time.Time{}.Add(4 * time.Millisecond),
					},
				},
				{
					{
						SeqNr:     2,
						Departure: time.Time{}.Add(6 * time.Millisecond),
						Arrival:   time.Time{}.Add(10 * time.Millisecond),
					},
					{
						SeqNr:     3,
						Departure: time.Time{}.Add(9 * time.Millisecond),
						Arrival:   time.Time{}.Add(10 * time.Millisecond),
					},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			aga := newArrivalGroupAccumulator()
			received := []arrivalGroup{}
			for _, ack := range tc.log {
				next := aga.onPacketAcked(ack)
				if next != nil {
					received = append(received, next)
				}
			}
			assert.Equal(t, tc.exp, received)
		})
	}
}
