// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

// groupEpoch is the origin of every test timestamp.
func groupEpoch() time.Time { return time.Unix(1_700_000_000, 0) }

// ackAt is a packet sent and received the given milliseconds after
// groupEpoch.
func ackAt(seq uint16, departureMS, arrivalMS float64) cc.Acknowledgment {
	return cc.Acknowledgment{
		SequenceNumber: seq,
		Departure:      groupEpoch().Add(msToDuration(departureMS)),
		Arrival:        groupEpoch().Add(msToDuration(arrivalMS)),
	}
}

func TestArrivalGroupSinglePacket(t *testing.T) {
	g := newArrivalGroup(ackAt(1, 10, 30))

	assert.Len(t, g.packets, 1)
	assert.Equal(t, g.firstDeparture, g.departure)
	assert.Equal(t, g.firstArrival, g.arrival)
	assert.Equal(t, groupEpoch().Add(10*time.Millisecond), g.departure)
	assert.Equal(t, groupEpoch().Add(30*time.Millisecond), g.arrival)
}

// The group's times are its LAST packet's, its extent starts at the first.
func TestArrivalGroupTracksLastDepartureAndArrival(t *testing.T) {
	g := newArrivalGroup(ackAt(1, 0, 20))
	g.add(ackAt(2, 2, 21))
	g.add(ackAt(3, 4, 23))

	assert.Len(t, g.packets, 3)
	assert.Equal(t, groupEpoch(), g.firstDeparture)
	assert.Equal(t, groupEpoch().Add(20*time.Millisecond), g.firstArrival)
	assert.Equal(t, groupEpoch().Add(4*time.Millisecond), g.departure)
	assert.Equal(t, groupEpoch().Add(23*time.Millisecond), g.arrival)
}

// A packet sent earlier than the group's latest keeps the latest departure.
func TestArrivalGroupKeepsLatestDeparture(t *testing.T) {
	g := newArrivalGroup(ackAt(1, 0, 20))
	g.add(ackAt(2, 4, 21))
	g.add(ackAt(3, 3, 22))

	assert.Equal(t, groupEpoch().Add(4*time.Millisecond), g.departure)
	assert.Equal(t, groupEpoch().Add(22*time.Millisecond), g.arrival)
}
