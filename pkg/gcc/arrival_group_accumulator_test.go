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
					packets: []cc.Acknowledgment{{
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(time.Millisecond),
					}},
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
				packets: []cc.Acknowledgment{
					{
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(15 * time.Millisecond),
					},
					{
						Departure: time.Time{}.Add(3 * time.Millisecond),
						Arrival:   time.Time{}.Add(20 * time.Millisecond),
					},
				},
				arrival:   time.Time{}.Add(20 * time.Millisecond),
				departure: time.Time{}.Add(3 * time.Millisecond),
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
					packets: []cc.Acknowledgment{
						{
							Arrival: time.Time{}.Add(15 * time.Millisecond),
						},
						{
							Departure: time.Time{}.Add(3 * time.Millisecond),
							Arrival:   time.Time{}.Add(20 * time.Millisecond),
						},
					},
					arrival:   time.Time{}.Add(20 * time.Millisecond),
					departure: time.Time{}.Add(3 * time.Millisecond),
				},
				{
					packets: []cc.Acknowledgment{
						{
							Departure: time.Time{}.Add(9 * time.Millisecond),
							Arrival:   time.Time{}.Add(30 * time.Millisecond),
						},
					},
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
					packets: []cc.Acknowledgment{
						{
							Departure: time.Time{},
							Arrival:   time.Time{}.Add(15 * time.Millisecond),
						},
					},
					arrival:   time.Time{}.Add(15 * time.Millisecond),
					departure: time.Time{},
				},
				{
					packets: []cc.Acknowledgment{
						{
							Departure: time.Time{}.Add(6 * time.Millisecond),
							Arrival:   time.Time{}.Add(34 * time.Millisecond),
						},
					},
					arrival:   time.Time{}.Add(34 * time.Millisecond),
					departure: time.Time{}.Add(6 * time.Millisecond),
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			aga := newArrivalGroupAccumulator()
			in := make(chan cc.Acknowledgment)
			out := aga.run(in)
			go func() {
				for _, as := range tc.log {
					in <- as
				}
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
