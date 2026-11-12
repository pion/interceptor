// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"io"
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeREDPacket(
	t *testing.T,
	header rtp.Header,
	redundantBlocks []Block,
	primaryPayload []byte,
) rtp.Packet {
	t.Helper()

	header.PayloadType = testREDPayloadType

	return rtp.Packet{
		Header: header,
		Payload: marshalREDPayload(t, Payload{
			RedundantBlocks: redundantBlocks,
			PrimaryBlock: Block{
				PayloadType: testOpusPayloadType,
				Payload:     primaryPayload,
			},
		}),
	}
}

func makePlainPacket(sequenceNumber uint16, timestamp uint32, payload []byte) rtp.Packet {
	return rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    testOpusPayloadType,
			SequenceNumber: sequenceNumber,
			Timestamp:      timestamp,
			SSRC:           testSSRC,
		},
		Payload: payload,
	}
}

func readerForPackets(t *testing.T, packets ...rtp.Packet) *queuedRTPReader {
	t.Helper()

	results := make([]readerResult, 0, len(packets))
	for _, packet := range packets {
		results = append(results, readerResult{raw: marshalPacket(t, packet)})
	}

	return &queuedRTPReader{results: results}
}

func readOutputPacket(
	t *testing.T, reader interceptor.RTPReader,
) (rtp.Packet, interceptor.Attributes) {
	t.Helper()

	buffer := make([]byte, 1500)
	n, attributes, err := reader.Read(buffer, interceptor.Attributes{})
	require.NoError(t, err)

	var packet rtp.Packet
	require.NoError(t, packet.Unmarshal(buffer[:n]))

	return packet, attributes
}

func TestReceiverInterceptorRecoversMissingPacketsInOrder(t *testing.T) {
	outerHeader := rtp.Header{
		Version:        2,
		Marker:         true,
		SequenceNumber: 3,
		Timestamp:      2_880,
		SSRC:           testSSRC,
		CSRC:           []uint32{0x11223344},
	}
	require.NoError(t, outerHeader.SetExtension(3, []byte{0xaa, 0xbb}))
	redPacket := makeREDPacket(t, outerHeader, []Block{
		{PayloadType: testOpusPayloadType, TimestampOffset: 1_920, Payload: []byte{0x01}},
		{PayloadType: testOpusPayloadType, TimestampOffset: 960, Payload: []byte{0x02}},
	}, []byte{0x03})
	inputAttributes := interceptor.Attributes{"arrival": "packet-3"}
	downstream := &queuedRTPReader{results: []readerResult{{
		raw:        marshalPacket(t, redPacket),
		attributes: inputAttributes,
	}}}
	reader := newTestReceiver(t, opusStreamInfo(), downstream)

	first, firstAttributes := readOutputPacket(t, reader)
	second, secondAttributes := readOutputPacket(t, reader)
	third, thirdAttributes := readOutputPacket(t, reader)

	assert.Equal(t, []uint16{1, 2, 3}, []uint16{
		first.SequenceNumber,
		second.SequenceNumber,
		third.SequenceNumber,
	})
	assert.Equal(t, []uint32{960, 1_920, 2_880}, []uint32{
		first.Timestamp,
		second.Timestamp,
		third.Timestamp,
	})
	assert.Equal(t, [][]byte{{0x01}, {0x02}, {0x03}}, [][]byte{
		first.Payload,
		second.Payload,
		third.Payload,
	})

	for _, recovered := range []rtp.Packet{first, second} {
		assert.Equal(t, testOpusPayloadType, recovered.PayloadType)
		assert.False(t, recovered.Marker)
		assert.False(t, recovered.Padding)
		assert.False(t, recovered.Extension)
		assert.Empty(t, recovered.Extensions)
		assert.Equal(t, outerHeader.CSRC, recovered.CSRC)
	}
	assert.True(t, third.Marker)
	assert.True(t, third.Extension)
	assert.Equal(t, outerHeader.CSRC, third.CSRC)

	for index, attributes := range []interceptor.Attributes{
		firstAttributes,
		secondAttributes,
		thirdAttributes,
	} {
		assert.Equal(t, "packet-3", attributes.Get("arrival"))
		cachedHeader, err := attributes.GetRTPHeader(nil)
		require.NoError(t, err)
		assert.Equal(t, uint16(index+1), cachedHeader.SequenceNumber) //nolint:gosec // Test input is bounded.
	}
}

func TestReceiverInterceptorHistory(t *testing.T) {
	third := timestampREDPacket(t, 3, 2_880, timestampBlock(1_920, 1), timestampBlock(960, 2))
	fourth := timestampREDPacket(t, 4, 3_840, timestampBlock(1_920, 2), timestampBlock(960, 3))
	wrapped := timestampREDPacket(t, 0, 480, timestampBlock(1_920, 1), timestampBlock(960, 2))
	tests := []struct {
		name     string
		incoming []rtp.Packet
		expected []rtp.Packet
	}{
		{
			name:     "OnlyMissingPackets",
			incoming: []rtp.Packet{makePlainPacket(1, 960, []byte{1}), third},
			expected: []rtp.Packet{
				makePlainPacket(1, 960, []byte{1}), makePlainPacket(2, 1_920, []byte{2}),
				makePlainPacket(3, 2_880, []byte{3}),
			},
		},
		{
			name: "LateAndDuplicatePackets",
			incoming: []rtp.Packet{
				third, makePlainPacket(2, 1_920, []byte{0xf2}), third, makePlainPacket(4, 3_840, []byte{4}),
			},
			expected: []rtp.Packet{
				makePlainPacket(1, 960, []byte{1}), makePlainPacket(2, 1_920, []byte{2}),
				makePlainPacket(3, 2_880, []byte{3}), makePlainPacket(4, 3_840, []byte{4}),
			},
		},
		{
			name:     "SeenPrimaryCanRecoverOlderPacket",
			incoming: []rtp.Packet{fourth, third},
			expected: []rtp.Packet{
				makePlainPacket(2, 1_920, []byte{2}), makePlainPacket(3, 2_880, []byte{3}),
				makePlainPacket(4, 3_840, []byte{4}), makePlainPacket(1, 960, []byte{1}),
			},
		},
		{
			name:     "UnseenReorderedPacket",
			incoming: []rtp.Packet{makePlainPacket(3, 2_880, []byte{3}), makePlainPacket(2, 1_920, []byte{2})},
			expected: []rtp.Packet{makePlainPacket(3, 2_880, []byte{3}), makePlainPacket(2, 1_920, []byte{2})},
		},
		{
			name: "SequenceAndTimestampWraparound",
			incoming: []rtp.Packet{
				wrapped, makePlainPacket(65_535, 4_294_966_816, []byte{0xf2}), makePlainPacket(1, 1_440, []byte{4}),
			},
			expected: []rtp.Packet{
				makePlainPacket(65_534, 4_294_965_856, []byte{1}), makePlainPacket(65_535, 4_294_966_816, []byte{2}),
				makePlainPacket(0, 480, []byte{0}), makePlainPacket(1, 1_440, []byte{4}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := newTestReceiver(t, opusStreamInfo(), readerForPackets(t, test.incoming...))
			assertTimestampOutputs(t, reader, test.expected...)
		})
	}
}

func TestReceiverInterceptorKeepsQueuedPacketAfterShortBuffer(t *testing.T) {
	redPacket := makeREDPacket(t, rtp.Header{
		Version:        2,
		SequenceNumber: 2,
		Timestamp:      1_920,
		SSRC:           testSSRC,
	}, []Block{{
		PayloadType:     testOpusPayloadType,
		TimestampOffset: 960,
		Payload:         []byte{0x01},
	}}, make([]byte, 50))
	downstream := readerForPackets(t, redPacket)
	reader := newTestReceiver(t, opusStreamInfo(), downstream)

	first, _ := readOutputPacket(t, reader)
	assert.Equal(t, uint16(1), first.SequenceNumber)
	assert.Empty(t, downstream.results, "the RED packet should be read only once")

	n, attributes, err := reader.Read(make([]byte, 20), interceptor.Attributes{})
	assert.Zero(t, n)
	assert.ErrorIs(t, err, io.ErrShortBuffer)
	cachedHeader, cacheErr := attributes.GetRTPHeader(nil)
	require.NoError(t, cacheErr)
	assert.Equal(t, uint16(2), cachedHeader.SequenceNumber)

	second, _ := readOutputPacket(t, reader)
	assert.Equal(t, uint16(2), second.SequenceNumber)
	assert.Len(t, second.Payload, 50)
}
