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
	tb testing.TB, info *interceptor.StreamInfo, downstream interceptor.RTPReader, opts ...ReceiverOption,
) interceptor.RTPReader {
	tb.Helper()

	factory, err := NewReceiverInterceptor(opts...)
	require.NoError(tb, err)

	receiver, err := factory.NewInterceptor("")
	require.NoError(tb, err)
	tb.Cleanup(func() {
		require.NoError(tb, receiver.Close())
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

func TestReceiverInterceptorConsumesPaddingOnlyPackets(t *testing.T) {
	media := timestampREDPacket(t, 2, 960)
	opusPadding := paddingOnlyPacket(4, 1_920, 1)
	opusPadding.PayloadType = testOpusPayloadType
	downstream := readerForPackets(t, paddingOnlyPacket(1, 960, 50), media, opusPadding)
	reader := newTestReceiverWithOptions(t, opusStreamInfo(), downstream)

	assertTimestampOutputs(t, reader, makePlainPacket(2, 960, []byte{2}))
}

func TestReceiverInterceptorPreservesReadErrors(t *testing.T) {
	tests := []struct {
		name    string
		padding []rtp.Packet
	}{
		{name: "BeforePadding"},
		{name: "AfterREDPadding", padding: []rtp.Packet{paddingOnlyPacket(1, 960, 50)}},
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

func TestReceiverInterceptorPassesThroughAnotherSSRC(t *testing.T) {
	packet := makePlainPacket(1, 960, []byte{1})
	packet.PayloadType = testREDPayloadType
	packet.SSRC++
	raw := marshalPacket(t, packet)
	attributes := interceptor.Attributes{"custom": "value"}
	downstream := &queuedRTPReader{results: []readerResult{{raw: raw, attributes: attributes}}}
	reader := newTestReceiver(t, opusStreamInfo(), downstream)

	buffer := make([]byte, 1500)
	n, outputAttributes, err := reader.Read(buffer, nil)
	require.NoError(t, err)
	assert.Equal(t, raw, buffer[:n])
	assert.Equal(t, "value", outputAttributes.Get("custom"))
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

func TestInterceptorsBypassIneligibleStreams(t *testing.T) {
	tests := []struct {
		name   string
		change func(*interceptor.StreamInfo)
	}{
		{"NonOpus", func(info *interceptor.StreamInfo) { info.MimeType = "video/VP8" }},
		{"NoREDPayloadType", func(info *interceptor.StreamInfo) { info.PayloadTypeForwardErrorCorrection = 0 }},
		{"SamePayloadType", func(info *interceptor.StreamInfo) { info.PayloadTypeForwardErrorCorrection = info.PayloadType }},
		{"InvalidOpusPayloadType", func(info *interceptor.StreamInfo) { info.PayloadType = maxPayloadType + 1 }},
		{"InvalidREDPayloadType", func(info *interceptor.StreamInfo) { info.PayloadTypeForwardErrorCorrection = maxPayloadType + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := opusStreamInfo()
			test.change(info)

			writer := &capturedWriter{}
			assert.Same(t, writer, newTestSender(t, info, writer))

			reader := &queuedRTPReader{}
			assert.Same(t, reader, newTestReceiverWithOptions(t, info, reader))
		})
	}
}
