package ccfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

func TestConvertTWCC(t *testing.T) {
	// timeZero := time.Now()
	cases := []struct {
		feedback *rtcp.TransportLayerCC
		expect   map[uint32][]acknowledgement
		expectTS time.Time
	}{
		{},
		{
			// ts: timeZero.Add(2 * time.Second),
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 178,
				PacketStatusCount:  0,
				ReferenceTime:      3,
				FbPktCount:         0,
				PacketChunks:       []rtcp.PacketStatusChunk{},
				RecvDeltas:         []*rtcp.RecvDelta{},
			},
			expect: map[uint32][]acknowledgement{
				0: nil,
			},
			expectTS: time.Time{}.Add(3 * 64 * time.Millisecond),
		},
		{
			// ts: timeZero.Add(2 * time.Second),
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 178,
				PacketStatusCount:  18,
				ReferenceTime:      3,
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
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedLargeDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedLargeDelta, Delta: 1000},
				},
			},
			expect: map[uint32][]acknowledgement{
				0: {
					// first run length chunk
					{seqNr: 178, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 1*time.Millisecond), ecn: 0},
					{seqNr: 179, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 2*time.Millisecond), ecn: 0},
					{seqNr: 180, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 3*time.Millisecond), ecn: 0},

					// first status vector chunk
					{seqNr: 181, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 4*time.Millisecond), ecn: 0},
					{seqNr: 182, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 5*time.Millisecond), ecn: 0},
					{seqNr: 183, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 6*time.Millisecond), ecn: 0},
					{seqNr: 184, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 185, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 186, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 187, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 188, arrived: false, arrival: time.Time{}, ecn: 0},

					// second status vector chunk
					{seqNr: 189, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 7*time.Millisecond), ecn: 0},
					{seqNr: 190, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 8*time.Millisecond), ecn: 0},
					{seqNr: 191, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 192, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 193, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 194, arrived: false, arrival: time.Time{}, ecn: 0},
					{seqNr: 195, arrived: false, arrival: time.Time{}, ecn: 0},
				},
			},
			expectTS: time.Time{}.Add(3 * 64 * time.Millisecond),
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			resTS, res := convertTWCC(tc.feedback)
			assert.Equal(t, tc.expect, res)
			assert.Equal(t, tc.expectTS, resTS)
		})
	}
}
