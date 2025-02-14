// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetdump

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

func TestSenderFilterEverythingOut(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewSenderInterceptor(
		RTPWriter(&buf),
		RTCPWriter(&buf),
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
		RTPFilter(func(*rtp.Packet) bool {
			return false
		}),
		RTCPFilter(func([]rtcp.Packet) bool {
			return false
		}),
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

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	assert.NoError(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})
	assert.NoError(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = testInterceptor.Close()
	assert.NoError(t, err)

	// Every packet should have been filtered out – nothing should be written.
	assert.Zero(t, buf.Len())
}

func TestSenderFilterNothing(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewSenderInterceptor(
		RTPWriter(&buf),
		RTCPWriter(&buf),
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
		RTPFilter(func(*rtp.Packet) bool {
			return true
		}),
		RTCPFilter(func([]rtcp.Packet) bool {
			return true
		}),
	)
	assert.NoError(t, err)

	testInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	assert.EqualValues(t, 0, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, testInterceptor)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	assert.NoError(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})
	assert.NoError(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = testInterceptor.Close()
	assert.NoError(t, err)

	assert.NotZero(t, buf.Len())
}

func TestSenderCustomBinaryFormatter(t *testing.T) {
	rtpBuf := bytes.Buffer{}
	rtcpBuf := bytes.Buffer{}

	factory, err := NewSenderInterceptor(
		RTPWriter(&rtpBuf),
		RTCPWriter(&rtcpBuf),
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
		// custom binary formatter to dump only seqno mod 256
		RTPBinaryFormatter(func(p *rtp.Packet, _ interceptor.Attributes) ([]byte, error) {
			return []byte{byte(p.SequenceNumber)}, nil
		}),
		// custom binary formatter to dump only DestinationSSRCs mod 256
		RTCPBinaryFormatter(func(p rtcp.Packet, _ interceptor.Attributes) ([]byte, error) {
			b := make([]byte, 0)
			for _, ssrc := range p.DestinationSSRC() {
				b = append(b, byte(ssrc))
			}

			return b, nil
		}),
	)
	assert.NoError(t, err)

	testInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	assert.EqualValues(t, 0, rtpBuf.Len())
	assert.EqualValues(t, 0, rtcpBuf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, testInterceptor)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  45,
	}})
	assert.NoError(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(123),
	}})
	assert.NoError(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = testInterceptor.Close()
	assert.NoError(t, err)

	// check that there is custom formatter results in buffer
	assert.Equal(t, []byte{123}, rtpBuf.Bytes())
	assert.Equal(t, []byte{45}, rtcpBuf.Bytes())
}

func TestSenderRTCPPerPacketFilter(t *testing.T) {
	buf := bytes.Buffer{}

	factory, err := NewSenderInterceptor(
		RTCPWriter(&buf),
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
		RTCPPerPacketFilter(func(packet rtcp.Packet) bool {
			_, isPli := packet.(*rtcp.PictureLossIndication)

			return isPli
		}),
		RTCPBinaryFormatter(func(p rtcp.Packet, _ interceptor.Attributes) ([]byte, error) {
			assert.IsType(t, &rtcp.PictureLossIndication{}, p)

			return []byte{123}, nil
		}),
	)
	assert.NoError(t, err)

	testInterceptor, err := factory.NewInterceptor("")
	assert.NoError(t, err)

	assert.EqualValues(t, 0, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, testInterceptor)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	assert.NoError(t, err)

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.ReceiverReport{
		SSRC: 789,
	}})
	assert.NoError(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = testInterceptor.Close()
	assert.NoError(t, err)

	// Only single PictureLossIndication should have been written.
	assert.Equal(t, []byte{123}, buf.Bytes())
}
