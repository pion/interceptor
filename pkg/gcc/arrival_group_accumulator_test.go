// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package gcc

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/cc"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestArrivalGroupAccumulator(t *testing.T) {
	triggerNewGroupElement := cc.Acknowledgment{
		Departure: time.Time{}.Add(time.Second),
		Arrival:   time.Time{}.Add(time.Second),
	}
	cases := []struct {
		name string
		log  []cc.Acknowledgment
		exp  []arrivalGroup
	}{
		{
			name: "emptyCreatesNoGroups",
			log:  []cc.Acknowledgment{},
			exp:  []arrivalGroup{},
		},
		{
			name: "createsSingleElementGroup",
			log: []cc.Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(time.Millisecond),
					departure: time.Time{},
				},
			},
		},
		{
			name: "createsTwoElementGroup",
			log: []cc.Acknowledgment{
				{
					Arrival: time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{{
				arrival:   time.Time{}.Add(20 * time.Millisecond),
				departure: time.Time{},
			}},
		},
		{
			name: "createsTwoArrivalGroups",
			log: []cc.Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(3 * time.Millisecond),
					Arrival:   time.Time{}.Add(20 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(9 * time.Millisecond),
					Arrival:   time.Time{}.Add(30 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(20 * time.Millisecond),
					departure: time.Time{}.Add(0 * time.Millisecond),
				},
				{
					arrival:   time.Time{}.Add(30 * time.Millisecond),
					departure: time.Time{}.Add(9 * time.Millisecond),
				},
			},
		},
		{
			name: "ignoresOutOfOrderPackets",
			log: []cc.Acknowledgment{
				{
					Departure: time.Time{},
					Arrival:   time.Time{}.Add(15 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(6 * time.Millisecond),
					Arrival:   time.Time{}.Add(34 * time.Millisecond),
				},
				{
					Departure: time.Time{}.Add(8 * time.Millisecond),
					Arrival:   time.Time{}.Add(30 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					arrival:   time.Time{}.Add(15 * time.Millisecond),
					departure: time.Time{},
				},
				{
					arrival:   time.Time{}.Add(34 * time.Millisecond),
					departure: time.Time{}.Add(6 * time.Millisecond),
				},
			},
		},
		{
			name: "newGroupBecauseOfInterDepartureTime",
			log: []cc.Acknowledgment{
				{
					SequenceNumber: 0,
					Departure:      time.Time{},
					Arrival:        time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SequenceNumber: 1,
					Departure:      time.Time{}.Add(3 * time.Millisecond),
					Arrival:        time.Time{}.Add(4 * time.Millisecond),
				},
				{
					SequenceNumber: 2,
					Departure:      time.Time{}.Add(6 * time.Millisecond),
					Arrival:        time.Time{}.Add(10 * time.Millisecond),
				},
				{
					SequenceNumber: 3,
					Departure:      time.Time{}.Add(9 * time.Millisecond),
					Arrival:        time.Time{}.Add(10 * time.Millisecond),
				},
				triggerNewGroupElement,
			},
			exp: []arrivalGroup{
				{
					departure: time.Time{},
					arrival:   time.Time{}.Add(4 * time.Millisecond),
				},
				{
					departure: time.Time{}.Add(6 * time.Millisecond),
					arrival:   time.Time{}.Add(10 * time.Millisecond),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aga := newArrivalGroupAccumulator()
			in := make(chan []cc.Acknowledgment)
			out := make(chan arrivalGroup)
			go func() {
				defer close(out)
				aga.run(in, func(ag arrivalGroup) {
					out <- ag
				})
			}()
			go func() {
				in <- tc.log
				close(in)
			}()
			received := []arrivalGroup{}
			for g := range out {
				received = append(received, g)
			}
			assert.Equal(t, tc.exp, received)
		})
	}
}

func TestArrivalGroupAccumulator_ConstantArrival(t *testing.T) {
	start := time.Unix(1, 0)
	acks := make([]cc.Acknowledgment, 10000)
	for i := range acks {
		acks[i] = cc.Acknowledgment{
			Departure: start.Add(time.Duration(i) * time.Microsecond),
			Arrival:   start,
		}
	}
	last := []cc.Acknowledgment{{Departure: start.Add(time.Second), Arrival: start.Add(time.Second)}}
	var got arrivalGroup
	groups := 0
	allocations := testing.AllocsPerRun(1, func() {
		groups = 0
		in := make(chan []cc.Acknowledgment, 2)
		in <- acks
		in <- last
		close(in)
		newArrivalGroupAccumulator().run(in, func(group arrivalGroup) {
			got = group
			groups++
		})
	})
	assert.LessOrEqual(t, allocations, float64(4))
	assert.Equal(t, 1, groups)
	assert.Equal(t, start, got.departure)
	assert.Equal(t, start, got.arrival)
}

func TestArrivalGroupAccumulatorTWCCRollover(t *testing.T) {
	adapter := cc.NewFeedbackAdapter()
	input := make(chan []cc.Acknowledgment, 3)
	for seq := range uint16(3) {
		header := &rtp.Header{}
		assert.NoError(t, header.SetExtension(1, []byte{0, byte(seq)}))
		departure := time.Time{}.Add(time.Duration(seq) * 64 * time.Millisecond)
		assert.NoError(t, adapter.OnSent(departure, header, 1200,
			interceptor.Attributes{cc.TwccExtensionAttributesKey: uint8(1)}))
		acks, err := adapter.OnTransportCCFeedback(departure, &rtcp.TransportLayerCC{
			BaseSequenceNumber: seq, PacketStatusCount: 1,
			ReferenceTime: (uint32(seq) + 0xffffff) & 0xffffff,
			PacketChunks: []rtcp.PacketStatusChunk{&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta, RunLength: 1,
			}},
			RecvDeltas: []*rtcp.RecvDelta{{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 250}},
		})
		assert.NoError(t, err)
		input <- acks
	}
	close(input)
	var groups []arrivalGroup
	newArrivalGroupAccumulator().run(input, func(group arrivalGroup) { groups = append(groups, group) })
	assert.Len(t, groups, 2)
	assert.Equal(t, 64*time.Millisecond, groups[1].arrival.Sub(groups[0].arrival))
}
