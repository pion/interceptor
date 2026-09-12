// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"
	"math"
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOpusPayloadType uint8  = 109
	testREDPayloadType  uint8  = 127
	testSSRC            uint32 = 0x10203040
)

var errTestWrite = errors.New("write failed")

type capturedWriter struct {
	packets    []rtp.Packet
	calls      int
	failOnCall int
	failErr    error
}

func (w *capturedWriter) Write(
	header *rtp.Header, payload []byte, _ interceptor.Attributes,
) (int, error) {
	w.calls++
	packet := rtp.Packet{
		Header:  header.Clone(),
		Payload: append([]byte(nil), payload...),
	}
	w.packets = append(w.packets, packet)

	if w.calls == w.failOnCall {
		return 0, w.failErr
	}

	return packet.MarshalSize(), nil
}

func newTestSender(
	t *testing.T, info *interceptor.StreamInfo, downstream interceptor.RTPWriter, opts ...SenderOption,
) interceptor.RTPWriter {
	t.Helper()

	factory, err := NewSenderInterceptor(opts...)
	require.NoError(t, err)

	sender, err := factory.NewInterceptor("")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sender.Close())
	})

	return sender.BindLocalStream(info, downstream)
}

func opusStreamInfo() *interceptor.StreamInfo {
	return &interceptor.StreamInfo{
		SSRC:                              testSSRC,
		PayloadType:                       testOpusPayloadType,
		PayloadTypeForwardErrorCorrection: testREDPayloadType,
		MimeType:                          "audio/opus",
	}
}

func writePacket(
	t *testing.T, writer interceptor.RTPWriter, header *rtp.Header, payload []byte,
) (int, error) {
	t.Helper()

	return writer.Write(header, payload, interceptor.Attributes{})
}

func requireREDPayload(t *testing.T, packet rtp.Packet) Payload {
	t.Helper()

	var payload Payload
	require.NoError(t, payload.Unmarshal(packet.Payload))

	return payload
}

func TestSenderInterceptorBuildsOpusRED(t *testing.T) {
	info := opusStreamInfo()
	info.MimeType = "AuDiO/OpUs"
	downstream := &capturedWriter{}
	writer := newTestSender(t, info, downstream)

	firstPayload := []byte{0x01, 0x02}
	firstHeader := &rtp.Header{
		Version:        2,
		PayloadType:    testOpusPayloadType,
		SequenceNumber: 10,
		Timestamp:      1_000,
		SSRC:           testSSRC,
	}
	_, err := writePacket(t, writer, firstHeader, firstPayload)
	require.NoError(t, err)
	require.Len(t, downstream.packets, 1)
	assert.Equal(t, testOpusPayloadType, downstream.packets[0].PayloadType)
	assert.Equal(t, firstPayload, downstream.packets[0].Payload)

	firstPayload[0] = 0xff
	secondHeader := &rtp.Header{
		Version:        2,
		PayloadType:    testOpusPayloadType,
		SequenceNumber: 11,
		Timestamp:      1_960,
		SSRC:           testSSRC,
	}
	_, err = writePacket(t, writer, secondHeader, []byte{0x03, 0x04})
	require.NoError(t, err)
	require.Len(t, downstream.packets, 2)
	assert.Equal(t, testREDPayloadType, downstream.packets[1].PayloadType)

	secondRED := requireREDPayload(t, downstream.packets[1])
	assert.Equal(t, []Block{{
		PayloadType:     testOpusPayloadType,
		TimestampOffset: 960,
		Payload:         []byte{0x01, 0x02},
	}}, secondRED.RedundantBlocks)
	assert.Equal(t, Block{PayloadType: testOpusPayloadType, Payload: []byte{0x03, 0x04}}, secondRED.PrimaryBlock)

	thirdHeader := &rtp.Header{
		Version:        2,
		Padding:        true,
		Marker:         true,
		PayloadType:    testOpusPayloadType,
		SequenceNumber: 12,
		Timestamp:      2_920,
		SSRC:           testSSRC,
		CSRC:           []uint32{0x55667788},
		PaddingSize:    4,
	}
	require.NoError(t, thirdHeader.SetExtension(3, []byte{0xaa, 0xbb}))
	expectedHeader := thirdHeader.Clone()
	expectedHeader.PayloadType = testREDPayloadType

	_, err = writePacket(t, writer, thirdHeader, []byte{0x05, 0x06})
	require.NoError(t, err)
	require.Len(t, downstream.packets, 3)
	assert.Equal(t, testOpusPayloadType, thirdHeader.PayloadType, "the caller's header must not be mutated")
	assert.Equal(t, expectedHeader, downstream.packets[2].Header)

	thirdRED := requireREDPayload(t, downstream.packets[2])
	assert.Equal(t, []Block{
		{PayloadType: testOpusPayloadType, TimestampOffset: 1_920, Payload: []byte{0x01, 0x02}},
		{PayloadType: testOpusPayloadType, TimestampOffset: 960, Payload: []byte{0x03, 0x04}},
	}, thirdRED.RedundantBlocks)
	assert.Equal(t, Block{PayloadType: testOpusPayloadType, Payload: []byte{0x05, 0x06}}, thirdRED.PrimaryBlock)
}

func TestSenderInterceptorHonorsMaxPacketSize(t *testing.T) {
	t.Run("DropsOldestBlockFirst", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream, SenderMaxPacketSize(37))
		payloads := [][]byte{
			make([]byte, 10),
			bytesWithValue(10, 0x02),
			bytesWithValue(10, 0x03),
		}

		for index, payload := range payloads {
			_, err := writePacket(t, writer, &rtp.Header{
				Version:        2,
				PayloadType:    testOpusPayloadType,
				SequenceNumber: uint16(index + 1),   //nolint:gosec // Test input is bounded.
				Timestamp:      uint32(index * 960), //nolint:gosec // Test input is bounded.
				SSRC:           testSSRC,
			}, payload)
			require.NoError(t, err)
		}

		require.Len(t, downstream.packets, 3)
		assert.LessOrEqual(t, downstream.packets[2].MarshalSize(), 37)
		redPayload := requireREDPayload(t, downstream.packets[2])
		assert.Equal(t, []Block{{
			PayloadType:     testOpusPayloadType,
			TimestampOffset: 960,
			Payload:         payloads[1],
		}}, redPayload.RedundantBlocks)
	})

	t.Run("UsesPlainOpusWhenNoBlockFits", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream, SenderMaxPacketSize(36))

		for index := range 2 {
			_, err := writePacket(t, writer, &rtp.Header{
				Version:        2,
				PayloadType:    testOpusPayloadType,
				SequenceNumber: uint16(index + 1),   //nolint:gosec // Test input is bounded.
				Timestamp:      uint32(index * 960), //nolint:gosec // Test input is bounded.
				SSRC:           testSSRC,
			}, make([]byte, 10))
			require.NoError(t, err)
		}

		require.Len(t, downstream.packets, 2)
		assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
	})
}

func TestSenderInterceptorResetsUnsafeHistory(t *testing.T) {
	t.Run("SequenceGap", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 1, 1_000, []byte{0x01})
		writeOpus(t, writer, 3, 2_920, []byte{0x03})
		writeOpus(t, writer, 4, 3_880, []byte{0x04})

		require.Len(t, downstream.packets, 3)
		assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
		redPayload := requireREDPayload(t, downstream.packets[2])
		assert.Equal(t, []byte{0x03}, redPayload.RedundantBlocks[0].Payload)
	})

	t.Run("Duplicate", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 10, 1_000, []byte{0x01})
		writeOpus(t, writer, 10, 1_000, []byte{0x02})
		writeOpus(t, writer, 11, 1_960, []byte{0x03})

		require.Len(t, downstream.packets, 3)
		assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
		redPayload := requireREDPayload(t, downstream.packets[2])
		assert.Equal(t, []byte{0x02}, redPayload.RedundantBlocks[0].Payload)
	})

	t.Run("Reordered", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 10, 1_000, []byte{0x01})
		writeOpus(t, writer, 9, 40, []byte{0x02})
		writeOpus(t, writer, 10, 1_000, []byte{0x03})

		require.Len(t, downstream.packets, 3)
		assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
		redPayload := requireREDPayload(t, downstream.packets[2])
		assert.Equal(t, []byte{0x02}, redPayload.RedundantBlocks[0].Payload)
	})

	t.Run("TimestampDiscontinuity", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 1, 1_000, []byte{0x01})
		writeOpus(t, writer, 2, 20_000, []byte{0x02})
		writeOpus(t, writer, 3, 20_960, []byte{0x03})

		require.Len(t, downstream.packets, 3)
		assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
		redPayload := requireREDPayload(t, downstream.packets[2])
		assert.Equal(t, []byte{0x02}, redPayload.RedundantBlocks[0].Payload)
	})

	t.Run("OversizedPayload", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 1, 1_000, []byte{0x01})
		writeOpus(t, writer, 2, 1_960, make([]byte, maxBlockLength+1))
		writeOpus(t, writer, 3, 2_920, []byte{0x03})
		writeOpus(t, writer, 4, 3_880, []byte{0x04})

		require.Len(t, downstream.packets, 4)
		assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
		assert.Equal(t, testOpusPayloadType, downstream.packets[2].PayloadType)
		assert.Equal(t, testREDPayloadType, downstream.packets[3].PayloadType)
	})

	t.Run("PayloadTypeSwitch", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 1, 1_000, []byte{0x01})
		_, err := writePacket(t, writer, &rtp.Header{
			Version:        2,
			PayloadType:    testREDPayloadType,
			SequenceNumber: 2,
			Timestamp:      1_960,
			SSRC:           testSSRC,
		}, []byte{0x7f})
		require.NoError(t, err)
		writeOpus(t, writer, 3, 2_920, []byte{0x03})

		require.Len(t, downstream.packets, 3)
		assert.Equal(t, testOpusPayloadType, downstream.packets[2].PayloadType)
	})
}

func TestSenderInterceptorHandlesWraparound(t *testing.T) {
	t.Run("SequenceNumber", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, math.MaxUint16, 1_000, []byte{0x01})
		writeOpus(t, writer, 0, 1_960, []byte{0x02})

		require.Len(t, downstream.packets, 2)
		assert.Equal(t, testREDPayloadType, downstream.packets[1].PayloadType)
	})

	t.Run("Timestamp", func(t *testing.T) {
		downstream := &capturedWriter{}
		writer := newTestSender(t, opusStreamInfo(), downstream)

		writeOpus(t, writer, 1, math.MaxUint32-479, []byte{0x01})
		writeOpus(t, writer, 2, 480, []byte{0x02})

		require.Len(t, downstream.packets, 2)
		redPayload := requireREDPayload(t, downstream.packets[1])
		assert.Equal(t, uint16(960), redPayload.RedundantBlocks[0].TimestampOffset)
	})
}

func TestSenderInterceptorBypassesIneligiblePackets(t *testing.T) {
	tests := []struct {
		name string
		info *interceptor.StreamInfo
	}{
		{name: "NonOpus", info: func() *interceptor.StreamInfo {
			info := opusStreamInfo()
			info.MimeType = "video/VP8"

			return info
		}()},
		{name: "NoREDPayloadType", info: func() *interceptor.StreamInfo {
			info := opusStreamInfo()
			info.PayloadTypeForwardErrorCorrection = 0

			return info
		}()},
		{name: "SamePayloadType", info: func() *interceptor.StreamInfo {
			info := opusStreamInfo()
			info.PayloadTypeForwardErrorCorrection = info.PayloadType

			return info
		}()},
		{name: "InvalidOpusPayloadType", info: func() *interceptor.StreamInfo {
			info := opusStreamInfo()
			info.PayloadType = maxPayloadType + 1

			return info
		}()},
		{name: "InvalidREDPayloadType", info: func() *interceptor.StreamInfo {
			info := opusStreamInfo()
			info.PayloadTypeForwardErrorCorrection = maxPayloadType + 1

			return info
		}()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			downstream := &capturedWriter{}
			writer := newTestSender(t, test.info, downstream)

			writeOpus(t, writer, 1, 1_000, []byte{0x01})
			writeOpus(t, writer, 2, 1_960, []byte{0x02})

			require.Len(t, downstream.packets, 2)
			assert.Equal(t, testOpusPayloadType, downstream.packets[0].PayloadType)
			assert.Equal(t, testOpusPayloadType, downstream.packets[1].PayloadType)
		})
	}
}

func TestSenderInterceptorDoesNotResetHistoryForAnotherSSRC(t *testing.T) {
	downstream := &capturedWriter{}
	writer := newTestSender(t, opusStreamInfo(), downstream)

	writeOpus(t, writer, 1, 1_000, []byte{0x01})
	_, err := writePacket(t, writer, &rtp.Header{
		Version:        2,
		PayloadType:    testOpusPayloadType,
		SequenceNumber: 100,
		Timestamp:      100_000,
		SSRC:           testSSRC + 1,
	}, []byte{0xff})
	require.NoError(t, err)
	writeOpus(t, writer, 2, 1_960, []byte{0x02})

	require.Len(t, downstream.packets, 3)
	assert.Equal(t, testREDPayloadType, downstream.packets[2].PayloadType)
}

func TestSenderInterceptorKeepsHistoryAfterWriteError(t *testing.T) {
	downstream := &capturedWriter{failOnCall: 1, failErr: errTestWrite}
	writer := newTestSender(t, opusStreamInfo(), downstream)

	_, err := writePacket(t, writer, &rtp.Header{
		Version:        2,
		PayloadType:    testOpusPayloadType,
		SequenceNumber: 1,
		Timestamp:      1_000,
		SSRC:           testSSRC,
	}, []byte{0x01})
	assert.ErrorIs(t, err, errTestWrite)

	writeOpus(t, writer, 2, 1_960, []byte{0x02})
	require.Len(t, downstream.packets, 2)
	redPayload := requireREDPayload(t, downstream.packets[1])
	assert.Equal(t, []byte{0x01}, redPayload.RedundantBlocks[0].Payload)
}

func TestSenderMaxPacketSizeValidation(t *testing.T) {
	factory, err := NewSenderInterceptor(SenderMaxPacketSize(0))
	require.NoError(t, err)

	_, err = factory.NewInterceptor("")
	assert.ErrorIs(t, err, errInvalidMaxPacketSize)
}

func writeOpus(
	t *testing.T, writer interceptor.RTPWriter, sequenceNumber uint16, timestamp uint32, payload []byte,
) {
	t.Helper()

	_, err := writePacket(t, writer, &rtp.Header{
		Version:        2,
		PayloadType:    testOpusPayloadType,
		SequenceNumber: sequenceNumber,
		Timestamp:      timestamp,
		SSRC:           testSSRC,
	}, payload)
	require.NoError(t, err)
}

func bytesWithValue(size int, value byte) []byte {
	payload := make([]byte, size)
	for index := range payload {
		payload[index] = value
	}

	return payload
}
