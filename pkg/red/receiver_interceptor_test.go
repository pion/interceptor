// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTestRead = errors.New("read failed")

type readerResult struct {
	raw        []byte
	attributes interceptor.Attributes
	err        error
}

type queuedRTPReader struct {
	results []readerResult
}

func (reader *queuedRTPReader) Read(
	buffer []byte, _ interceptor.Attributes,
) (int, interceptor.Attributes, error) {
	if len(reader.results) == 0 {
		return 0, nil, io.EOF
	}

	result := reader.results[0]
	reader.results = reader.results[1:]
	if result.err != nil {
		return 0, result.attributes, result.err
	}
	if len(buffer) < len(result.raw) {
		return 0, result.attributes, io.ErrShortBuffer
	}

	copy(buffer, result.raw)

	return len(result.raw), result.attributes, nil
}

func newTestReceiver(
	t *testing.T, info *interceptor.StreamInfo, downstream interceptor.RTPReader,
) interceptor.RTPReader {
	t.Helper()

	// These synthetic fixtures declare continuous 20 ms packetization.
	return newTestReceiverWithOptions(t, info, downstream, ReceiverPacketDuration(20*time.Millisecond))
}

func newTestReceiverWithOptions(
	t *testing.T, info *interceptor.StreamInfo, downstream interceptor.RTPReader, opts ...ReceiverOption,
) interceptor.RTPReader {
	t.Helper()

	factory, err := NewReceiverInterceptor(opts...)
	require.NoError(t, err)

	receiver, err := factory.NewInterceptor("")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, receiver.Close())
	})

	return receiver.BindRemoteStream(info, downstream)
}

func marshalPacket(t *testing.T, packet rtp.Packet) []byte {
	t.Helper()

	raw, err := packet.Marshal()
	require.NoError(t, err)

	return raw
}

func marshalREDPayload(t *testing.T, payload Payload) []byte {
	t.Helper()

	raw, err := payload.Marshal()
	require.NoError(t, err)

	return raw
}

func paddingOnlyPacket(sequenceNumber uint16, timestamp uint32, paddingSize byte) rtp.Packet {
	return rtp.Packet{Header: rtp.Header{
		Version:        2,
		Padding:        true,
		PayloadType:    testREDPayloadType,
		SequenceNumber: sequenceNumber,
		Timestamp:      timestamp,
		SSRC:           testSSRC,
		PaddingSize:    paddingSize,
	}}
}

func TestReceiverInterceptorExtractsPrimaryOpus(t *testing.T) {
	outerHeader := rtp.Header{
		Version:        2,
		Padding:        true,
		Marker:         true,
		PayloadType:    testREDPayloadType,
		SequenceNumber: 20,
		Timestamp:      10_000,
		SSRC:           testSSRC,
		CSRC:           []uint32{0x11223344},
		PaddingSize:    4,
	}
	require.NoError(t, outerHeader.SetExtension(3, []byte{0xaa, 0xbb}))
	redPayload := marshalREDPayload(t, Payload{
		PrimaryBlock: Block{
			PayloadType: testOpusPayloadType,
			Payload:     []byte{0x03, 0x04, 0x05},
		},
	})
	inputPacket := rtp.Packet{Header: outerHeader, Payload: redPayload}
	inputAttributes := interceptor.Attributes{"custom": "value"}
	inputAttributes.SetRTPHeader(&outerHeader)
	downstream := &queuedRTPReader{results: []readerResult{{
		raw:        marshalPacket(t, inputPacket),
		attributes: inputAttributes,
	}}}
	reader := newTestReceiver(t, opusStreamInfo(), downstream)

	buffer := make([]byte, 1500)
	n, outputAttributes, err := reader.Read(buffer, interceptor.Attributes{})
	require.NoError(t, err)

	var outputPacket rtp.Packet
	require.NoError(t, outputPacket.Unmarshal(buffer[:n]))
	expectedHeader := outerHeader.Clone()
	expectedHeader.PayloadType = testOpusPayloadType
	assert.Equal(t, expectedHeader, outputPacket.Header)
	assert.Equal(t, []byte{0x03, 0x04, 0x05}, outputPacket.Payload)
	assert.Equal(t, byte(4), outputPacket.PaddingSize)
	assert.Equal(t, "value", outputAttributes.Get("custom"))

	cachedHeader, err := outputAttributes.GetRTPHeader(nil)
	require.NoError(t, err)
	assert.Equal(t, &expectedHeader, cachedHeader)
	originalCachedHeader, err := inputAttributes.GetRTPHeader(nil)
	require.NoError(t, err)
	assert.Equal(t, testREDPayloadType, originalCachedHeader.PayloadType)
	outputAttributes.Set("output-only", true)
	assert.Nil(t, inputAttributes.Get("output-only"), "the input attribute map must not be mutated")
}

func TestReceiverInterceptorConsumesPaddingOnlyPackets(t *testing.T) {
	info := opusStreamInfo()
	info.PayloadTypeForwardErrorCorrection = 63
	mediaPacket := func(sequenceNumber uint16, timestamp uint32) rtp.Packet {
		packet := makeREDPacket(t, rtp.Header{
			Version:        2,
			SequenceNumber: sequenceNumber,
			Timestamp:      timestamp,
			SSRC:           testSSRC,
		}, nil, []byte{byte(sequenceNumber)}) //nolint:gosec // Test sequence numbers are bounded below.
		packet.PayloadType = info.PayloadTypeForwardErrorCorrection

		return packet
	}

	redPadding := paddingOnlyPacket(21, 960, 50)
	redPadding.PayloadType = info.PayloadTypeForwardErrorCorrection
	opusPadding := paddingOnlyPacket(1, 960, 50)
	opusPadding.PayloadType = testOpusPayloadType
	terminalPadding := paddingOnlyPacket(5, 3_840, 1)
	terminalPadding.PayloadType = info.PayloadTypeForwardErrorCorrection

	tests := []struct {
		name     string
		input    []rtp.Packet
		expected []rtp.Packet
	}{
		{
			name:  "RED",
			input: []rtp.Packet{mediaPacket(20, 960), redPadding, mediaPacket(22, 1_920)},
			expected: []rtp.Packet{
				makePlainPacket(20, 960, []byte{20}), makePlainPacket(22, 1_920, []byte{22}),
			},
		},
		{
			name:     "Opus",
			input:    []rtp.Packet{opusPadding, mediaPacket(2, 1_920)},
			expected: []rtp.Packet{makePlainPacket(2, 1_920, []byte{2})},
		},
		{
			name: "TerminalGapIsNotFabricated",
			input: []rtp.Packet{
				makePlainPacket(1, 960, []byte{1}), makePlainPacket(2, 1_920, []byte{2}),
				makePlainPacket(3, 2_880, []byte{3}), terminalPadding,
			},
			expected: []rtp.Packet{
				makePlainPacket(1, 960, []byte{1}), makePlainPacket(2, 1_920, []byte{2}),
				makePlainPacket(3, 2_880, []byte{3}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			downstream := readerForPackets(t, test.input...)
			reader := newTestReceiverWithOptions(t, info, downstream)
			for _, expected := range test.expected {
				packet, _ := readOutputPacket(t, reader)
				assert.Equal(t, expected.Header, packet.Header)
				assert.Equal(t, expected.Payload, packet.Payload)
			}

			n, _, err := reader.Read(make([]byte, 1500), interceptor.Attributes{})
			assert.Zero(t, n)
			assert.ErrorIs(t, err, io.EOF)
			assert.Empty(t, downstream.results)
		})
	}
}

func TestReceiverInterceptorPreservesReadErrors(t *testing.T) {
	opusPadding := paddingOnlyPacket(1, 960, 50)
	opusPadding.PayloadType = testOpusPayloadType
	tests := []struct {
		name    string
		padding []rtp.Packet
	}{
		{name: "BeforePadding"},
		{name: "AfterREDPadding", padding: []rtp.Packet{paddingOnlyPacket(1, 960, 50)}},
		{name: "AfterOpusPadding", padding: []rtp.Packet{opusPadding}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			downstream := readerForPackets(t, test.padding...)
			downstream.results = append(downstream.results, readerResult{err: errTestRead})
			reader := newTestReceiver(t, opusStreamInfo(), downstream)

			n, _, err := reader.Read(make([]byte, 1500), interceptor.Attributes{})
			assert.Zero(t, n)
			assert.ErrorIs(t, err, errTestRead)
			assert.Empty(t, downstream.results)
		})
	}
}

func TestReceiverInterceptorPassesThroughPlainAndUnrelatedRTP(t *testing.T) {
	tests := []struct {
		name   string
		header rtp.Header
	}{
		{
			name: "PlainOpus",
			header: rtp.Header{
				Version:        2,
				PayloadType:    testOpusPayloadType,
				SequenceNumber: 1,
				SSRC:           testSSRC,
			},
		},
		{
			name: "UnrelatedPayloadType",
			header: rtp.Header{
				Version:        2,
				PayloadType:    101,
				SequenceNumber: 2,
				SSRC:           testSSRC,
			},
		},
		{
			name: "AnotherSSRC",
			header: rtp.Header{
				Version:        2,
				PayloadType:    testREDPayloadType,
				SequenceNumber: 3,
				SSRC:           testSSRC + 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputAttributes := interceptor.Attributes{"custom": test.name}
			inputRaw := marshalPacket(t, rtp.Packet{
				Header:  test.header,
				Payload: []byte{0x01, 0x02, 0x03},
			})
			downstream := &queuedRTPReader{results: []readerResult{{
				raw:        inputRaw,
				attributes: inputAttributes,
			}}}
			reader := newTestReceiver(t, opusStreamInfo(), downstream)

			buffer := make([]byte, 1500)
			n, outputAttributes, err := reader.Read(buffer, interceptor.Attributes{})
			require.NoError(t, err)
			assert.Equal(t, inputRaw, buffer[:n])
			assert.Equal(t, test.name, outputAttributes.Get("custom"))
		})
	}
}

func TestReceiverInterceptorRejectsInvalidPackets(t *testing.T) {
	header := rtp.Header{
		Version:        2,
		PayloadType:    testREDPayloadType,
		SequenceNumber: 1,
		SSRC:           testSSRC,
	}
	malformedPadding := marshalPacket(t, paddingOnlyPacket(1, 960, 4))
	malformedPadding[len(malformedPadding)-1] = 0
	validPacket := makeREDPacket(t, rtp.Header{
		Version:        2,
		SequenceNumber: 2,
		Timestamp:      1_920,
		SSRC:           testSSRC,
	}, nil, []byte{2})
	tests := []struct {
		name    string
		raw     []byte
		err     error
		message string
	}{
		{
			name: "EmptyREDWithoutPadding",
			raw:  marshalPacket(t, rtp.Packet{Header: header}),
			err:  errInvalidREDPayload,
		},
		{
			name: "TruncatedREDHeader",
			raw:  marshalPacket(t, rtp.Packet{Header: header, Payload: []byte{followBit}}),
			err:  errInvalidREDPayload,
		},
		{
			name: "PrimaryPayloadType",
			raw: marshalPacket(t, rtp.Packet{Header: header, Payload: marshalREDPayload(t, Payload{
				PrimaryBlock: Block{PayloadType: testOpusPayloadType - 1, Payload: []byte{1}},
			})}),
			err: errUnexpectedPrimaryPayloadType,
		},
		{
			name: "EmptyPrimaryPayload",
			raw: marshalPacket(t, rtp.Packet{Header: header, Payload: marshalREDPayload(t, Payload{
				PrimaryBlock: Block{PayloadType: testOpusPayloadType},
			})}),
			err: errEmptyPrimaryPayload,
		},
		{
			name:    "MalformedRTPPadding",
			raw:     malformedPadding,
			message: "invalid RTP padding",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			downstream := &queuedRTPReader{results: []readerResult{
				{raw: test.raw}, {raw: marshalPacket(t, validPacket)},
			}}
			reader := newTestReceiver(t, opusStreamInfo(), downstream)

			n, _, err := reader.Read(make([]byte, 1500), interceptor.Attributes{})
			assert.Zero(t, n)
			if test.err != nil {
				assert.ErrorIs(t, err, test.err)
			} else {
				assert.ErrorContains(t, err, test.message)
				assert.NotErrorIs(t, err, errInvalidREDPayload)
			}

			packet, _ := readOutputPacket(t, reader)
			assert.Equal(t, uint16(2), packet.SequenceNumber)
			assert.Equal(t, testOpusPayloadType, packet.PayloadType)
			assert.Equal(t, []byte{2}, packet.Payload)
			assert.Empty(t, downstream.results)
		})
	}
}

func TestReceiverInterceptorBypassesIneligibleStream(t *testing.T) {
	info := opusStreamInfo()
	info.MimeType = "video/VP8"
	downstream := &queuedRTPReader{}

	factory, err := NewReceiverInterceptor()
	require.NoError(t, err)
	receiver, err := factory.NewInterceptor("")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, receiver.Close())
	})

	assert.Same(t, downstream, receiver.BindRemoteStream(info, downstream))
}
