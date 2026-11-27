// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"bytes"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestBufferStart(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewInterceptor(
		WithLoggerFactory(logging.NewDefaultLoggerFactory()),
	)
	assert.NoError(t, err)

	testInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	assert.Zero(t, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, testInterceptor)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	stream.ReceiveRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)
	select {
	case pkt := <-stream.ReadRTP():
		assert.EqualValues(t, nil, pkt)
	default:
		// No data ready to read, this is what we expect
	}
	err = testInterceptor.Close()
	assert.NoError(t, err)
	assert.Zero(t, buf.Len())
}

func TestReceiverBuffersAndPlaysout(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewInterceptor(
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	testInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	assert.EqualValues(t, 0, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, testInterceptor)

	stream.ReceiveRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	for s := range 61 {
		stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: uint16(s), //nolint:gosec // G115
		}})
	}
	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)
	for s := range 10 {
		read := <-stream.ReadRTP()
		seq := read.Packet.Header.SequenceNumber
		assert.EqualValues(t, uint16(s), seq) //nolint:gosec // G115
	}
	assert.NoError(t, stream.Close())
	err = testInterceptor.Close()
	assert.NoError(t, err)
}

func newDynamicStream(t *testing.T, opts ...Option) (*test.MockStream, interceptor.Interceptor) {
	t.Helper()
	factory, err := NewInterceptor(WithBufferOptions(opts...))
	assert.NoError(t, err)
	testInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	return test.NewMockStream(&interceptor.StreamInfo{SSRC: 123456, ClockRate: 90000}, testInterceptor), testInterceptor
}

func readSeq(t *testing.T, stream *test.MockStream) uint16 {
	t.Helper()
	select {
	case read := <-stream.ReadRTP():
		assert.NoError(t, read.Err)

		return read.Packet.SequenceNumber
	case <-time.After(time.Second):
		assert.Fail(t, "timed out waiting for packet")

		return 0
	}
}

func assertNothingReadable(t *testing.T, stream *test.MockStream) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	select {
	case read := <-stream.ReadRTP():
		assert.Fail(t, "unexpected packet", "%v", read.Packet)
	default:
	}
}

func TestReceiverDynamicLengthBandwidth(t *testing.T) {
	stream, testInterceptor := newDynamicStream(t,
		WithDynamicLength(DynamicLengthConfig{TargetDelay: 100 * time.Millisecond}),
		WithBandwidthEstimator(&fakeBWE{1_000_000}),
	)
	for seq := range uint16(12) {
		stream.ReceiveRTP(dynPacket(seq))
	}
	assertNothingReadable(t, stream)

	stream.ReceiveRTP(dynPacket(12))
	assert.Equal(t, uint16(0), readSeq(t, stream))

	assert.NoError(t, stream.Close())
	assert.NoError(t, testInterceptor.Close())
}

func TestReceiverDynamicLengthLoss(t *testing.T) {
	stream, testInterceptor := newDynamicStream(t, WithMinimumPacketCount(5), WithDynamicLength(DynamicLengthConfig{}))
	for seq := range uint16(30) {
		if seq != 10 {
			stream.ReceiveRTP(dynPacket(seq))
		}
	}
	for seq := range uint16(26) {
		if seq != 10 {
			assert.Equal(t, seq, readSeq(t, stream))
		}
	}
	assertNothingReadable(t, stream)

	assert.NoError(t, stream.Close())
	assert.NoError(t, testInterceptor.Close())
}
