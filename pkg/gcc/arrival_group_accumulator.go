package gcc

import (
	"time"

	"github.com/pion/interceptor/internal/cc"
)

type arrivalGroupAccumulator struct {
	interDepartureThreshold          time.Duration
	interArrivalThreshold            time.Duration
	interGroupDelayVariationTreshold time.Duration
}

func newArrivalGroupAccumulator() *arrivalGroupAccumulator {
	return &arrivalGroupAccumulator{
		interDepartureThreshold:          5 * time.Millisecond,
		interArrivalThreshold:            5 * time.Millisecond,
		interGroupDelayVariationTreshold: 0,
	}
}

func (a *arrivalGroupAccumulator) run(in <-chan cc.Acknowledgment) <-chan arrivalGroup {
	out := make(chan arrivalGroup)
	go func() {
		init := false
		group := arrivalGroup{}
		for next := range in {
			if !init {
				group.add(next)
				init = true
				continue
			}
			if next.Arrival.Before(group.arrival) {
				// ignore out of order arrivals
				continue
			}
			if next.Departure.After(group.departure) {
				if interDepartureTimePkt(group, next) <= a.interDepartureThreshold {
					group.add(next)
					continue
				}

				if interArrivalTimePkt(group, next) <= a.interArrivalThreshold &&
					interGroupDelayVariationPkt(group, next) < a.interGroupDelayVariationTreshold {
					group.add(next)
					continue
				}

				out <- group
				group = arrivalGroup{}
				group.add(next)
			}
		}
		close(out)
	}()
	return out
}
