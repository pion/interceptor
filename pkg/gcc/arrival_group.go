// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"time"

	"github.com/pion/interceptor/internal/cc"
)

// arrivalGroup is a group of packets sent in one burst. As in libwebrtc's
// InterArrivalDelta, inter-group deltas use the group's LAST departure and
// LAST arrival: pairing the first departure with the last arrival made the
// group's own send span read as queueing delay.
type arrivalGroup struct {
	packets []cc.Acknowledgment
	// firstDeparture and firstArrival bound the group's extent.
	firstDeparture time.Time
	firstArrival   time.Time
	// departure is the latest departure, arrival the last arrival.
	departure time.Time
	arrival   time.Time
}

func newArrivalGroup(a cc.Acknowledgment) arrivalGroup {
	return arrivalGroup{
		packets:        []cc.Acknowledgment{a},
		firstDeparture: a.Departure,
		firstArrival:   a.Arrival,
		departure:      a.Departure,
		arrival:        a.Arrival,
	}
}

func (g *arrivalGroup) add(a cc.Acknowledgment) {
	g.packets = append(g.packets, a)
	if a.Departure.After(g.departure) {
		g.departure = a.Departure
	}
	g.arrival = a.Arrival
}

func (g arrivalGroup) String() string {
	s := "ARRIVALGROUP:\n"
	s += fmt.Sprintf("\tARRIVAL:\t%v\n", int64(float64(g.arrival.UnixNano())/1e+6))
	s += fmt.Sprintf("\tDEPARTURE:\t%v\n", int64(float64(g.departure.UnixNano())/1e+6))
	s += fmt.Sprintf("\tPACKETS:\n%v\n", g.packets)

	return s
}
