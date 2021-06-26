package cc

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/types"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

const hdrExtID = uint8(1)

func getPacketWithTransportCCExt(t *testing.T, SequenceNumber uint16) *rtp.Packet {
	pkt := rtp.Packet{
		Header:  rtp.Header{},
		Payload: []byte{},
	}
	ext := &rtp.TransportCCExtension{
		TransportSequence: SequenceNumber,
	}
	b, err := ext.Marshal()
	assert.NoError(t, err)
	assert.NoError(t, pkt.SetExtension(hdrExtID, b))
	return &pkt
}

func TestFeedbackAdapterTWCC(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		adapter := NewFeedbackAdapter()
		result, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{})
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("setsCorrectReceiveTime", func(t *testing.T) {
		t0 := time.Time{}
		adapter := NewFeedbackAdapter()
		headers := []rtp.Header{}
		for i := uint16(0); i < 22; i++ {
			pkt := getPacketWithTransportCCExt(t, i)
			headers = append(headers, pkt.Header)
			assert.NoError(t, adapter.OnSent(t0, &pkt.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		}
		results, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
			Header:             rtcp.Header{},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 0,
			PacketStatusCount:  22,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedLargeDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
					},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeOneBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
					},
				},
				&rtcp.RunLengthChunk{
					Type:               rtcp.TypeTCCRunLengthChunk,
					PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
					RunLength:          1,
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4, // 4*250us=1ms
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
					Delta: 100,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 12, // 3*4*250us=3ms
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
			},
		})

		assert.NoError(t, err)

		assert.NotEmpty(t, results)
		assert.Len(t, results, 22)

		assert.Contains(t, results, types.PacketResult{
			SentPacket: types.SentPacket{
				SendTime: t0,
				Header:   &headers[0],
			},
			ReceiveTime: t0.Add(time.Millisecond),
			Received:    true,
		})

		assert.Contains(t, results, types.PacketResult{
			SentPacket: types.SentPacket{
				SendTime: t0,
				Header:   &headers[1],
			},
			ReceiveTime: t0.Add(101 * time.Millisecond),
			Received:    true,
		})

		for i := uint16(2); i < 7; i++ {
			assert.Contains(t, results, types.PacketResult{
				SentPacket: types.SentPacket{
					SendTime: t0,
					Header:   &headers[i],
				},
				ReceiveTime: time.Time{},
				Received:    false,
			})
		}

		assert.Contains(t, results, types.PacketResult{
			SentPacket: types.SentPacket{
				SendTime: t0,
				Header:   &headers[7],
			},
			ReceiveTime: t0.Add(104 * time.Millisecond),
			Received:    true,
		})

		for i := uint16(8); i < 21; i++ {
			assert.Contains(t, results, types.PacketResult{
				SentPacket: types.SentPacket{
					SendTime: t0,
					Header:   &headers[i],
				},
				ReceiveTime: time.Time{},
				Received:    false,
			})
		}

		assert.Contains(t, results, types.PacketResult{
			SentPacket: types.SentPacket{
				SendTime: t0,
				Header:   &headers[21],
			},
			ReceiveTime: t0.Add(105 * time.Millisecond),
			Received:    true,
		})
	})

	t.Run("doesNotCrashOnTooManyFeedbackReports", func(*testing.T) {
		adapter := NewFeedbackAdapter()
		assert.NotPanics(t, func() {
			_, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
				Header:             rtcp.Header{},
				SenderSSRC:         0,
				MediaSSRC:          0,
				BaseSequenceNumber: 0,
				PacketStatusCount:  0,
				ReferenceTime:      0,
				FbPktCount:         0,
				PacketChunks: []rtcp.PacketStatusChunk{
					&rtcp.StatusVectorChunk{
						PacketStatusChunk: nil,
						Type:              rtcp.TypeTCCStatusVectorChunk,
						SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
						SymbolList: []uint16{
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
							rtcp.TypeTCCPacketNotReceived,
						},
					},
				},
				RecvDeltas: []*rtcp.RecvDelta{
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 4, // 4*250us=1ms
					},
				},
			})
			assert.NoError(t, err)
		})
	})

	t.Run("worksOnSequenceNumberWrapAround", func(t *testing.T) {
		t0 := time.Time{}
		adapter := NewFeedbackAdapter()
		pkt65535 := getPacketWithTransportCCExt(t, 65535)
		pkt0 := getPacketWithTransportCCExt(t, 0)
		assert.NoError(t, adapter.OnSent(t0, &pkt65535.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		assert.NoError(t, adapter.OnSent(t0, &pkt0.Header, interceptor.Attributes{twccExtension: hdrExtID}))

		results, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
			Header:             rtcp.Header{},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 65535,
			PacketStatusCount:  2,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
					},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
			},
		})
		assert.NoError(t, err)

		assert.NotEmpty(t, results)
		assert.Len(t, results, 2)
		assert.Contains(t, results, types.PacketResult{
			SentPacket: types.SentPacket{
				SendTime: t0,
				Header:   &pkt65535.Header,
			},
			ReceiveTime: t0.Add(1 * time.Millisecond),
			Received:    true,
		})
		assert.Contains(t, results, types.PacketResult{
			SentPacket: types.SentPacket{
				SendTime: t0,
				Header:   &pkt0.Header,
			},
			ReceiveTime: t0.Add(2 * time.Millisecond),
			Received:    true,
		})
	})

	t.Run("ignoresPossiblyInFlightPackets", func(t *testing.T) {
		t0 := time.Time{}
		adapter := NewFeedbackAdapter()
		headers := []rtp.Header{}
		for i := uint16(0); i < 8; i++ {
			pkt := getPacketWithTransportCCExt(t, i)
			headers = append(headers, pkt.Header)
			assert.NoError(t, adapter.OnSent(t0, &pkt.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		}

		results, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
			Header:             rtcp.Header{},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 0,
			PacketStatusCount:  3,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
					},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4, // 4*250us=1ms
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4, // 4*250us=1ms
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4, // 4*250us=1ms
				},
			},
		})
		assert.NoError(t, err)
		assert.Len(t, results, 3)
		for i := uint16(0); i < 3; i++ {
			assert.Contains(t, results, types.PacketResult{
				SentPacket: types.SentPacket{
					SendTime: t0,
					Header:   &headers[i],
				},
				ReceiveTime: t0.Add(time.Duration(i+1) * time.Millisecond),
				Received:    true,
			})
		}
	})

	t.Run("runLengthChunk", func(t *testing.T) {
		adapter := NewFeedbackAdapter()
		t0 := time.Time{}
		for i := uint16(0); i < 20; i++ {
			pkt := getPacketWithTransportCCExt(t, i)
			assert.NoError(t, adapter.OnSent(t0, &pkt.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		}
		packets, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
			Header:             rtcp.Header{},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 0,
			PacketStatusCount:  3,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.RunLengthChunk{
					PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
					RunLength:          3,
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
			},
		})

		assert.NoError(t, err)
		assert.Len(t, packets, 3)
	})

	t.Run("statusVectorChunk", func(t *testing.T) {
		adapter := NewFeedbackAdapter()
		t0 := time.Time{}
		for i := uint16(0); i < 20; i++ {
			pkt := getPacketWithTransportCCExt(t, i)
			assert.NoError(t, adapter.OnSent(t0, &pkt.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		}
		packets, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
			Header:             rtcp.Header{},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 0,
			PacketStatusCount:  3,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks: []rtcp.PacketStatusChunk{
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
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
					},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
			},
		})

		assert.NoError(t, err)
		assert.Len(t, packets, 3)
	})

	t.Run("mixedRunLengthAndStatusVector", func(t *testing.T) {
		adapter := NewFeedbackAdapter()

		t0 := time.Time{}
		for i := uint16(0); i < 20; i++ {
			pkt := getPacketWithTransportCCExt(t, i)
			assert.NoError(t, adapter.OnSent(t0, &pkt.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		}

		packets, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
			Header:             rtcp.Header{},
			SenderSSRC:         0,
			MediaSSRC:          0,
			BaseSequenceNumber: 0,
			PacketStatusCount:  10,
			ReferenceTime:      0,
			FbPktCount:         0,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketNotReceived,
					},
				},
				&rtcp.RunLengthChunk{
					PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
					RunLength:          3,
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
				{
					Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
					Delta: 4,
				},
			},
		})
		assert.NoError(t, err)
		assert.Len(t, packets, 10)
	})

	t.Run("doesNotcrashOnInvalidTWCCPacket", func(t *testing.T) {

		adapter := NewFeedbackAdapter()

		t0 := time.Time{}
		for i := uint16(1008); i < 1030; i++ {
			pkt := getPacketWithTransportCCExt(t, i)
			assert.NoError(t, adapter.OnSent(t0, &pkt.Header, interceptor.Attributes{twccExtension: hdrExtID}))
		}

		assert.NotPanics(t, func() {
			// TODO(mathis): Run length seems off, maybe check why TWCC generated this?
			packets, err := adapter.OnIncomingTransportCC(&rtcp.TransportLayerCC{
				Header:             rtcp.Header{},
				SenderSSRC:         0,
				MediaSSRC:          0,
				BaseSequenceNumber: 1008,
				PacketStatusCount:  8,
				ReferenceTime:      278,
				FbPktCount:         170,
				PacketChunks: []rtcp.PacketStatusChunk{
					&rtcp.StatusVectorChunk{
						SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
						SymbolList: []uint16{
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketReceivedSmallDelta,
							rtcp.TypeTCCPacketNotReceived,
						},
					},
					&rtcp.RunLengthChunk{
						PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
						RunLength:          5632,
					},
				},
				RecvDeltas: []*rtcp.RecvDelta{
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 25000,
					},
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 0,
					},
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 29500,
					},
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 16750,
					},
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 23500,
					},
					{
						Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
						Delta: 0,
					},
				},
			})
			assert.Error(t, err)
			assert.Empty(t, packets)
		})
	})
}
