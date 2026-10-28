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
		assert.Equal(t, want.PayloadType, got.PayloadType)
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

type timestampRecoveryTest struct {
	name     string
	input    []rtp.Packet
	expected []rtp.Packet
}

func runTimestampRecoveryTests(t *testing.T, tests []timestampRecoveryTest, options ...ReceiverOption) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := newTestReceiverWithOptions(t, opusStreamInfo(), readerForPackets(t, test.input...), options...)
			assertTimestampOutputs(t, reader, test.expected...)
		})
	}
}

func TestReceiverTimestampMapping(t *testing.T) {
	first := makePlainPacket(1, 960, []byte{1})
	third := makePlainPacket(3, 2_880, []byte{3})
	lateCarrier := timestampREDPacket(t, 4, 3_840, timestampBlock(960, 3), timestampBlock(1_920, 2))
	paddingAnchor := makePlainPacket(9, 8_640, []byte{9})
	wrapAnchor := makePlainPacket(65_534, 4_294_965_856, []byte{0xfe})

	runTimestampRecoveryTests(t, []timestampRecoveryTest{
		{
			// Two missing packets begin 10 ms apart, then the last covers 20 ms.
			name:  "ReversedBlocksWithVariableDuration",
			input: []rtp.Packet{first, timestampREDPacket(t, 4, 2_880, timestampBlock(960, 3), timestampBlock(1_440, 2))},
			expected: []rtp.Packet{
				first, makePlainPacket(2, 1_440, []byte{2}),
				makePlainPacket(3, 1_920, []byte{3}), makePlainPacket(4, 2_880, []byte{4}),
			},
		},
		{
			name: "UnanchoredBlocksLeaveOriginalsDeliverable",
			input: []rtp.Packet{
				timestampREDPacket(t, 3, 2_880, timestampBlock(1_920, 1), timestampBlock(960, 2)),
				first, makePlainPacket(2, 1_920, []byte{2}),
			},
			expected: []rtp.Packet{third, first, makePlainPacket(2, 1_920, []byte{2})},
		},
		{
			name:     "SilenceAdvancesTimeWithoutConsumingSequenceNumbers",
			input:    []rtp.Packet{first, timestampREDPacket(t, 3, 9_600, timestampBlock(5_760, 2))},
			expected: []rtp.Packet{first, makePlainPacket(2, 3_840, []byte{2}), makePlainPacket(3, 9_600, []byte{3})},
		},
		{
			name: "ObservedPaddingAndAlreadySeenBackup",
			input: []rtp.Packet{
				paddingAnchor, paddingOnlyPacket(11, 9_600, 50),
				timestampREDPacket(t, 12, 11_520, timestampBlock(2_880, 9), timestampBlock(1_920, 10)),
			},
			expected: []rtp.Packet{paddingAnchor, makePlainPacket(10, 9_600, []byte{10}), makePlainPacket(12, 11_520, []byte{12})},
		},
		{
			name: "ObservedPaddingHasNoMissingMedia",
			input: []rtp.Packet{
				makePlainPacket(10, 9_600, []byte{10}), paddingOnlyPacket(11, 9_600, 50),
				timestampREDPacket(t, 12, 10_560, timestampBlock(960, 10)),
			},
			expected: []rtp.Packet{makePlainPacket(10, 9_600, []byte{10}), makePlainPacket(12, 10_560, []byte{12})},
		},
		{
			name:  "SequenceAndTimestampWraparound",
			input: []rtp.Packet{wrapAnchor, timestampREDPacket(t, 1, 1_440, timestampBlock(960, 0), timestampBlock(1_920, 0xff))},
			expected: []rtp.Packet{
				wrapAnchor, makePlainPacket(65_535, 4_294_966_816, []byte{0xff}),
				makePlainPacket(0, 480, []byte{0}), makePlainPacket(1, 1_440, []byte{1}),
			},
		},
		{
			name: "HistoryResetRejectsStalePacketsAndAcceptsUnseenRecentPackets",
			input: []rtp.Packet{
				first, timestampREDPacket(t, 100, 96_000, timestampBlock(1_920, 98), timestampBlock(960, 99)),
				makePlainPacket(1, 960, []byte{0xf1}), makePlainPacket(99, 95_040, []byte{99}), makePlainPacket(101, 96_960, []byte{101}),
			},
			expected: []rtp.Packet{
				first, makePlainPacket(100, 96_000, []byte{100}),
				makePlainPacket(99, 95_040, []byte{99}), makePlainPacket(101, 96_960, []byte{101}),
			},
		},
		{
			name:     "ReorderedAnchorWithSeenPrimary",
			input:    []rtp.Packet{lateCarrier, first, lateCarrier, makePlainPacket(2, 1_920, []byte{0xf2})},
			expected: []rtp.Packet{makePlainPacket(4, 3_840, []byte{4}), first, makePlainPacket(2, 1_920, []byte{2}), third},
		},
		{
			name:  "IntermediateAnchorsPartitionGaps",
			input: []rtp.Packet{first, third, timestampREDPacket(t, 5, 4_800, timestampBlock(960, 4), timestampBlock(2_880, 2))},
			expected: []rtp.Packet{
				first, third, makePlainPacket(2, 1_920, []byte{2}),
				makePlainPacket(4, 3_840, []byte{4}), makePlainPacket(5, 4_800, []byte{5}),
			},
		},
	})
}

func TestReceiverAmbiguousBlocks(t *testing.T) {
	first := makePlainPacket(1, 960, []byte{1})
	second := makePlainPacket(2, 1_920, []byte{2})
	third := makePlainPacket(3, 2_880, []byte{3})
	anchor := makePlainPacket(95, 92_160, []byte{95})
	carrier := timestampREDPacket(t, 100, 96_000, timestampBlock(1_920, 0xa0))
	primary := makePlainPacket(100, 96_000, []byte{100})
	unsupported := timestampBlock(960, 3)
	unsupported.PayloadType--

	// Identical received packets admit these two continuous Opus histories:
	// 20,20,10,10 ms or 10,10,20,20 ms for the four missing packets. A backup
	// timestamp alone cannot choose SN 97 or 98; every late original must survive.
	runTimestampRecoveryTests(t, []timestampRecoveryTest{
		{
			name: "PartialGapWithBackupAtSequence97",
			input: []rtp.Packet{
				anchor, carrier, makePlainPacket(96, 93_120, []byte{96}),
				makePlainPacket(97, 94_080, []byte{0xa0}), makePlainPacket(98, 95_040, []byte{98}), makePlainPacket(99, 95_520, []byte{99}),
			},
			expected: []rtp.Packet{
				anchor, primary, makePlainPacket(96, 93_120, []byte{96}),
				makePlainPacket(97, 94_080, []byte{0xa0}), makePlainPacket(98, 95_040, []byte{98}), makePlainPacket(99, 95_520, []byte{99}),
			},
		},
		{
			name: "PartialGapWithBackupAtSequence98",
			input: []rtp.Packet{
				anchor, carrier, makePlainPacket(96, 93_120, []byte{96}),
				makePlainPacket(97, 93_600, []byte{97}), makePlainPacket(98, 94_080, []byte{0xa0}), makePlainPacket(99, 95_040, []byte{99}),
			},
			expected: []rtp.Packet{
				anchor, primary, makePlainPacket(96, 93_120, []byte{96}),
				makePlainPacket(97, 93_600, []byte{97}), makePlainPacket(98, 94_080, []byte{0xa0}), makePlainPacket(99, 95_040, []byte{99}),
			},
		},
		{
			name:     "IdenticalCopiesIdentifyOnePacket",
			input:    []rtp.Packet{first, timestampREDPacket(t, 3, 2_880, timestampBlock(960, 2), timestampBlock(960, 2))},
			expected: []rtp.Packet{first, second, third},
		},
		{
			name:     "ConflictingCopiesLeaveOriginalDeliverable",
			input:    []rtp.Packet{first, timestampREDPacket(t, 3, 2_880, timestampBlock(960, 2), timestampBlock(960, 0xf2)), second},
			expected: []rtp.Packet{first, third, second},
		},
		{
			name:     "UnsupportedCopiesDoNotCompleteGap",
			input:    []rtp.Packet{first, timestampREDPacket(t, 4, 3_840, timestampBlock(1_920, 2), unsupported), second, third},
			expected: []rtp.Packet{first, makePlainPacket(4, 3_840, []byte{4}), second, third},
		},
	})

	for _, test := range []struct {
		name  string
		block Block
	}{
		{"UnsupportedPayloadType", Block{PayloadType: testOpusPayloadType - 1, TimestampOffset: 960, Payload: []byte{0xf1}}},
		{"ZeroTimestampOffset", timestampBlock(0, 0xf1)},
		{"EmptyPayload", Block{PayloadType: testOpusPayloadType, TimestampOffset: 960}},
	} {
		runTimestampRecoveryTests(t, []timestampRecoveryTest{{
			name:     test.name,
			input:    []rtp.Packet{makePlainPacket(0, 0, []byte{0}), timestampREDPacket(t, 2, 1_920, test.block), first},
			expected: []rtp.Packet{makePlainPacket(0, 0, []byte{0}), second, first},
		}})
	}
}

func TestReceiverFixedDurationMapping(t *testing.T) {
	first := makePlainPacket(1, 960, []byte{1})
	padding := paddingOnlyPacket(3, 2_880, 1)
	other := makePlainPacket(3, 2_880, []byte{0xa3})
	other.PayloadType--

	runTimestampRecoveryTests(t, []timestampRecoveryTest{
		{
			name:     "ReversedSparseBlocks",
			input:    []rtp.Packet{timestampREDPacket(t, 100, 96_000, timestampBlock(960, 99), timestampBlock(2_880, 97))},
			expected: []rtp.Packet{makePlainPacket(97, 93_120, []byte{97}), makePlainPacket(99, 95_040, []byte{99}), makePlainPacket(100, 96_000, []byte{100})},
		},
		{
			name:     "OffsetBetweenPacketsLeavesOriginalDeliverable",
			input:    []rtp.Packet{timestampREDPacket(t, 100, 96_000, timestampBlock(1_000, 0xf9), timestampBlock(1_920, 98)), makePlainPacket(99, 95_040, []byte{99})},
			expected: []rtp.Packet{makePlainPacket(98, 94_080, []byte{98}), makePlainPacket(100, 96_000, []byte{100}), makePlainPacket(99, 95_040, []byte{99})},
		},
		{
			name:     "ContradictoryTimingFallsBackToCompleteGap",
			input:    []rtp.Packet{first, timestampREDPacket(t, 4, 2_880, timestampBlock(960, 3), timestampBlock(1_440, 2))},
			expected: []rtp.Packet{first, makePlainPacket(2, 1_440, []byte{2}), makePlainPacket(3, 1_920, []byte{3}), makePlainPacket(4, 2_880, []byte{4})},
		},
		{
			name:     "ContradictoryTimingLeavesPartialGapMissing",
			input:    []rtp.Packet{makePlainPacket(95, 92_160, []byte{95}), timestampREDPacket(t, 100, 96_000, timestampBlock(1_920, 97)), makePlainPacket(97, 94_080, []byte{97})},
			expected: []rtp.Packet{makePlainPacket(95, 92_160, []byte{95}), makePlainPacket(100, 96_000, []byte{100}), makePlainPacket(97, 94_080, []byte{97})},
		},
		{
			name:     "ObservedPaddingFallsBackDespiteMatchingMediaCadence",
			input:    []rtp.Packet{first, padding, timestampREDPacket(t, 4, 3_840, timestampBlock(960, 2))},
			expected: []rtp.Packet{first, makePlainPacket(2, 2_880, []byte{2}), makePlainPacket(4, 3_840, []byte{4})},
		},
		{
			name:     "OtherPayloadTypeFallsBackDespiteMatchingMediaCadence",
			input:    []rtp.Packet{first, other, timestampREDPacket(t, 4, 3_840, timestampBlock(960, 2))},
			expected: []rtp.Packet{first, other, makePlainPacket(2, 2_880, []byte{2}), makePlainPacket(4, 3_840, []byte{4})},
		},
	}, ReceiverPacketDuration(20*time.Millisecond))
}
