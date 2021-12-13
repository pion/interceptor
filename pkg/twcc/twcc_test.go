package twcc

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

func Test_chunk_add(t *testing.T) {
	t.Run("fill with not received", func(t *testing.T) {
		c := &chunk{}

		for i := 0; i < maxRunLengthCap; i++ {
			assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived), i)
			c.add(rtcp.TypeTCCPacketNotReceived)
		}
		assert.Equal(t, make([]uint16, maxRunLengthCap), c.deltas)
		assert.False(t, c.hasDifferentTypes)

		assert.False(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		assert.False(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		assert.False(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))

		statusChunk := c.encode()
		assert.IsType(t, &rtcp.RunLengthChunk{}, statusChunk)

		buf, err := statusChunk.Marshal()
		assert.NoError(t, err)
		assert.Equal(t, []byte{0x1f, 0xff}, buf)
	})

	t.Run("fill with small delta", func(t *testing.T) {
		c := &chunk{}

		for i := 0; i < maxOneBitCap; i++ {
			assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta), i)
			c.add(rtcp.TypeTCCPacketReceivedSmallDelta)
		}

		assert.Equal(t, c.deltas, []uint16{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
		assert.False(t, c.hasDifferentTypes)

		assert.False(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))
		assert.False(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))

		statusChunk := c.encode()
		assert.IsType(t, &rtcp.RunLengthChunk{}, statusChunk)

		buf, err := statusChunk.Marshal()
		assert.NoError(t, err)
		assert.Equal(t, []byte{0x20, 0xe}, buf)
	})

	t.Run("fill with large delta", func(t *testing.T) {
		c := &chunk{}

		for i := 0; i < maxTwoBitCap; i++ {
			assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta), i)
			c.add(rtcp.TypeTCCPacketReceivedLargeDelta)
		}

		assert.Equal(t, c.deltas, []uint16{2, 2, 2, 2, 2, 2, 2})
		assert.True(t, c.hasLargeDelta)
		assert.False(t, c.hasDifferentTypes)

		assert.False(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		assert.False(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))

		statusChunk := c.encode()
		assert.IsType(t, &rtcp.RunLengthChunk{}, statusChunk)

		buf, err := statusChunk.Marshal()
		assert.NoError(t, err)
		assert.Equal(t, []byte{0x40, 0x7}, buf)
	})

	t.Run("fill with different types", func(t *testing.T) {
		c := &chunk{}

		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		c.add(rtcp.TypeTCCPacketReceivedSmallDelta)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		c.add(rtcp.TypeTCCPacketReceivedSmallDelta)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		c.add(rtcp.TypeTCCPacketReceivedSmallDelta)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		c.add(rtcp.TypeTCCPacketReceivedSmallDelta)

		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))
		c.add(rtcp.TypeTCCPacketReceivedLargeDelta)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))
		c.add(rtcp.TypeTCCPacketReceivedLargeDelta)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))
		c.add(rtcp.TypeTCCPacketReceivedLargeDelta)

		assert.False(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))

		statusChunk := c.encode()
		assert.IsType(t, &rtcp.StatusVectorChunk{}, statusChunk)

		buf, err := statusChunk.Marshal()
		assert.NoError(t, err)
		assert.Equal(t, []byte{0xd5, 0x6a}, buf)
	})

	t.Run("overfill and encode", func(t *testing.T) {
		c := chunk{}

		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedSmallDelta))
		c.add(rtcp.TypeTCCPacketReceivedSmallDelta)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)
		assert.True(t, c.canAdd(rtcp.TypeTCCPacketNotReceived))
		c.add(rtcp.TypeTCCPacketNotReceived)

		assert.False(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))

		statusChunk1 := c.encode()
		assert.IsType(t, &rtcp.StatusVectorChunk{}, statusChunk1)
		assert.Equal(t, 1, len(c.deltas))

		assert.True(t, c.canAdd(rtcp.TypeTCCPacketReceivedLargeDelta))
		c.add(rtcp.TypeTCCPacketReceivedLargeDelta)

		statusChunk2 := c.encode()
		assert.IsType(t, &rtcp.StatusVectorChunk{}, statusChunk2)

		assert.Equal(t, 0, len(c.deltas))

		assert.Equal(t, &rtcp.StatusVectorChunk{
			SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
			SymbolList: []uint16{rtcp.TypeTCCPacketNotReceived, rtcp.TypeTCCPacketReceivedLargeDelta},
		}, statusChunk2)
	})
}

func Test_feedback(t *testing.T) {
	t.Run("add simple", func(t *testing.T) {
		f := feedback{}

		got := f.addReceived(0, 10)

		assert.True(t, got)
	})

	t.Run("add too large", func(t *testing.T) {
		f := feedback{}

		assert.False(t, f.addReceived(12, 8200*1000*250))
	})

	t.Run("add received 1", func(t *testing.T) {
		f := &feedback{}
		f.setBase(1, 1000*1000)

		got := f.addReceived(1, 1023*1000)

		assert.True(t, got)
		assert.Equal(t, uint16(2), f.nextSequenceNumber)
		assert.Equal(t, int64(15), f.refTimestamp64MS)

		got = f.addReceived(4, 1086*1000)
		assert.True(t, got)
		assert.Equal(t, uint16(5), f.nextSequenceNumber)
		assert.Equal(t, int64(15), f.refTimestamp64MS)

		assert.True(t, f.lastChunk.hasDifferentTypes)
		assert.Equal(t, 4, len(f.lastChunk.deltas))
		assert.NotContains(t, f.lastChunk.deltas, rtcp.TypeTCCPacketReceivedLargeDelta)
	})

	t.Run("add received 2", func(t *testing.T) {
		f := newFeedback(0, 0, 0)
		f.setBase(5, 320*1000)

		got := f.addReceived(5, 320*1000)
		assert.True(t, got)
		got = f.addReceived(7, 448*1000)
		assert.True(t, got)
		got = f.addReceived(8, 512*1000)
		assert.True(t, got)
		got = f.addReceived(11, 768*1000)
		assert.True(t, got)

		pkt := f.getRTCP()

		assert.True(t, pkt.Header.Padding)
		assert.Equal(t, uint16(7), pkt.Header.Length)
		assert.Equal(t, uint16(5), pkt.BaseSequenceNumber)
		assert.Equal(t, uint16(7), pkt.PacketStatusCount)
		assert.Equal(t, uint32(5), pkt.ReferenceTime)
		assert.Equal(t, uint8(0), pkt.FbPktCount)
		assert.Equal(t, 1, len(pkt.PacketChunks))

		assert.Equal(t, []rtcp.PacketStatusChunk{&rtcp.StatusVectorChunk{
			SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
			SymbolList: []uint16{
				rtcp.TypeTCCPacketReceivedSmallDelta,
				rtcp.TypeTCCPacketNotReceived,
				rtcp.TypeTCCPacketReceivedLargeDelta,
				rtcp.TypeTCCPacketReceivedLargeDelta,
				rtcp.TypeTCCPacketNotReceived,
				rtcp.TypeTCCPacketNotReceived,
				rtcp.TypeTCCPacketReceivedLargeDelta,
			},
		}}, pkt.PacketChunks)

		expectedDeltas := []*rtcp.RecvDelta{
			{
				Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
				Delta: 0,
			},
			{
				Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
				Delta: 0x0200 * rtcp.TypeTCCDeltaScaleFactor,
			},
			{
				Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
				Delta: 0x0100 * rtcp.TypeTCCDeltaScaleFactor,
			},
			{
				Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
				Delta: 0x0400 * rtcp.TypeTCCDeltaScaleFactor,
			},
		}
		assert.Equal(t, len(expectedDeltas), len(pkt.RecvDeltas))
		for i, d := range expectedDeltas {
			assert.Equal(t, d, pkt.RecvDeltas[i])
		}
	})

	t.Run("add received wrapped sequence number", func(t *testing.T) {
		f := newFeedback(0, 0, 0)
		f.setBase(65535, 320*1000)

		got := f.addReceived(65535, 320*1000)
		assert.True(t, got)
		got = f.addReceived(7, 448*1000)
		assert.True(t, got)
		got = f.addReceived(8, 512*1000)
		assert.True(t, got)
		got = f.addReceived(11, 768*1000)
		assert.True(t, got)

		pkt := f.getRTCP()

		assert.True(t, pkt.Header.Padding)
		assert.Equal(t, uint16(7), pkt.Header.Length)
		assert.Equal(t, uint16(65535), pkt.BaseSequenceNumber)
		assert.Equal(t, uint16(13), pkt.PacketStatusCount)
		assert.Equal(t, uint32(5), pkt.ReferenceTime)
		assert.Equal(t, uint8(0), pkt.FbPktCount)
		assert.Equal(t, 2, len(pkt.PacketChunks))

		assert.Equal(t, []rtcp.PacketStatusChunk{
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
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
			&rtcp.StatusVectorChunk{
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedLargeDelta,
					rtcp.TypeTCCPacketReceivedLargeDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedLargeDelta,
				},
			},
		}, pkt.PacketChunks)

		expectedDeltas := []*rtcp.RecvDelta{
			{
				Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
				Delta: 0,
			},
			{
				Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
				Delta: 0x0200 * rtcp.TypeTCCDeltaScaleFactor,
			},
			{
				Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
				Delta: 0x0100 * rtcp.TypeTCCDeltaScaleFactor,
			},
			{
				Type:  rtcp.TypeTCCPacketReceivedLargeDelta,
				Delta: 0x0400 * rtcp.TypeTCCDeltaScaleFactor,
			},
		}
		assert.Equal(t, len(expectedDeltas), len(pkt.RecvDeltas))
		for i, d := range expectedDeltas {
			assert.Equal(t, d, pkt.RecvDeltas[i])
		}
	})

	t.Run("get RTCP", func(t *testing.T) {
		testcases := []struct {
			arrivalTS              int64
			sequenceNumber         uint16
			wantRefTime            uint32
			wantBaseSequenceNumber uint16
		}{
			{320, 1, 5, 1},
			{1000, 2, 15, 2},
		}
		for _, tt := range testcases {
			tt := tt

			t.Run("set correct base seq and time", func(t *testing.T) {
				f := newFeedback(0, 0, 0)
				f.setBase(tt.sequenceNumber, tt.arrivalTS*1000)

				got := f.getRTCP()
				assert.Equal(t, tt.wantRefTime, got.ReferenceTime)
				assert.Equal(t, tt.wantBaseSequenceNumber, got.BaseSequenceNumber)
			})
		}
	})
}

func addRun(t *testing.T, r *Recorder, sequenceNumbers []uint16, arrivalTimes []int64) {
	assert.Equal(t, len(sequenceNumbers), len(arrivalTimes))

	for i := range sequenceNumbers {
		r.Record(5000, sequenceNumbers[i], arrivalTimes[i])
	}
}

const (
	scaleFactorReferenceTime = 64000
)

func increaseTime(arrivalTime *int64, increaseAmount int64) int64 {
	*arrivalTime += increaseAmount
	return *arrivalTime
}

func marshalAll(t *testing.T, pkts []rtcp.Packet) {
	for _, pkt := range pkts {
		_, err := pkt.Marshal()
		assert.NoError(t, err)
	}
}

func TestBuildFeedbackPacket(t *testing.T) {
	r := NewRecorder(5000)

	arrivalTime := int64(scaleFactorReferenceTime)
	addRun(t, r, []uint16{0, 1, 2, 3, 4, 5, 6, 7}, []int64{
		scaleFactorReferenceTime,
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor*256),
	})

	rtcpPackets := r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))

	assert.Equal(t, &rtcp.TransportLayerCC{
		Header: rtcp.Header{
			Count:   rtcp.FormatTCC,
			Type:    rtcp.TypeTransportSpecificFeedback,
			Padding: true,
			Length:  8,
		},
		SenderSSRC:         5000,
		MediaSSRC:          5000,
		BaseSequenceNumber: 0,
		ReferenceTime:      1,
		FbPktCount:         0,
		PacketStatusCount:  8,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.RunLengthChunk{
				Type:               rtcp.TypeTCCRunLengthChunk,
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
				RunLength:          7,
			},
			&rtcp.RunLengthChunk{
				Type:               rtcp.TypeTCCRunLengthChunk,
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedLargeDelta,
				RunLength:          1,
			},
		},
		RecvDeltas: []*rtcp.RecvDelta{
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedLargeDelta, Delta: rtcp.TypeTCCDeltaScaleFactor * 256},
		},
	}, rtcpPackets[0].(*rtcp.TransportLayerCC))
	marshalAll(t, rtcpPackets)
}

func TestBuildFeedbackPacket_Rolling(t *testing.T) {
	r := NewRecorder(5000)

	arrivalTime := int64(scaleFactorReferenceTime)
	addRun(t, r, []uint16{65535}, []int64{
		arrivalTime,
	})

	rtcpPackets := r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))

	addRun(t, r, []uint16{4, 8, 9, 10}, []int64{
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
	})

	rtcpPackets = r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))

	assert.Equal(t, &rtcp.TransportLayerCC{
		Header: rtcp.Header{
			Count:   rtcp.FormatTCC,
			Type:    rtcp.TypeTransportSpecificFeedback,
			Padding: true,
			Length:  6,
		},
		SenderSSRC:         5000,
		MediaSSRC:          5000,
		BaseSequenceNumber: 4,
		ReferenceTime:      1,
		FbPktCount:         1,
		PacketStatusCount:  7,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.StatusVectorChunk{
				Type:       rtcp.TypeTCCRunLengthChunk,
				SymbolSize: rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList: []uint16{
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketNotReceived,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
					rtcp.TypeTCCPacketReceivedSmallDelta,
				},
			},
		},
		RecvDeltas: []*rtcp.RecvDelta{
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
		},
	}, rtcpPackets[0].(*rtcp.TransportLayerCC))
	marshalAll(t, rtcpPackets)
}

func TestBuildFeedbackPacketCount(t *testing.T) {
	r := NewRecorder(5000)

	arrivalTime := int64(scaleFactorReferenceTime)
	addRun(t, r, []uint16{0}, []int64{
		arrivalTime,
	})

	pkts := r.BuildFeedbackPacket()
	assert.Len(t, pkts, 1)

	twcc := pkts[0].(*rtcp.TransportLayerCC)
	assert.Equal(t, uint8(0), twcc.FbPktCount)

	addRun(t, r, []uint16{0}, []int64{
		arrivalTime,
	})

	pkts = r.BuildFeedbackPacket()
	assert.Len(t, pkts, 1)

	twcc = pkts[0].(*rtcp.TransportLayerCC)
	assert.Equal(t, uint8(1), twcc.FbPktCount)
}

func TestDuplicatePackets(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		r := NewRecorder(5000)

		arrivalTime := int64(scaleFactorReferenceTime)
		addRun(t, r, []uint16{12, 13, 13, 14}, []int64{
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
		})

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  6,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 12,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  3,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.RunLengthChunk{
					PacketStatusChunk:  nil,
					Type:               rtcp.TypeTCCRunLengthChunk,
					PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
					RunLength:          3,
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			},
		}, rtcpPackets[0].(*rtcp.TransportLayerCC))
		marshalAll(t, rtcpPackets)
	})

	t.Run("2", func(t *testing.T) {
		r := NewRecorder(5000)

		arrivalTime := int64(scaleFactorReferenceTime)
		addRun(t, r, []uint16{12, 14, 14, 15}, []int64{
			increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
			increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
			increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
			increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		})

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  6,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 12,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  4,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              0,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
					},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			},
		}, rtcpPackets[0].(*rtcp.TransportLayerCC))
		marshalAll(t, rtcpPackets)
	})

	t.Run("3", func(t *testing.T) {
		r := NewRecorder(5000)

		arrivalTime := int64(scaleFactorReferenceTime)
		addRun(t, r, []uint16{12, 14, 14, 15}, []int64{
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
		})

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  6,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 12,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  4,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              0,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketNotReceived,
						rtcp.TypeTCCPacketReceivedSmallDelta,
						rtcp.TypeTCCPacketReceivedSmallDelta,
					},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			},
		}, rtcpPackets[0].(*rtcp.TransportLayerCC))
		marshalAll(t, rtcpPackets)
	})

	t.Run("4", func(t *testing.T) {
		r := NewRecorder(5000)

		sequenceNumbers := []uint16{
			22, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 38, 39, 40, 41, 42, 44, 44, 45, 46, 48, 48, 50, 50, 52, 52, 54, 54, 56, 56, 57, 58, 60, 60, 61, 62, 63, 64, 65, 66, 67, 68, 71, 72,
		}
		arrivalTime := int64(scaleFactorReferenceTime)
		for i := range sequenceNumbers {
			r.Record(5000, sequenceNumbers[i], increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor))
		}

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		pkt := rtcpPackets[0].(*rtcp.TransportLayerCC)
		bs, err := pkt.Marshal()
		unmarshalled := &rtcp.TransportLayerCC{}
		assert.NoError(t, err)
		assert.NoError(t, unmarshalled.Unmarshal(bs))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  17,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 22,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  51,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeOneBit,
					SymbolList: []uint16{
						// [1 0 1 1 1 1 1 1 1 1 1 1 1 1]
						rtcp.TypeTCCPacketReceivedSmallDelta, // 22
						rtcp.TypeTCCPacketNotReceived,        // 23
						rtcp.TypeTCCPacketReceivedSmallDelta, // 24
						rtcp.TypeTCCPacketReceivedSmallDelta, // 25
						rtcp.TypeTCCPacketReceivedSmallDelta, // 26
						rtcp.TypeTCCPacketReceivedSmallDelta, // 27
						rtcp.TypeTCCPacketReceivedSmallDelta, // 28
						rtcp.TypeTCCPacketReceivedSmallDelta, // 29
						rtcp.TypeTCCPacketReceivedSmallDelta, // 30
						rtcp.TypeTCCPacketReceivedSmallDelta, // 31
						rtcp.TypeTCCPacketReceivedSmallDelta, // 32
						rtcp.TypeTCCPacketReceivedSmallDelta, // 33
						rtcp.TypeTCCPacketReceivedSmallDelta, // 34
						rtcp.TypeTCCPacketReceivedSmallDelta, // 35
					},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeOneBit,
					SymbolList: []uint16{
						// [1 0 1 1 1 1 1 0 1 1 1 0 1 0]
						rtcp.TypeTCCPacketReceivedSmallDelta, // 36
						rtcp.TypeTCCPacketNotReceived,        // 37
						rtcp.TypeTCCPacketReceivedSmallDelta, // 38
						rtcp.TypeTCCPacketReceivedSmallDelta, // 39
						rtcp.TypeTCCPacketReceivedSmallDelta, // 40
						rtcp.TypeTCCPacketReceivedSmallDelta, // 41
						rtcp.TypeTCCPacketReceivedSmallDelta, // 42
						rtcp.TypeTCCPacketNotReceived,        // 43
						rtcp.TypeTCCPacketReceivedSmallDelta, // 44
						rtcp.TypeTCCPacketReceivedSmallDelta, // 45
						rtcp.TypeTCCPacketReceivedSmallDelta, // 46
						rtcp.TypeTCCPacketNotReceived,        // 47
						rtcp.TypeTCCPacketReceivedSmallDelta, // 48
						rtcp.TypeTCCPacketNotReceived,        // 49
					},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeOneBit,
					SymbolList: []uint16{
						// [1 0 1 0 1 0 1 1 1 0 1 1 1 1]
						rtcp.TypeTCCPacketReceivedSmallDelta, // 50
						rtcp.TypeTCCPacketNotReceived,        // 51
						rtcp.TypeTCCPacketReceivedSmallDelta, // 52
						rtcp.TypeTCCPacketNotReceived,        // 53
						rtcp.TypeTCCPacketReceivedSmallDelta, // 54
						rtcp.TypeTCCPacketNotReceived,        // 55
						rtcp.TypeTCCPacketReceivedSmallDelta, // 56
						rtcp.TypeTCCPacketReceivedSmallDelta, // 57
						rtcp.TypeTCCPacketReceivedSmallDelta, // 58
						rtcp.TypeTCCPacketNotReceived,        // 59
						rtcp.TypeTCCPacketReceivedSmallDelta, // 60
						rtcp.TypeTCCPacketReceivedSmallDelta, // 61
						rtcp.TypeTCCPacketReceivedSmallDelta, // 62
						rtcp.TypeTCCPacketReceivedSmallDelta, // 63
					},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						// [1 1 1 1 1 0 0]
						rtcp.TypeTCCPacketReceivedSmallDelta, // 64
						rtcp.TypeTCCPacketReceivedSmallDelta, // 65
						rtcp.TypeTCCPacketReceivedSmallDelta, // 66
						rtcp.TypeTCCPacketReceivedSmallDelta, // 67
						rtcp.TypeTCCPacketReceivedSmallDelta, // 68
						rtcp.TypeTCCPacketNotReceived,        // 69
						rtcp.TypeTCCPacketNotReceived,        // 70
					},
				},
				&rtcp.RunLengthChunk{
					PacketStatusChunk:  nil,
					Type:               rtcp.RunLengthChunkType,
					PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
					RunLength:          2,
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			},
		}, unmarshalled)
		marshalAll(t, rtcpPackets)
	})

	t.Run("5", func(t *testing.T) {
		r := NewRecorder(5000)

		sequenceNumbers := []uint16{75, 76, 77, 78, 79, 80, 81, 83, 83, 85, 85, 86, 87, 88, 91, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101, 102, 103, 104, 107, 108}
		arrivalTime := int64(scaleFactorReferenceTime)
		for i := range sequenceNumbers {
			r.Record(5000, sequenceNumbers[i], increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor))
		}

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		pkt := rtcpPackets[0].(*rtcp.TransportLayerCC)
		bs, err := pkt.Marshal()
		unmarshalled := &rtcp.TransportLayerCC{}
		assert.NoError(t, err)
		assert.NoError(t, unmarshalled.Unmarshal(bs))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  13,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 75,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  34,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeOneBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta, // 75
						rtcp.TypeTCCPacketReceivedSmallDelta, // 76
						rtcp.TypeTCCPacketReceivedSmallDelta, // 77
						rtcp.TypeTCCPacketReceivedSmallDelta, // 78
						rtcp.TypeTCCPacketReceivedSmallDelta, // 79
						rtcp.TypeTCCPacketReceivedSmallDelta, // 80
						rtcp.TypeTCCPacketReceivedSmallDelta, // 81
						rtcp.TypeTCCPacketNotReceived,        // 82
						rtcp.TypeTCCPacketReceivedSmallDelta, // 83
						rtcp.TypeTCCPacketNotReceived,        // 84
						rtcp.TypeTCCPacketReceivedSmallDelta, // 85
						rtcp.TypeTCCPacketReceivedSmallDelta, // 86
						rtcp.TypeTCCPacketReceivedSmallDelta, // 87
						rtcp.TypeTCCPacketReceivedSmallDelta, // 88
					},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeOneBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketNotReceived,        // 89
						rtcp.TypeTCCPacketNotReceived,        // 90
						rtcp.TypeTCCPacketReceivedSmallDelta, // 91
						rtcp.TypeTCCPacketReceivedSmallDelta, // 92
						rtcp.TypeTCCPacketReceivedSmallDelta, // 93
						rtcp.TypeTCCPacketReceivedSmallDelta, // 94
						rtcp.TypeTCCPacketReceivedSmallDelta, // 95
						rtcp.TypeTCCPacketReceivedSmallDelta, // 96
						rtcp.TypeTCCPacketReceivedSmallDelta, // 97
						rtcp.TypeTCCPacketReceivedSmallDelta, // 98
						rtcp.TypeTCCPacketReceivedSmallDelta, // 99
						rtcp.TypeTCCPacketReceivedSmallDelta, // 100
						rtcp.TypeTCCPacketReceivedSmallDelta, // 101
						rtcp.TypeTCCPacketReceivedSmallDelta, // 102
					},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.TypeTCCStatusVectorChunk,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList: []uint16{
						rtcp.TypeTCCPacketReceivedSmallDelta, // 103
						rtcp.TypeTCCPacketReceivedSmallDelta, // 104
						rtcp.TypeTCCPacketNotReceived,        // 105
						rtcp.TypeTCCPacketNotReceived,        // 106
						rtcp.TypeTCCPacketReceivedSmallDelta, // 107
						rtcp.TypeTCCPacketReceivedSmallDelta, // 108
						rtcp.TypeTCCPacketNotReceived,        // 109
					},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			},
		}, unmarshalled)
		marshalAll(t, rtcpPackets)
	})
}

func TestShortDeltas(t *testing.T) {
	t.Run("SplitsOneBitDeltas", func(t *testing.T) {
		r := NewRecorder(5000)

		arrivalTime := int64(scaleFactorReferenceTime)
		addRun(t, r, []uint16{3, 4, 5, 7, 6, 8, 10, 11, 13, 14}, []int64{
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
		})

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		pkt := rtcpPackets[0].(*rtcp.TransportLayerCC)
		bs, err := pkt.Marshal()
		unmarshalled := &rtcp.TransportLayerCC{}
		assert.NoError(t, err)
		assert.NoError(t, unmarshalled.Unmarshal(bs))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  8,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 3,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  12,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.BitVectorChunkType,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList:        []uint16{1, 1, 1, 1, 1, 1, 0},
				},
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.BitVectorChunkType,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList:        []uint16{1, 1, 0, 1, 1, 0, 0},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			},
		}, unmarshalled)
		marshalAll(t, rtcpPackets)
	})

	t.Run("padsTwoBitDeltas", func(t *testing.T) {
		r := NewRecorder(5000)

		arrivalTime := int64(scaleFactorReferenceTime)
		addRun(t, r, []uint16{3, 4, 5, 7}, []int64{
			arrivalTime,
			arrivalTime,
			arrivalTime,
			arrivalTime,
		})

		rtcpPackets := r.BuildFeedbackPacket()
		assert.Equal(t, 1, len(rtcpPackets))

		pkt := rtcpPackets[0].(*rtcp.TransportLayerCC)
		bs, err := pkt.Marshal()
		unmarshalled := &rtcp.TransportLayerCC{}
		assert.NoError(t, err)
		assert.NoError(t, unmarshalled.Unmarshal(bs))

		assert.Equal(t, &rtcp.TransportLayerCC{
			Header: rtcp.Header{
				Count:   rtcp.FormatTCC,
				Type:    rtcp.TypeTransportSpecificFeedback,
				Padding: true,
				Length:  6,
			},
			SenderSSRC:         5000,
			MediaSSRC:          5000,
			BaseSequenceNumber: 3,
			ReferenceTime:      1,
			FbPktCount:         0,
			PacketStatusCount:  5,
			PacketChunks: []rtcp.PacketStatusChunk{
				&rtcp.StatusVectorChunk{
					PacketStatusChunk: nil,
					Type:              rtcp.BitVectorChunkType,
					SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
					SymbolList:        []uint16{1, 1, 1, 0, 1, 0, 0},
				},
			},
			RecvDeltas: []*rtcp.RecvDelta{
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
				{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			},
		}, unmarshalled)
		marshalAll(t, rtcpPackets)
	})
}

func TestReorderedPackets(t *testing.T) {
	r := NewRecorder(5000)

	arrivalTime := int64(scaleFactorReferenceTime)
	addRun(t, r, []uint16{3, 4, 5, 7, 6, 8, 10, 11, 13, 14}, []int64{
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
		increaseTime(&arrivalTime, rtcp.TypeTCCDeltaScaleFactor),
	})

	rtcpPackets := r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))

	pkt := rtcpPackets[0].(*rtcp.TransportLayerCC)
	bs, err := pkt.Marshal()
	unmarshalled := &rtcp.TransportLayerCC{}
	assert.NoError(t, err)
	assert.NoError(t, unmarshalled.Unmarshal(bs))

	assert.Equal(t, &rtcp.TransportLayerCC{
		Header: rtcp.Header{
			Count:   rtcp.FormatTCC,
			Type:    rtcp.TypeTransportSpecificFeedback,
			Padding: true,
			Length:  8,
		},
		SenderSSRC:         5000,
		MediaSSRC:          5000,
		BaseSequenceNumber: 3,
		ReferenceTime:      1,
		FbPktCount:         0,
		PacketStatusCount:  12,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.StatusVectorChunk{
				PacketStatusChunk: nil,
				Type:              rtcp.BitVectorChunkType,
				SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList:        []uint16{1, 1, 1, 1, 2, 1, 0},
			},
			&rtcp.StatusVectorChunk{
				PacketStatusChunk: nil,
				Type:              rtcp.BitVectorChunkType,
				SymbolSize:        rtcp.TypeTCCSymbolSizeTwoBit,
				SymbolList:        []uint16{1, 1, 0, 1, 1, 0, 0},
			},
		},
		RecvDeltas: []*rtcp.RecvDelta{
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedLargeDelta, Delta: -rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 2 * rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: rtcp.TypeTCCDeltaScaleFactor},
		},
	}, unmarshalled)
	marshalAll(t, rtcpPackets)
}

func TestInsertSorted(t *testing.T) {
	cases := []struct {
		l        []pktInfo
		e        pktInfo
		expected []pktInfo
	}{
		{
			l: []pktInfo{},
			e: pktInfo{},
			expected: []pktInfo{{
				sequenceNumber: 0,
				arrivalTime:    0,
			}},
		},
		{
			l: []pktInfo{
				{
					sequenceNumber: 0,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 1,
					arrivalTime:    0,
				},
			},
			e: pktInfo{
				sequenceNumber: 2,
				arrivalTime:    0,
			},
			expected: []pktInfo{
				{
					sequenceNumber: 0,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 1,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 2,
					arrivalTime:    0,
				},
			},
		},
		{
			l: []pktInfo{
				{
					sequenceNumber: 0,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 2,
					arrivalTime:    0,
				},
			},
			e: pktInfo{
				sequenceNumber: 1,
				arrivalTime:    0,
			},
			expected: []pktInfo{
				{
					sequenceNumber: 0,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 1,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 2,
					arrivalTime:    0,
				},
			},
		},
		{
			l: []pktInfo{
				{
					sequenceNumber: 0,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 1,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 2,
					arrivalTime:    0,
				},
			},
			e: pktInfo{
				sequenceNumber: 1,
				arrivalTime:    0,
			},
			expected: []pktInfo{
				{
					sequenceNumber: 0,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 1,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 2,
					arrivalTime:    0,
				},
			},
		},
		{
			l: []pktInfo{
				{
					sequenceNumber: 10,
					arrivalTime:    0,
				},
			},
			e: pktInfo{
				sequenceNumber: 9,
				arrivalTime:    0,
			},
			expected: []pktInfo{
				{
					sequenceNumber: 9,
					arrivalTime:    0,
				},
				{
					sequenceNumber: 10,
					arrivalTime:    0,
				},
			},
		},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, c.expected, insertSorted(c.l, c.e))
		})
	}
}

func mustParse(t string) time.Time {
	d, err := time.Parse("15:04:05.99999999", t)
	if err != nil {
		panic(err)
	}
	return d
}

func TestTHING(t *testing.T) {
	r := NewRecorder(5000)

	type packet struct {
		nr uint16
		ts time.Time
	}
	pkts := []packet{
		{nr: 1, ts: mustParse("13:21:32.66605293")},
		{nr: 3, ts: mustParse("13:21:32.68289038")},
		{nr: 2, ts: mustParse("13:21:32.6829856")},
		{nr: 4, ts: mustParse("13:21:32.68331201")},
		{nr: 5, ts: mustParse("13:21:32.69534352")},
		{nr: 6, ts: mustParse("13:21:32.69545351")},
		{nr: 7, ts: mustParse("13:21:32.70660835")},
		{nr: 9, ts: mustParse("13:21:32.71778934")},
		{nr: 10, ts: mustParse("13:21:32.73091492")},
	}

	fmt.Println(pkts[0].ts)
	start := pkts[0].ts.Add(-128 * time.Millisecond)
	fmt.Println(start)

	for _, pkt := range pkts {
		r.Record(5000, pkt.nr, pkt.ts.Sub(start).Microseconds())
	}

	rtcpPackets := r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))
	fmt.Println(rtcpPackets[0].(*rtcp.TransportLayerCC))

	pkts = []packet{
		{nr: 13, ts: mustParse("13:21:32.7400967")},
		{nr: 14, ts: mustParse("13:21:32.74720876")},
		{nr: 15, ts: mustParse("13:21:32.75139925")},
		{nr: 18, ts: mustParse("13:21:32.75755856")},
		{nr: 19, ts: mustParse("13:21:32.76365941")},
		{nr: 20, ts: mustParse("13:21:32.76374313")},
		{nr: 21, ts: mustParse("13:21:32.76584219")},
		{nr: 22, ts: mustParse("13:21:32.76797761")},
		{nr: 23, ts: mustParse("13:21:32.77013109")},
		{nr: 24, ts: mustParse("13:21:32.77332659")},
		{nr: 25, ts: mustParse("13:21:32.7754075")},
		{nr: 26, ts: mustParse("13:21:32.77761419")},
		{nr: 27, ts: mustParse("13:21:32.78074493")},
		{nr: 28, ts: mustParse("13:21:32.78080871")},
		{nr: 29, ts: mustParse("13:21:32.78295785")},
		{nr: 30, ts: mustParse("13:21:32.78304688")},
		{nr: 32, ts: mustParse("13:21:32.78554982")},
		{nr: 33, ts: mustParse("13:21:32.78741282")},
		{nr: 34, ts: mustParse("13:21:32.78750512")},
		{nr: 35, ts: mustParse("13:21:32.79050869")},
		{nr: 36, ts: mustParse("13:21:32.79059667")},
		{nr: 37, ts: mustParse("13:21:32.79265193")},
		{nr: 38, ts: mustParse("13:21:32.79273576")},
		{nr: 39, ts: mustParse("13:21:32.79472212")},
		{nr: 40, ts: mustParse("13:21:32.79482135")},
	}

	for _, pkt := range pkts {
		r.Record(5000, pkt.nr, pkt.ts.Sub(start).Microseconds())
	}

	rtcpPackets = r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))
	fmt.Println(rtcpPackets[0].(*rtcp.TransportLayerCC))

	pkts = []packet{
		{nr: 46, ts: mustParse("13:21:32.87171882")},
		{nr: 46, ts: mustParse("13:21:32.87181345")},
		{nr: 47, ts: mustParse("13:21:32.87379482")},
		{nr: 48, ts: mustParse("13:21:32.87385716")},
		{nr: 49, ts: mustParse("13:21:32.87696351")},
		{nr: 50, ts: mustParse("13:21:32.87703434")},
		{nr: 51, ts: mustParse("13:21:32.88018133")},
		{nr: 52, ts: mustParse("13:21:32.88028212")},
		{nr: 54, ts: mustParse("13:21:32.8803838")},
		{nr: 54, ts: mustParse("13:21:32.88042859")},
		{nr: 56, ts: mustParse("13:21:32.88232208")},
		{nr: 56, ts: mustParse("13:21:32.88244116")},
		{nr: 58, ts: mustParse("13:21:32.88259426")},
		{nr: 58, ts: mustParse("13:21:32.88265957")},
		{nr: 60, ts: mustParse("13:21:32.88559268")},
		{nr: 60, ts: mustParse("13:21:32.88569149")},
		{nr: 61, ts: mustParse("13:21:32.88576656")},
		{nr: 62, ts: mustParse("13:21:32.88585761")},
		{nr: 65, ts: mustParse("13:21:32.91189473")},
	}

	for _, pkt := range pkts {
		r.Record(5000, pkt.nr, pkt.ts.Sub(start).Microseconds())
	}

	rtcpPackets = r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))
	fmt.Println(rtcpPackets[0].(*rtcp.TransportLayerCC))

	pkts = []packet{
		{nr: 68, ts: mustParse("13:21:32.94151858")},
		{nr: 69, ts: mustParse("13:21:32.94162367")},
		{nr: 70, ts: mustParse("13:21:32.94367602")},
		{nr: 71, ts: mustParse("13:21:32.94376743")},
		{nr: 72, ts: mustParse("13:21:32.94583518")},
		{nr: 73, ts: mustParse("13:21:32.94593532")},
		{nr: 75, ts: mustParse("13:21:32.94603652")},
		{nr: 75, ts: mustParse("13:21:32.94610846")},
		{nr: 76, ts: mustParse("13:21:32.94620313")},
		{nr: 77, ts: mustParse("13:21:32.94803155")},
		{nr: 78, ts: mustParse("13:21:32.94812239")},
		{nr: 80, ts: mustParse("13:21:32.94826421")},
		{nr: 80, ts: mustParse("13:21:32.94833984")},
		{nr: 81, ts: mustParse("13:21:32.94848815")},
		{nr: 82, ts: mustParse("13:21:32.95025247")},
		{nr: 83, ts: mustParse("13:21:32.95035025")},
		{nr: 85, ts: mustParse("13:21:32.95048062")},
		{nr: 85, ts: mustParse("13:21:32.95052717")},
		{nr: 86, ts: mustParse("13:21:32.95059531")},
		{nr: 87, ts: mustParse("13:21:32.95266087")},
		{nr: 88, ts: mustParse("13:21:32.95275829")},
		{nr: 89, ts: mustParse("13:21:32.95285479")},
		{nr: 90, ts: mustParse("13:21:32.95295125")},
		{nr: 91, ts: mustParse("13:21:32.95306884")},
		{nr: 92, ts: mustParse("13:21:32.9547105")},
		{nr: 93, ts: mustParse("13:21:32.95483123")},
		{nr: 94, ts: mustParse("13:21:32.95492333")},
		{nr: 96, ts: mustParse("13:21:32.95504746")},
		{nr: 96, ts: mustParse("13:21:32.95513238")},
		{nr: 97, ts: mustParse("13:21:32.95999092")},
		{nr: 100, ts: mustParse("13:21:33.00914531")},
		{nr: 101, ts: mustParse("13:21:33.01114703")},
	}

	for _, pkt := range pkts {
		go r.Record(5000, pkt.nr, pkt.ts.Sub(start).Microseconds())
	}

	rtcpPackets = r.BuildFeedbackPacket()
	assert.Equal(t, 1, len(rtcpPackets))
	fmt.Println(rtcpPackets[0].(*rtcp.TransportLayerCC))
}
