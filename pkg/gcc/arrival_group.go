// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"fmt"
	"time"

	"github.com/pion/interceptor/internal/cc"
)

type arrivalGroup struct {
	packets   []cc.Acknowledgment
	departure time.Time
	arrival   time.Time
}

func (g *arrivalGroup) add(a cc.Acknowledgment) {
	g.packets = append(g.packets, a)
	g.arrival = a.Arrival
	g.departure = a.Departure
}

func (g arrivalGroup) String() string {
	s := "ARRIVALGROUP:\n"
	s += fmt.Sprintf("\tARRIVAL:\t%v\n", int64(float64(g.arrival.UnixNano())/1e+6))
	s += fmt.Sprintf("\tDEPARTURE:\t%v\n", int64(float64(g.departure.UnixNano())/1e+6))
	s += fmt.Sprintf("\tPACKETS:\n%v\n", g.packets)
	return s
}
