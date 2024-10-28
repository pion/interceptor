// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpbuffer

import (
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRTPBuffer(t *testing.T) {
	pm := NewPacketFactoryCopy()
	for _, start := range []uint16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 511, 512, 513, 32767, 32768, 32769, 65527, 65528, 65529, 65530, 65531, 65532, 65533, 65534, 65535} {
		start := start

		sb, err := NewRTPBuffer(8)
		require.NoError(t, err)

		add := func(nums ...uint16) {
			for _, n := range nums {
				seq := start + n
				pkt, err := pm.NewPacket(&rtp.Header{SequenceNumber: seq}, nil, 0, 0)
				require.NoError(t, err)
				sb.Add(pkt)
			}
		}

		assertGet := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				packet := sb.Get(seq)
				if packet == nil {
					t.Errorf("packet not found: %d", seq)
					continue
				}
				if packet.Header().SequenceNumber != seq {
					t.Errorf("packet for %d returned with incorrect SequenceNumber: %d", seq, packet.Header().SequenceNumber)
				}
				packet.Release()
			}
		}
		assertNOTGet := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				packet := sb.Get(seq)
				if packet != nil {
					t.Errorf("packet found for %d: %d", seq, packet.Header().SequenceNumber)
				}
			}
		}

		add(0, 1, 2, 3, 4, 5, 6, 7)
		assertGet(0, 1, 2, 3, 4, 5, 6, 7)

		add(8)
		assertGet(8)
		assertNOTGet(0)

		add(10)
		assertGet(10)
		assertNOTGet(1, 2, 9)

		add(22)
		assertGet(22)
		assertNOTGet(3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21)
	}
}

func TestRTPBuffer_WithRTX(t *testing.T) {
	pm := NewPacketFactoryCopy()
	for _, start := range []uint16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 511, 512, 513, 32767, 32768, 32769, 65527, 65528, 65529, 65530, 65531, 65532, 65533, 65534, 65535} {
		start := start

		sb, err := NewRTPBuffer(8)
		require.NoError(t, err)

		add := func(nums ...uint16) {
			for _, n := range nums {
				seq := start + n
				pkt, err := pm.NewPacket(&rtp.Header{SequenceNumber: seq, PayloadType: 2}, []byte("originalcontent"), 1, 1)
				require.NoError(t, err)
				sb.Add(pkt)
			}
		}

		assertGet := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				packet := sb.Get(seq)
				if packet == nil {
					t.Errorf("packet not found: %d", seq)
					continue
				}
				if packet.Header().SSRC != 1 && packet.Header().PayloadType != 1 {
					t.Errorf("packet for %d returned with incorrect SSRC : %d and PayloadType: %d", seq, packet.Header().SSRC, packet.Header().PayloadType)
				}
				packet.Release()
			}
		}
		assertNOTGet := func(nums ...uint16) {
			t.Helper()
			for _, n := range nums {
				seq := start + n
				packet := sb.Get(seq)
				if packet != nil {
					t.Errorf("packet found for %d: %d", seq, packet.Header().SequenceNumber)
				}
			}
		}

		add(0, 1, 2, 3, 4, 5, 6, 7)
		assertGet(0, 1, 2, 3, 4, 5, 6, 7)

		add(8)
		assertGet(8)
		assertNOTGet(0)

		add(10)
		assertGet(10)
		assertNOTGet(1, 2, 9)

		add(22)
		assertGet(22)
		assertNOTGet(3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21)
	}
}

func TestRTPBuffer_Overridden(t *testing.T) {
	// override original packet content and get
	pm := NewPacketFactoryCopy()
	sb, err := NewRTPBuffer(1)
	require.NoError(t, err)
	require.Equal(t, uint16(1), sb.size)

	originalBytes := []byte("originalContent")
	pkt, err := pm.NewPacket(&rtp.Header{SequenceNumber: 1}, originalBytes, 0, 0)
	require.NoError(t, err)
	sb.Add(pkt)

	// change payload
	copy(originalBytes, "altered")
	retrieved := sb.Get(1)
	require.NotNil(t, retrieved)
	require.Equal(t, "originalContent", string(retrieved.Payload()))
	retrieved.Release()
	require.Equal(t, 1, retrieved.count)

	// ensure original packet is released
	pkt, err = pm.NewPacket(&rtp.Header{SequenceNumber: 2}, originalBytes, 0, 0)
	require.NoError(t, err)
	sb.Add(pkt)
	require.Equal(t, 0, retrieved.count)

	require.Nil(t, sb.Get(1))
}

func TestRTPBuffer_Overridden_WithRTX_AND_Padding(t *testing.T) {
	// override original packet content and get
	pm := NewPacketFactoryCopy()
	sb, err := NewRTPBuffer(1)
	require.NoError(t, err)
	require.Equal(t, uint16(1), sb.size)

	originalBytes := []byte("originalContent\x01")
	pkt, err := pm.NewPacket(&rtp.Header{SequenceNumber: 1, Padding: true, SSRC: 2, PayloadType: 3}, originalBytes, 1, 1)
	require.NoError(t, err)
	sb.Add(pkt)

	// change payload
	copy(originalBytes, "altered")
	retrieved := sb.Get(1)
	require.NotNil(t, retrieved)
	require.Equal(t, "\x00\x01originalContent", string(retrieved.Payload()))
	retrieved.Release()
	require.Equal(t, 1, retrieved.count)

	// ensure original packet is released
	pkt, err = pm.NewPacket(&rtp.Header{SequenceNumber: 2}, originalBytes, 1, 1)
	require.NoError(t, err)
	sb.Add(pkt)
	require.Equal(t, 0, retrieved.count)

	require.Nil(t, sb.Get(1))
}

func TestRTPBuffer_Overridden_WithRTX_NILPayload(t *testing.T) {
	// override original packet content and get
	pm := NewPacketFactoryCopy()
	sb, err := NewRTPBuffer(1)
	require.NoError(t, err)
	require.Equal(t, uint16(1), sb.size)

	pkt, err := pm.NewPacket(&rtp.Header{SequenceNumber: 1}, nil, 1, 1)
	require.NoError(t, err)
	sb.Add(pkt)

	// change payload

	retrieved := sb.Get(1)
	require.NotNil(t, retrieved)
	require.Equal(t, "\x00\x01", string(retrieved.Payload()))
	retrieved.Release()
	require.Equal(t, 1, retrieved.count)

	// ensure original packet is released
	pkt, err = pm.NewPacket(&rtp.Header{SequenceNumber: 2}, []byte("altered"), 1, 1)
	require.NoError(t, err)
	sb.Add(pkt)
	require.Equal(t, 0, retrieved.count)

	require.Nil(t, sb.Get(1))
}
