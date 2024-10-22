// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
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
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
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
	for s := 0; s < 910; s++ {
		stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: uint16(s), //nolint:gosec // G115
		}})
	}
	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)
	for s := 0; s < 50; s++ {
		read := <-stream.ReadRTP()
		if read.Err != nil {
			t.Fatal(read.Err)
		}
		seq := read.Packet.Header.SequenceNumber
		assert.EqualValues(t, uint16(s), seq) //nolint:gosec // G115
	}
	assert.NoError(t, stream.Close())
	err = testInterceptor.Close()
	assert.NoError(t, err)
}

func TestReceiverBuffersAndPlaysoutSkippingMissingPackets(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewInterceptor(
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
		WithSkipMissingPackets(),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("jitterbuffer")
	assert.NoError(t, err)

	assert.EqualValues(t, 0, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, i)

	for s := 0; s < 420; s++ {
		if s == 6 {
			s++
		}
		if s == 40 {
			s = s + 20
		}
		stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: uint16(s),
		}})
	}

	for s := 0; s < 100; s++ {
		read := <-stream.ReadRTP()
		if read.Err != nil {
			continue
		}
		seq := read.Packet.Header.SequenceNumber
		if s == 6 {
			s++
		}
		if s == 40 {
			s = s + 20
		}
		assert.EqualValues(t, uint16(s), seq)
	}
	assert.NoError(t, stream.Close())
	err = i.Close()
	assert.NoError(t, err)
}
