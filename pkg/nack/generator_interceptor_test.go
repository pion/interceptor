// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package nack

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestGeneratorInterceptor(t *testing.T) {
	const interval = time.Millisecond * 10
	f, err := NewGeneratorInterceptor(
		GeneratorSize(64),
		GeneratorSkipLastN(2),
		GeneratorInterval(interval),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	i, err := f.NewInterceptor("")
	assert.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         1,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, i)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 14, 16, 18} {
		stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})

		select {
		case r := <-stream.ReadRTP():
			assert.NoError(t, r.Err)
			assert.Equal(t, seqNum, r.Packet.SequenceNumber)
		case <-time.After(50 * time.Millisecond):
			t.Fatal("receiver rtp packet not found")
		}
	}

	time.Sleep(interval * 2) // wait for at least 2 nack packets

	select {
	case <-stream.WrittenRTCP():
		// ignore the first nack, it might only contain the sequence id 13 as missing
	default:
	}

	select {
	case pkts := <-stream.WrittenRTCP():
		assert.Equal(t, 1, len(pkts), "single packet RTCP Compound Packet expected")

		p, ok := pkts[0].(*rtcp.TransportLayerNack)
		assert.True(t, ok, "TransportLayerNack rtcp packet expected, found: %T", pkts[0])

		assert.Equal(t, uint16(13), p.Nacks[0].PacketID)
		// we want packets: 13, 15 (not packet 17, because skipLastN is setReceived to 2)
		assert.Equal(t, rtcp.PacketBitmap(0b10), p.Nacks[0].LostPackets)
	case <-time.After(10 * time.Millisecond):
		t.Fatal("written rtcp packet not found")
	}
}

func TestGeneratorInterceptor_InvalidSize(t *testing.T) {
	f, _ := NewGeneratorInterceptor(GeneratorSize(5))

	_, err := f.NewInterceptor("")
	assert.Error(t, err, ErrInvalidSize)
}

func TestGeneratorInterceptor_StreamFilter(t *testing.T) {
	const interval = time.Millisecond * 10
	f, err := NewGeneratorInterceptor(
		GeneratorSize(64),
		GeneratorSkipLastN(2),
		GeneratorInterval(interval),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
		GeneratorStreamsFilter(func(info *interceptor.StreamInfo) bool {
			return info.SSRC != 1 // enable nacks only for ssrc 2
		}),
	)
	assert.NoError(t, err)

	testInterceptor, err := f.NewInterceptor("")
	assert.NoError(t, err)

	streamWithoutNacks := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         1,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, testInterceptor)
	defer func() {
		assert.NoError(t, streamWithoutNacks.Close())
	}()

	streamWithNacks := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         2,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, testInterceptor)
	defer func() {
		assert.NoError(t, streamWithNacks.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 14, 16, 18} {
		streamWithNacks.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})
		streamWithoutNacks.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})
	}

	time.Sleep(interval * 2) // wait for at least 2 nack packets

	// both test streams receive RTCP packets about both test streams (as they both call BindRTCPWriter), so we
	// can check only one
	rtcpStream := streamWithNacks.WrittenRTCP()

	for {
		select {
		case pkts := <-rtcpStream:
			for _, pkt := range pkts {
				if nack, isNack := pkt.(*rtcp.TransportLayerNack); isNack {
					assert.NotEqual(t, uint32(1), nack.MediaSSRC) // check there are no nacks for ssrc 1
				}
			}
		default:
			return
		}
	}
}
