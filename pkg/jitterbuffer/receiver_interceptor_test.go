// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"bytes"
	"errors"
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
	factory, err := NewInterceptor(
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, i)
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
	err = i.Close()
	assert.NoError(t, err)
}

func TestReceiverBuffersAndPlaysout(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewInterceptor(
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	assert.EqualValues(t, 0, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, i)

	stream.ReceiveRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	for s := 0; s < 61; s++ {
		stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: uint16(s),
		}})
	}
	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)
	for s := 0; s < 10; s++ {
		read := <-stream.ReadRTP()
		seq := read.Packet.Header.SequenceNumber
		assert.EqualValues(t, uint16(s), seq)
	}
	assert.NoError(t, stream.Close())
	err = i.Close()
	assert.NoError(t, err)
}

type MockRTPReader struct {
	readFunc func([]byte, interceptor.Attributes) (int, interceptor.Attributes, error)
}

func (m *MockRTPReader) Read(data []byte, attrs interceptor.Attributes) (int, interceptor.Attributes, error) {
	if m.readFunc != nil {
		return m.readFunc(data, attrs)
	}
	return 0, nil, errors.New("mock function not implemented")
}

func NewMockRTPReader(readFunc func([]byte, interceptor.Attributes) (int, interceptor.Attributes, error)) *MockRTPReader {
	return &MockRTPReader{
		readFunc: readFunc,
	}
}

func TestReceiverInterceptorHonorsBufferLength(t *testing.T) {
	buf := []byte{0x80, 0x88, 0xe6, 0xfd, 0x01, 0x01, 0x01, 0x01, 0x01,
		0xde, 0xad, 0xbe, 0xef, 0x01, 0x01, 0x01, 0x01, 0x01}
	readBuf := make([]byte, 2048)
	copy(readBuf[0:], buf)
	copy(readBuf[17:], buf)
	factory, err := NewInterceptor(
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")

	rtpReadFn := NewMockRTPReader(func(data []byte, attrs interceptor.Attributes) (int, interceptor.Attributes, error) {
		copy(data, readBuf)
		return 7, attrs, nil
	})
	reader := i.BindRemoteStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, rtpReadFn)

	bufLen, _, err := reader.Read(readBuf, interceptor.Attributes{})
	assert.Contains(t, err.Error(), "7 < 12")
	assert.Equal(t, 0, bufLen)

	err = i.Close()
	assert.NoError(t, err)

}
