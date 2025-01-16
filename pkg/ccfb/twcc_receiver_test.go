package ccfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

func TestConvertTWCC(t *testing.T) {
	timeZero := time.Now()
	cases := []struct {
		ts       time.Time
		feedback *rtcp.TransportLayerCC
		expect   map[uint32]acknowledgementList
	}{
		{},
		{
			ts: timeZero.Add(2 * time.Second),
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 178,
				PacketStatusCount:  0,
				ReferenceTime:      0,
				FbPktCount:         0,
				PacketChunks:       []rtcp.PacketStatusChunk{},
				RecvDeltas:         []*rtcp.RecvDelta{},
			},
			expect: map[uint32]acknowledgementList{
				2: {
					ts:   timeZero.Add(2 * time.Second),
					acks: []acknowledgement{},
				},
			},
		},
		{
			ts: timeZero.Add(2 * time.Second),
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 178,
				PacketStatusCount:  3,
				ReferenceTime:      0,
				FbPktCount:         0,
				PacketChunks: []rtcp.PacketStatusChunk{
					&rtcp.RunLengthChunk{
						PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
						RunLength:          3,
					},
					&rtcp.StatusVectorChunk{
						SymbolSize: rtcp.TypeTCCSymbolSizeOneBit,
						SymbolList: []uint16{
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
						},
					},
					&rtcp.StatusVectorChunk{
						SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
						SymbolList: []uint16{
							rtcp.TypeTCCPacketReceivedLargeDelta,
							rtcp.TypeTCCPacketReceivedLargeDelta,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
						},
					},
				},
				RecvDeltas: []*rtcp.RecvDelta{
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedLargeDelta, Delta: 0},
					{Type: rtcp.TypeTCCPacketReceivedLargeDelta, Delta: 0},
				},
			},
			expect: map[uint32]acknowledgementList{
				2: {
					ts: timeZero.Add(2 * time.Second),
					acks: []acknowledgement{
						// first run length chunk
						{seqNr: 178, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 179, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 180, arrived: true, arrival: time.Time{}, ecn: 0},

						// first status vector chunk
						{seqNr: 181, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 182, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 183, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 184, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 185, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 186, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 187, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 188, arrived: false, arrival: time.Time{}, ecn: 0},

						// second status vector chunk
						{seqNr: 189, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 190, arrived: true, arrival: time.Time{}, ecn: 0},
						{seqNr: 191, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 192, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 193, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 194, arrived: false, arrival: time.Time{}, ecn: 0},
						{seqNr: 195, arrived: false, arrival: time.Time{}, ecn: 0},
					},
				},
			},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			res := convertTWCC(tc.ts, tc.feedback)

			// Can't directly check equality since arrival timestamp conversions
			// may be slightly off due to ntp conversions.
			assert.Equal(t, len(tc.expect), len(res))
			for i, ee := range tc.expect {
				assert.Equal(t, ee.ts, res[i].ts)
				for j, ack := range ee.acks {
					assert.Equal(t, ack.seqNr, res[i].acks[j].seqNr)
					assert.Equal(t, ack.arrived, res[i].acks[j].arrived)
					assert.Equal(t, ack.ecn, res[i].acks[j].ecn)
					assert.InDelta(t, ack.arrival.UnixNano(), res[i].acks[j].arrival.UnixNano(), float64(time.Millisecond.Nanoseconds()))
				}
			}
		})
	}

}
