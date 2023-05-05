// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

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

func (a *arrivalGroupAccumulator) run(in <-chan []cc.Acknowledgment, agWriter func(arrivalGroup)) {
	init := false
	group := arrivalGroup{}
	for acks := range in {
		for _, next := range acks {
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

				agWriter(group)
				group = arrivalGroup{}
				group.add(next)
			}
		}
	}
}

func interArrivalTimePkt(a arrivalGroup, b cc.Acknowledgment) time.Duration {
	return b.Arrival.Sub(a.arrival)
}

func interDepartureTimePkt(a arrivalGroup, b cc.Acknowledgment) time.Duration {
	if len(a.packets) == 0 {
		return 0
	}
	return b.Departure.Sub(a.packets[len(a.packets)-1].Departure)
}

func interGroupDelayVariationPkt(a arrivalGroup, b cc.Acknowledgment) time.Duration {
	return b.Arrival.Sub(a.arrival) - b.Departure.Sub(a.departure)
}
