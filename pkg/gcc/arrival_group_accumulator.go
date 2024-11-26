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
				group = newArrivalGroup(next)
				init = true
				continue
			}
			if next.Arrival.Before(group.arrival) {
				// ignore out of order arrivals
				continue
			}
			if next.Departure.After(group.departure) {
				// A sequence of packets which are sent within a burst_time interval
				// constitute a group.
				if interDepartureTimePkt(group, next) <= a.interDepartureThreshold {
					group.add(next)
					continue
				}

				// A Packet which has an inter-arrival time less than burst_time and
				// an inter-group delay variation d(i) less than 0 is considered
				// being part of the current group of packets.
				if interArrivalTimePkt(group, next) <= a.interArrivalThreshold &&
					interGroupDelayVariationPkt(group, next) < a.interGroupDelayVariationTreshold {
					group.add(next)
					continue
				}

				agWriter(group)
				group = newArrivalGroup(next)
			}
		}
	}
}

func interArrivalTimePkt(group arrivalGroup, ack cc.Acknowledgment) time.Duration {
	return ack.Arrival.Sub(group.arrival)
}

func interDepartureTimePkt(group arrivalGroup, ack cc.Acknowledgment) time.Duration {
	if len(group.packets) == 0 {
		return 0
	}
	return ack.Departure.Sub(group.departure)
}

func interGroupDelayVariationPkt(group arrivalGroup, ack cc.Acknowledgment) time.Duration {
	return ack.Arrival.Sub(group.arrival) - ack.Departure.Sub(group.departure)
}
