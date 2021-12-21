package gcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInterArrivalTime(t *testing.T) {
	cases := []struct {
		a   arrivalGroup
		b   Acknowledgment
		exp time.Duration
	}{
		{
			a:   arrivalGroup{},
			b:   Acknowledgment{},
			exp: 0,
		},
		{
			a: arrivalGroup{},
			b: Acknowledgment{
				Arrival: time.Time{}.Add(5 * time.Millisecond),
			},
			exp: 5 * time.Millisecond,
		},
	}
	for i, tc := range cases {
		i := i
		tc := tc
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tc.exp, interArrivalTimePkt(tc.a, tc.b))
		})
	}
}

func TestInterDepartureTime(t *testing.T) {
	cases := []struct {
		a   arrivalGroup
		b   Acknowledgment
		exp time.Duration
	}{
		{
			a:   arrivalGroup{},
			b:   Acknowledgment{},
			exp: 0,
		},
		{
			a: arrivalGroup{
				packets: []Acknowledgment{{
					Arrival: time.Time{},
				}},
				arrival:   time.Time{},
				departure: time.Time{},
			},
			b: Acknowledgment{
				Departure: time.Time{}.Add(5 * time.Millisecond),
				Arrival:   time.Time{},
			},
			exp: 5 * time.Millisecond,
		},
	}
	for i, tc := range cases {
		i := i
		tc := tc
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tc.exp, interDepartureTimePkt(tc.a, tc.b))
		})
	}
}

func TestInterGroupDelayVariation(t *testing.T) {
	cases := []struct {
		a, b arrivalGroup
		exp  time.Duration
	}{
		{
			a:   arrivalGroup{},
			b:   arrivalGroup{},
			exp: 0,
		},
		{
			a: arrivalGroup{
				packets:   []Acknowledgment{},
				arrival:   time.Time{}.Add(5 * time.Millisecond),
				departure: time.Time{},
			},
			b: arrivalGroup{
				packets:   []Acknowledgment{},
				arrival:   time.Time{}.Add(15 * time.Millisecond),
				departure: time.Time{}.Add(5 * time.Millisecond),
			},
			exp: 5 * time.Millisecond,
		},
	}
	for i, tc := range cases {
		i := i
		tc := tc
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tc.exp, interGroupDelayVariation(tc.a, tc.b))
		})
	}
}

func TestInterGroupDelayVariationPkt(t *testing.T) {
	cases := []struct {
		a   arrivalGroup
		b   Acknowledgment
		exp time.Duration
	}{
		{
			a:   arrivalGroup{},
			b:   Acknowledgment{},
			exp: 0,
		},
		{
			a: arrivalGroup{
				packets:   []Acknowledgment{},
				arrival:   time.Time{}.Add(5 * time.Millisecond),
				departure: time.Time{},
			},
			b: Acknowledgment{
				Departure: time.Time{}.Add(5 * time.Millisecond),
				Arrival:   time.Time{}.Add(15 * time.Millisecond),
			},
			exp: 5 * time.Millisecond,
		},
	}
	for i, tc := range cases {
		i := i
		tc := tc
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tc.exp, interGroupDelayVariationPkt(tc.a, tc.b))
		})
	}
}

func TestPreFilter(t *testing.T) {
	cases := []struct {
		log []Acknowledgment
		exp []arrivalGroup
	}{
		{
			log: []Acknowledgment{},
			exp: []arrivalGroup{},
		},
		{
			log: []Acknowledgment{
				{
					Size:      0,
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(time.Millisecond),
					RTT:       0,
				},
			},
			exp: []arrivalGroup{
				{
					packets: []Acknowledgment{{
						Size:      0,
						Departure: time.Time{},
						Arrival:   time.Time{}.Add(time.Millisecond),
						RTT:       0,
					}},
					arrival:   time.Time{}.Add(time.Millisecond),
					departure: time.Time{},
					rtt:       0,
				},
			},
		},
		{
			log: []Acknowledgment{
				{
					Arrival: time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
			},
			exp: []arrivalGroup{{
				packets: []Acknowledgment{
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
			}},
		},
		{
			log: []Acknowledgment{
				{
					Arrival: time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(6 * time.Millisecond),
					Arrival:   time.Time{}.Add(30 * time.Millisecond),
				},
			},
			exp: []arrivalGroup{
				{
					packets: []Acknowledgment{
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
					packets: []Acknowledgment{
						{
							Departure: time.Time{}.Add(6 * time.Millisecond),
							Arrival:   time.Time{}.Add(30 * time.Millisecond),
						},
					},
					arrival:   time.Time{}.Add(30 * time.Millisecond),
					departure: time.Time{}.Add(6 * time.Millisecond),
				},
			},
		},
	}
	for i, tc := range cases {
		i := i
		tc := tc
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tc.exp, preFilter(nil, tc.log))
		})
	}
}
