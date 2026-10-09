// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/cc"
	"github.com/stretchr/testify/assert"
)

// groupSeqs runs the accumulator over acks and returns the sequence numbers
// of every completed group.
func groupSeqs(acks []cc.Acknowledgment) [][]uint16 {
	in := make(chan []cc.Acknowledgment, 1)
	in <- acks
	close(in)
	groups := [][]uint16{}
	newArrivalGroupAccumulator().run(in, func(g arrivalGroup) {
		seqs := []uint16{}
		for _, p := range g.packets {
			seqs = append(seqs, p.SequenceNumber)
		}
		groups = append(groups, seqs)
	})

	return groups
}

func TestArrivalGroupAccumulator(t *testing.T) {
	cases := []struct {
		name string
		acks []cc.Acknowledgment
		exp  [][]uint16
	}{
		{
			name: "empty creates no groups",
			acks: nil,
			exp:  [][]uint16{},
		},
		{
			name: "a group completes when the next one starts",
			acks: []cc.Acknowledgment{ackAt(1, 0, 10), ackAt(2, 20, 30)},
			exp:  [][]uint16{{1}},
		},
		{
			name: "packets sent within 5 ms form one group",
			acks: []cc.Acknowledgment{
				ackAt(1, 0, 10), ackAt(2, 2, 12), ackAt(3, 5, 15), ackAt(4, 20, 30),
			},
			exp: [][]uint16{{1, 2, 3}},
		},
		{
			// The 5 ms span counts from the group's FIRST packet: packets
			// 3 ms apart do not chain into one long group.
			name: "a paced burst splits after 5 ms",
			acks: []cc.Acknowledgment{
				ackAt(1, 0, 10), ackAt(2, 3, 13), ackAt(3, 6, 16), ackAt(4, 9, 19), ackAt(5, 30, 40),
			},
			exp: [][]uint16{{1, 2}, {3, 4}},
		},
		{
			// Arriving faster than sent (≤ 5 ms apart, shrinking delay):
			// a burst that queued behind earlier traffic stays one group.
			name: "a burst arriving back to back stays one group",
			acks: []cc.Acknowledgment{
				ackAt(1, 0, 20), ackAt(2, 6, 21), ackAt(3, 12, 22), ackAt(4, 40, 60),
			},
			exp: [][]uint16{{1, 2, 3}},
		},
		{
			name: "packets sent at the same time always group",
			acks: []cc.Acknowledgment{
				ackAt(1, 0, 10), ackAt(2, 0, 25), ackAt(3, 30, 40),
			},
			exp: [][]uint16{{1, 2}},
		},
		{
			name: "out of order packets are ignored",
			acks: []cc.Acknowledgment{
				ackAt(1, 10, 20), ackAt(2, 5, 21), ackAt(3, 12, 15), ackAt(4, 30, 40), ackAt(5, 50, 60),
			},
			exp: [][]uint16{{1}, {4}},
		},
		{
			name: "lost packets are ignored",
			acks: []cc.Acknowledgment{
				ackAt(1, 0, 10),
				{SequenceNumber: 2, Departure: groupEpoch().Add(2 * time.Millisecond)},
				ackAt(3, 3, 13), ackAt(4, 30, 40),
			},
			exp: [][]uint16{{1, 3}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.exp, groupSeqs(tc.acks))
		})
	}
}

// A burst may extend a group by arrival for at most maxBurstDuration.
func TestArrivalGroupAccumulatorBoundsBursts(t *testing.T) {
	var acks []cc.Acknowledgment
	// Sent 6 ms apart, arriving 4 ms apart: every packet extends the burst.
	for i := range 40 {
		acks = append(acks, ackAt(uint16(i), float64(i*6), 200+float64(i*4))) //nolint:gosec // small
	}
	acks = append(acks, ackAt(99, 1000, 1200))

	groups := groupSeqs(acks)
	assert.Len(t, groups, 2)
	// Arrivals 0, 4, … 96 ms after the first are < 100 ms: 25 packets.
	assert.Len(t, groups[0], 25)
	assert.Len(t, groups[1], 15)
}
