// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

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
		expect   []acknowledgement
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
			expect: nil,
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
			expect: []acknowledgement{
				// first run length chunk
				{sequenceNumber: 178, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 1*time.Millisecond), ecn: 0},
				{sequenceNumber: 179, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 2*time.Millisecond), ecn: 0},
				{sequenceNumber: 180, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 3*time.Millisecond), ecn: 0},

				// first status vector chunk
				{sequenceNumber: 181, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 4*time.Millisecond), ecn: 0},
				{sequenceNumber: 182, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 5*time.Millisecond), ecn: 0},
				{sequenceNumber: 183, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 6*time.Millisecond), ecn: 0},
				{sequenceNumber: 184, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 185, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 186, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 187, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 188, arrived: false, arrival: time.Time{}, ecn: 0},

				// second status vector chunk
				{sequenceNumber: 189, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 7*time.Millisecond), ecn: 0},
				{sequenceNumber: 190, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 8*time.Millisecond), ecn: 0},
				{sequenceNumber: 191, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 192, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 193, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 194, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 195, arrived: false, arrival: time.Time{}, ecn: 0},
			},
		},
		{
			// PacketStatusCount smaller than the number of chunks.
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 200,
				PacketStatusCount:  4,
				ReferenceTime:      3,
				FbPktCount:         0,
				PacketChunks: []rtcp.PacketStatusChunk{
					&rtcp.RunLengthChunk{
						PacketStatusSymbol: rtcp.TypeTCCPacketNotReceived,
						RunLength:          1,
					},
					&rtcp.RunLengthChunk{
						PacketStatusSymbol: rtcp.TypeTCCPacketReceivedWithoutDelta,
						RunLength:          1,
					},
					&rtcp.RunLengthChunk{
						PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
						RunLength:          5,
					},
					&rtcp.StatusVectorChunk{
						SymbolSize: rtcp.TypeTCCSymbolSizeOneBit,
						SymbolList: []uint16{
							rtcp.TypeTCCPacketNotReceived,
						},
					},
				},
				RecvDeltas: []*rtcp.RecvDelta{},
			},
			expect: []acknowledgement{
				{sequenceNumber: 200, arrived: false, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 201, arrived: true, arrival: time.Time{}, ecn: 0},
			},
		},
		{
			// PacketStatusCount smaller than the number of actual chunk and
			// fewer RecvDeltas than symbols requiring one.
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 300,
				PacketStatusCount:  3,
				ReferenceTime:      3,
				FbPktCount:         0,
				PacketChunks: []rtcp.PacketStatusChunk{
					&rtcp.StatusVectorChunk{
						SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
						SymbolList: []uint16{
							rtcp.TypeTCCPacketReceivedWithoutDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
						},
					},
				},
				RecvDeltas: []*rtcp.RecvDelta{
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
				},
			},
			expect: []acknowledgement{
				{sequenceNumber: 300, arrived: true, arrival: time.Time{}, ecn: 0},
				{sequenceNumber: 301, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 1*time.Millisecond), ecn: 0},
			},
		},
		{
			// PacketStatusCount is larger than the number of actual chunks.
			feedback: &rtcp.TransportLayerCC{
				SenderSSRC:         1,
				MediaSSRC:          2,
				BaseSequenceNumber: 400,
				PacketStatusCount:  10,
				ReferenceTime:      3,
				FbPktCount:         0,
				PacketChunks: []rtcp.PacketStatusChunk{
					&rtcp.RunLengthChunk{
						PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
						RunLength:          2,
					},
				},
				RecvDeltas: []*rtcp.RecvDelta{
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
					{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
				},
			},
			expect: []acknowledgement{
				{sequenceNumber: 400, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 1*time.Millisecond), ecn: 0},
				{sequenceNumber: 401, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 2*time.Millisecond), ecn: 0},
			},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			res := convertTWCC(tc.feedback)
			assert.Equal(t, tc.expect, res)
		})
	}
}

func TestConvertTWCCUncappedRunLength(t *testing.T) {
	packet := &rtcp.TransportLayerCC{
		Header: rtcp.Header{
			Type:  rtcp.TypeTransportSpecificFeedback,
			Count: rtcp.FormatTCC,
		},
		SenderSSRC:         1,
		MediaSSRC:          2,
		BaseSequenceNumber: 178,
		PacketStatusCount:  1,
		ReferenceTime:      3,
		FbPktCount:         0,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
				RunLength:          100,
			},
		},
		RecvDeltas: []*rtcp.RecvDelta{
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 1000},
		},
	}
	packet.Header.Length = uint16(packet.MarshalSize()/4 - 1) // nolint:gosec

	raw, err := packet.Marshal()
	assert.NoError(t, err)

	unmarshalled := &rtcp.TransportLayerCC{}
	assert.NoError(t, unmarshalled.Unmarshal(raw))
	assert.Equal(t, uint16(1), unmarshalled.PacketStatusCount)
	assert.Len(t, unmarshalled.RecvDeltas, 1)

	assert.NotPanics(t, func() {
		res := convertTWCC(unmarshalled)
		assert.Equal(t, []acknowledgement{
			{sequenceNumber: 178, arrived: true, arrival: time.Time{}.Add(3*64*time.Millisecond + 1*time.Millisecond), ecn: 0},
		}, res)
	})
}
