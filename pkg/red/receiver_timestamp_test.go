// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"io"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func timestampREDPacket(t *testing.T, sequence uint16, timestamp uint32, blocks ...Block) rtp.Packet {
	t.Helper()

	return makeREDPacket(t, rtp.Header{
		Version:        2,
		SequenceNumber: sequence,
		Timestamp:      timestamp,
		SSRC:           testSSRC,
	}, blocks, []byte{byte(sequence & 0xff)}) //nolint:gosec // Test payload uses only the low byte of the sequence number.
}

func timestampBlock(offset uint16, payload byte) Block {
	return Block{PayloadType: testOpusPayloadType, TimestampOffset: offset, Payload: []byte{payload}}
}

func assertTimestampOutputs(t *testing.T, reader interceptor.RTPReader, expected ...rtp.Packet) {
	t.Helper()

	for _, want := range expected {
		got, attributes := readOutputPacket(t, reader)
		assert.Equal(t, want.SequenceNumber, got.SequenceNumber)
		assert.Equal(t, want.Timestamp, got.Timestamp)
		assert.Equal(t, want.Payload, got.Payload)
		assert.Equal(t, testOpusPayloadType, got.PayloadType)
		header, err := attributes.GetRTPHeader(marshalPacket(t, got))
		assert.NoError(t, err)
		if err == nil {
			assert.Equal(t, got.SequenceNumber, header.SequenceNumber)
			assert.Equal(t, got.Timestamp, header.Timestamp)
		}
	}

	n, _, err := reader.Read(make([]byte, 1500), nil)
	assert.Zero(t, n)
	assert.ErrorIs(t, err, io.EOF)
}

func TestReceiverTimestampRecoveryOrdersCompleteVariableDurationGap(t *testing.T) {
	// The missing packets begin 10 ms apart, then the last covers 20 ms.
	// Their header order is newest first and their offsets are not a fixed cadence.
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 4, 2_880,
		timestampBlock(960, 3), timestampBlock(1_440, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier))

	assertTimestampOutputs(t, reader, anchor,
		makePlainPacket(2, 1_440, []byte{2}),
		makePlainPacket(3, 1_920, []byte{3}),
		makePlainPacket(4, 2_880, []byte{4}))
}

func TestReceiverTimestampRecoverySkipsPartialGapWithoutSuppressingOriginals(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 4, 3_840, timestampBlock(960, 3))
	lateSecond := makePlainPacket(2, 1_920, []byte{2})
	lateThird := makePlainPacket(3, 2_880, []byte{3})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(),
		readerForPackets(t, anchor, carrier, lateSecond, lateThird))

	assertTimestampOutputs(t, reader, anchor, makePlainPacket(4, 3_840, []byte{4}), lateSecond, lateThird)
}

func TestReceiverTimestampRecoverySkipsUnanchoredBlocks(t *testing.T) {
	carrier := timestampREDPacket(t, 3, 2_880, timestampBlock(1_920, 1), timestampBlock(960, 2))
	lateFirst := makePlainPacket(1, 960, []byte{1})
	lateSecond := makePlainPacket(2, 1_920, []byte{2})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, carrier, lateFirst, lateSecond))

	assertTimestampOutputs(t, reader, makePlainPacket(3, 2_880, []byte{3}), lateFirst, lateSecond)
}

func TestReceiverTimestampRecoveryHandlesCompleteGapAcrossSilence(t *testing.T) {
	// Silence changes timestamp spacing without consuming sequence numbers.
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 3, 9_600, timestampBlock(5_760, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier))

	assertTimestampOutputs(t, reader, anchor,
		makePlainPacket(2, 3_840, []byte{2}), makePlainPacket(3, 9_600, []byte{3}))
}

func TestReceiverTimestampRecoveryDeduplicatesBlockTimestamps(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 3, 2_880, timestampBlock(960, 2), timestampBlock(960, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier))

	assertTimestampOutputs(t, reader, anchor,
		makePlainPacket(2, 1_920, []byte{2}), makePlainPacket(3, 2_880, []byte{3}))
}

func TestReceiverTimestampRecoverySkipsConflictingBlocksWithoutSuppressingOriginal(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 3, 2_880, timestampBlock(960, 2), timestampBlock(960, 0xf2))
	lateOriginal := makePlainPacket(2, 1_920, []byte{2})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier, lateOriginal))

	assertTimestampOutputs(t, reader, anchor, makePlainPacket(3, 2_880, []byte{3}), lateOriginal)
}

func TestReceiverTimestampRecoveryDoesNotGuessSequenceFromElapsedTime(t *testing.T) {
	for _, originalSequence := range []uint16{97, 98} {
		t.Run(string(rune('A'+originalSequence-97)), func(t *testing.T) {
			// These identical received packets admit two continuous Opus histories:
			// 20,20,10,10 ms or 10,10,20,20 ms for the four missing packets.
			// The same backup timestamp belongs to SN 97 or 98 respectively.
			anchor := makePlainPacket(95, 92_160, []byte{95})
			carrier := timestampREDPacket(t, 100, 96_000, timestampBlock(1_920, 0xa0))
			lateOriginal := makePlainPacket(originalSequence, 94_080, []byte{0xa0})
			reader := newTestReceiverWithOptions(t, opusStreamInfo(),
				readerForPackets(t, anchor, carrier, lateOriginal))

			assertTimestampOutputs(t, reader, anchor, makePlainPacket(100, 96_000, []byte{100}), lateOriginal)
		})
	}
}

func TestReceiverTimestampRecoveryDoesNotCountUnsupportedBlocks(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	unsupported := timestampBlock(960, 3)
	unsupported.PayloadType--
	carrier := timestampREDPacket(t, 4, 3_840, timestampBlock(1_920, 2), unsupported)
	lateSecond := makePlainPacket(2, 1_920, []byte{2})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier, lateSecond))

	assertTimestampOutputs(t, reader, anchor, makePlainPacket(4, 3_840, []byte{4}), lateSecond)
}

func TestReceiverTimestampRecoveryExcludesObservedPaddingFromGap(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 4, 2_880, timestampBlock(960, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(),
		readerForPackets(t, anchor, paddingOnlyPacket(3, 1_920, 1), carrier))

	assertTimestampOutputs(t, reader, anchor,
		makePlainPacket(2, 1_920, []byte{2}), makePlainPacket(4, 2_880, []byte{4}))
}

func TestReceiverTimestampRecoveryWrapsSequenceAndTimestamp(t *testing.T) {
	anchor := makePlainPacket(65_534, 4_294_965_856, []byte{0xfe})
	carrier := timestampREDPacket(t, 1, 1_440, timestampBlock(960, 0), timestampBlock(1_920, 0xff))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier))

	assertTimestampOutputs(t, reader, anchor,
		makePlainPacket(65_535, 4_294_966_816, []byte{0xff}),
		makePlainPacket(0, 480, []byte{0}), makePlainPacket(1, 1_440, []byte{1}))
}

func TestReceiverTimestampRecoveryDropsAnchorsOnHistoryReset(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 100, 96_000, timestampBlock(1_920, 98), timestampBlock(960, 99))
	lateOriginal := makePlainPacket(99, 95_040, []byte{99})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier, lateOriginal))

	assertTimestampOutputs(t, reader, anchor, makePlainPacket(100, 96_000, []byte{100}), lateOriginal)
}

func TestReceiverTimestampRecoveryUsesReorderedAnchorWithSeenPrimary(t *testing.T) {
	carrier := timestampREDPacket(t, 4, 3_840, timestampBlock(960, 3), timestampBlock(1_920, 2))
	lateAnchor := makePlainPacket(1, 960, []byte{1})
	lateDuplicate := makePlainPacket(2, 1_920, []byte{0xf2})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(),
		readerForPackets(t, carrier, lateAnchor, carrier, lateDuplicate))

	assertTimestampOutputs(t, reader, makePlainPacket(4, 3_840, []byte{4}), lateAnchor,
		makePlainPacket(2, 1_920, []byte{2}), makePlainPacket(3, 2_880, []byte{3}))
}

func TestReceiverTimestampRecoveryUsesIntermediateAnchors(t *testing.T) {
	first := makePlainPacket(1, 960, []byte{1})
	third := makePlainPacket(3, 2_880, []byte{3})
	carrier := timestampREDPacket(t, 5, 4_800, timestampBlock(960, 4), timestampBlock(2_880, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, first, third, carrier))

	assertTimestampOutputs(t, reader, first, third,
		makePlainPacket(2, 1_920, []byte{2}), makePlainPacket(4, 3_840, []byte{4}),
		makePlainPacket(5, 4_800, []byte{5}))
}

func TestReceiverFixedDurationRecoveryOrdersSparseBlocksByOffset(t *testing.T) {
	carrier := timestampREDPacket(t, 100, 96_000, timestampBlock(960, 99), timestampBlock(2_880, 97))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, carrier),
		ReceiverPacketDuration(20*time.Millisecond))

	assertTimestampOutputs(t, reader, makePlainPacket(97, 93_120, []byte{97}),
		makePlainPacket(99, 95_040, []byte{99}), makePlainPacket(100, 96_000, []byte{100}))
}

func TestReceiverFixedDurationRecoverySkipsOffsetsBetweenPackets(t *testing.T) {
	carrier := timestampREDPacket(t, 100, 96_000, timestampBlock(1_000, 0xf9), timestampBlock(1_920, 98))
	lateOriginal := makePlainPacket(99, 95_040, []byte{99})
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, carrier, lateOriginal),
		ReceiverPacketDuration(20*time.Millisecond))

	assertTimestampOutputs(t, reader, makePlainPacket(98, 94_080, []byte{98}),
		makePlainPacket(100, 96_000, []byte{100}), lateOriginal)
}

func TestReceiverFixedDurationRecoveryFallsBackForContradictoryTiming(t *testing.T) {
	t.Run("CompleteGap", func(t *testing.T) {
		anchor := makePlainPacket(1, 960, []byte{1})
		carrier := timestampREDPacket(t, 4, 2_880, timestampBlock(960, 3), timestampBlock(1_440, 2))
		reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier),
			ReceiverPacketDuration(20*time.Millisecond))

		assertTimestampOutputs(t, reader, anchor, makePlainPacket(2, 1_440, []byte{2}),
			makePlainPacket(3, 1_920, []byte{3}), makePlainPacket(4, 2_880, []byte{4}))
	})

	t.Run("PartialGap", func(t *testing.T) {
		anchor := makePlainPacket(95, 92_160, []byte{95})
		carrier := timestampREDPacket(t, 100, 96_000, timestampBlock(1_920, 97))
		lateOriginal := makePlainPacket(97, 94_080, []byte{97})
		reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, carrier, lateOriginal),
			ReceiverPacketDuration(20*time.Millisecond))

		assertTimestampOutputs(t, reader, anchor, makePlainPacket(100, 96_000, []byte{100}), lateOriginal)
	})
}

func TestReceiverFixedDurationRecoveryFallsBackAfterObservedPadding(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	carrier := timestampREDPacket(t, 4, 2_880, timestampBlock(960, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(),
		readerForPackets(t, anchor, paddingOnlyPacket(3, 1_920, 1), carrier),
		ReceiverPacketDuration(20*time.Millisecond))

	assertTimestampOutputs(t, reader, anchor, makePlainPacket(2, 1_920, []byte{2}),
		makePlainPacket(4, 2_880, []byte{4}))
}

func TestReceiverFixedDurationRecoveryFallsBackAfterOtherPayloadType(t *testing.T) {
	anchor := makePlainPacket(1, 960, []byte{1})
	other := makePlainPacket(3, 1_920, []byte{0xa3})
	other.PayloadType--
	carrier := timestampREDPacket(t, 4, 2_880, timestampBlock(960, 2))
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, anchor, other, carrier),
		ReceiverPacketDuration(20*time.Millisecond))

	first, _ := readOutputPacket(t, reader)
	assert.Equal(t, anchor.SequenceNumber, first.SequenceNumber)
	second, _ := readOutputPacket(t, reader)
	assert.Equal(t, other.SequenceNumber, second.SequenceNumber)
	assert.Equal(t, other.PayloadType, second.PayloadType)
	assert.Equal(t, other.Payload, second.Payload)
	assertTimestampOutputs(t, reader, makePlainPacket(2, 1_920, []byte{2}),
		makePlainPacket(4, 2_880, []byte{4}))
}
