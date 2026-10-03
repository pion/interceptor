// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"errors"
	"io"
	"testing"

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

	factory, err := NewReceiverInterceptor()
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

func TestReceiverInterceptorRejectsInvalidRED(t *testing.T) {
	validPacket := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    testREDPayloadType,
			SequenceNumber: 2,
			SSRC:           testSSRC,
		},
		Payload: marshalREDPayload(t, Payload{
			PrimaryBlock: Block{PayloadType: testOpusPayloadType, Payload: []byte{0x02}},
		}),
	}
	downstream := &queuedRTPReader{results: []readerResult{
		{
			raw: marshalPacket(t, rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    testREDPayloadType,
					SequenceNumber: 1,
					SSRC:           testSSRC,
				},
				Payload: []byte{followBit},
			}),
		},
		{raw: marshalPacket(t, validPacket)},
	}}
	reader := newTestReceiver(t, opusStreamInfo(), downstream)
	buffer := make([]byte, 1500)

	n, _, err := reader.Read(buffer, interceptor.Attributes{})
	assert.Zero(t, n)
	assert.ErrorIs(t, err, errInvalidREDPayload)

	n, _, err = reader.Read(buffer, interceptor.Attributes{})
	require.NoError(t, err)
	var outputPacket rtp.Packet
	require.NoError(t, outputPacket.Unmarshal(buffer[:n]))
	assert.Equal(t, testOpusPayloadType, outputPacket.PayloadType)
	assert.Equal(t, []byte{0x02}, outputPacket.Payload)
}

func TestReceiverInterceptorRejectsUnexpectedPrimary(t *testing.T) {
	tests := []struct {
		name    string
		primary Block
		err     error
	}{
		{
			name:    "PayloadType",
			primary: Block{PayloadType: testOpusPayloadType - 1, Payload: []byte{0x01}},
			err:     errUnexpectedPrimaryPayloadType,
		},
		{
			name:    "EmptyPayload",
			primary: Block{PayloadType: testOpusPayloadType},
			err:     errEmptyPrimaryPayload,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputPacket := rtp.Packet{
				Header: rtp.Header{
					Version:     2,
					PayloadType: testREDPayloadType,
					SSRC:        testSSRC,
				},
				Payload: marshalREDPayload(t, Payload{PrimaryBlock: test.primary}),
			}
			downstream := &queuedRTPReader{results: []readerResult{{raw: marshalPacket(t, inputPacket)}}}
			reader := newTestReceiver(t, opusStreamInfo(), downstream)

			n, _, err := reader.Read(make([]byte, 1500), interceptor.Attributes{})
			assert.Zero(t, n)
			assert.ErrorIs(t, err, test.err)
		})
	}
}

func TestReceiverInterceptorPassesThroughReadErrors(t *testing.T) {
	downstream := &queuedRTPReader{results: []readerResult{{err: errTestRead}}}
	reader := newTestReceiver(t, opusStreamInfo(), downstream)

	n, _, err := reader.Read(make([]byte, 1500), interceptor.Attributes{})
	assert.Zero(t, n)
	assert.ErrorIs(t, err, errTestRead)
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
