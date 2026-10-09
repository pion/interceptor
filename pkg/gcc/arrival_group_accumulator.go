// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"time"

	"github.com/pion/interceptor/internal/cc"
)

const (
	// burstTime is the departure span of one group (libwebrtc
	// kBurstDeltaThreshold / kSendTimeGroupLength).
	burstTime = 5 * time.Millisecond
	// maxBurstDuration bounds how long arrivals may extend a group.
	maxBurstDuration = 100 * time.Millisecond
)

// arrivalGroupAccumulator groups acknowledged packets into send bursts the
// way libwebrtc's InterArrivalDelta does.
type arrivalGroupAccumulator struct{}

func newArrivalGroupAccumulator() *arrivalGroupAccumulator {
	return &arrivalGroupAccumulator{}
}

func (a *arrivalGroupAccumulator) run(in <-chan []cc.Acknowledgment, agWriter func(arrivalGroup)) {
	init := false
	group := arrivalGroup{}
	for acks := range in {
		for _, next := range acks {
			if next.Arrival.IsZero() {
				// lost: carries no arrival time
				continue
			}
			if !init {
				group = newArrivalGroup(next)
				init = true

				continue
			}
			if next.Departure.Before(group.firstDeparture) || next.Arrival.Before(group.arrival) {
				// reordered: ignored by the arrival-time model
				continue
			}
			if belongsToGroup(group, next) {
				group.add(next)

				continue
			}
			agWriter(group)
			group = newArrivalGroup(next)
		}
	}
}

// belongsToGroup reports whether next extends the group: sent within
// burstTime of the group's first packet, or part of a burst that arrived
// back to back (it reached the receiver faster than it was sent).
func belongsToGroup(group arrivalGroup, next cc.Acknowledgment) bool {
	if belongsToBurst(group, next) {
		return true
	}

	return next.Departure.Sub(group.firstDeparture) <= burstTime
}

func belongsToBurst(group arrivalGroup, next cc.Acknowledgment) bool {
	arrivalDelta := next.Arrival.Sub(group.arrival)
	departureDelta := next.Departure.Sub(group.departure)
	if departureDelta == 0 {
		return true
	}
	propagationDelta := arrivalDelta - departureDelta

	return propagationDelta < 0 &&
		arrivalDelta <= burstTime &&
		next.Arrival.Sub(group.firstArrival) < maxBurstDuration
}
