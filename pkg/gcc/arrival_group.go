package gcc

import (
	"fmt"
	"time"
)

type arrivalGroup struct {
	packets   []Acknowledgment
	arrival   time.Time
	departure time.Time
	rtt       time.Duration
}

func (g *arrivalGroup) add(a Acknowledgment) {
	g.packets = append(g.packets, a)
	g.arrival = a.Arrival
	g.departure = a.Departure
	g.rtt = a.RTT
}

func (g arrivalGroup) String() string {
	s := "ARRIVALGROUP:\n"
	s += fmt.Sprintf("\tARRIVAL:\t%v\n", g.arrival)
	s += fmt.Sprintf("\tDEPARTURE:\t%v\n", g.departure)
	s += fmt.Sprintf("\tRTT:\t%v\n", g.rtt)
	s += fmt.Sprintf("\tPACKETS:%v\n", g.packets)
	return s
}
