// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package nack

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestResponderInterceptor(t *testing.T) {
	tests := []struct {
		name string
		opts []ResponderOption
	}{
		{
			name: "with copy",
			opts: []ResponderOption{
				ResponderSize(8),
				ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
			},
		},
		{
			name: "without copy",
			opts: []ResponderOption{
				ResponderSize(8),
				ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
				DisableCopy(),
			},
		},
	}

	for _, item := range tests {
		item := item
		t.Run(item.name, func(t *testing.T) {
			f, err := NewResponderInterceptor(item.opts...)
			require.NoError(t, err)

			i, err := f.NewInterceptor("")
			require.NoError(t, err)

			stream := test.NewMockStream(&interceptor.StreamInfo{
				SSRC:         1,
				RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
			}, i)
			defer func() {
				require.NoError(t, stream.Close())
			}()

			for _, seqNum := range []uint16{10, 11, 12, 14, 15} {
				require.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}}))

				select {
				case p := <-stream.WrittenRTP():
					require.Equal(t, seqNum, p.SequenceNumber)
				case <-time.After(10 * time.Millisecond):
					t.Fatal("written rtp packet not found")
				}
			}

			stream.ReceiveRTCP([]rtcp.Packet{
				&rtcp.TransportLayerNack{
					MediaSSRC:  1,
					SenderSSRC: 2,
					Nacks: []rtcp.NackPair{
						{PacketID: 11, LostPackets: 0b1011}, // sequence numbers: 11, 12, 13, 15
					},
				},
			})

			// seq number 13 was never sent, so it can't be resent
			for _, seqNum := range []uint16{11, 12, 15} {
				select {
				case p := <-stream.WrittenRTP():
					require.Equal(t, seqNum, p.SequenceNumber)
				case <-time.After(10 * time.Millisecond):
					t.Fatal("written rtp packet not found")
				}
			}

			select {
			case p := <-stream.WrittenRTP():
				t.Errorf("no more rtp packets expected, found sequence number: %v", p.SequenceNumber)
			case <-time.After(10 * time.Millisecond):
			}
		})
	}
}

func TestResponderInterceptor_InvalidSize(t *testing.T) {
	f, _ := NewResponderInterceptor(ResponderSize(5))

	_, err := f.NewInterceptor("")
	require.Error(t, err, ErrInvalidSize)
}

func TestResponderInterceptor_DisableCopy(t *testing.T) {
	f, err := NewResponderInterceptor(
		ResponderSize(8),
		ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
		DisableCopy(),
	)
	require.NoError(t, err)
	i, err := f.NewInterceptor("id")
	require.NoError(t, err)
	_, ok := i.(*ResponderInterceptor).packetFactory.(*rtpbuffer.PacketFactoryNoOp)
	require.True(t, ok)
}

// this test is only useful when being run with the race detector, it won't fail otherwise:
//
// go test -race ./pkg/nack/
func TestResponderInterceptor_Race(t *testing.T) {
	f, err := NewResponderInterceptor(
		ResponderSize(32768),
		ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	require.NoError(t, err)

	i, err := f.NewInterceptor("")
	require.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         1,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, i)

	for seqNum := uint16(0); seqNum < 500; seqNum++ {
		require.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}}))

		// 25% packet loss
		if seqNum%4 == 0 {
			time.Sleep(time.Duration(seqNum%23) * time.Millisecond)
			stream.ReceiveRTCP([]rtcp.Packet{
				&rtcp.TransportLayerNack{
					MediaSSRC:  1,
					SenderSSRC: 2,
					Nacks: []rtcp.NackPair{
						{PacketID: seqNum, LostPackets: 0},
					},
				},
			})
		}
	}
}

func TestResponderInterceptor_StreamFilter(t *testing.T) {
	f, err := NewResponderInterceptor(
		ResponderSize(8),
		ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
		ResponderStreamsFilter(func(info *interceptor.StreamInfo) bool {
			return info.SSRC != 1 // enable nacks only for ssrc 2
		}))

	require.NoError(t, err)

	i, err := f.NewInterceptor("")
	require.NoError(t, err)

	streamWithoutNacks := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         1,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, i)
	defer func() {
		require.NoError(t, streamWithoutNacks.Close())
	}()

	streamWithNacks := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:         2,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "nack"}},
	}, i)
	defer func() {
		require.NoError(t, streamWithNacks.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 14, 15} {
		require.NoError(t, streamWithoutNacks.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum, SSRC: 1}}))
		require.NoError(t, streamWithNacks.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum, SSRC: 2}}))

		select {
		case p := <-streamWithoutNacks.WrittenRTP():
			require.Equal(t, seqNum, p.SequenceNumber)
		case <-time.After(10 * time.Millisecond):
			t.Fatal("written rtp packet not found")
		}

		select {
		case p := <-streamWithNacks.WrittenRTP():
			require.Equal(t, seqNum, p.SequenceNumber)
		case <-time.After(10 * time.Millisecond):
			t.Fatal("written rtp packet not found")
		}
	}

	streamWithoutNacks.ReceiveRTCP([]rtcp.Packet{
		&rtcp.TransportLayerNack{
			MediaSSRC:  1,
			SenderSSRC: 2,
			Nacks: []rtcp.NackPair{
				{PacketID: 11, LostPackets: 0b1011}, // sequence numbers: 11, 12, 13, 15
			},
		},
	})

	streamWithNacks.ReceiveRTCP([]rtcp.Packet{
		&rtcp.TransportLayerNack{
			MediaSSRC:  2,
			SenderSSRC: 2,
			Nacks: []rtcp.NackPair{
				{PacketID: 11, LostPackets: 0b1011}, // sequence numbers: 11, 12, 13, 15
			},
		},
	})

	select {
	case <-streamWithNacks.WrittenRTP():
	case <-time.After(10 * time.Millisecond):
		t.Fatal("nack response expected")
	}

	select {
	case <-streamWithoutNacks.WrittenRTP():
		t.Fatal("no nack response expected")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestResponderInterceptor_RFC4588(t *testing.T) {
	f, err := NewResponderInterceptor()
	require.NoError(t, err)

	i, err := f.NewInterceptor("")
	require.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:                      1,
		SSRCRetransmission:        2,
		PayloadTypeRetransmission: 2,
		RTCPFeedback:              []interceptor.RTCPFeedback{{Type: "nack"}},
	}, i)
	defer func() {
		require.NoError(t, stream.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 14, 15} {
		require.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}}))

		select {
		case p := <-stream.WrittenRTP():
			require.Equal(t, seqNum, p.SequenceNumber)
		case <-time.After(10 * time.Millisecond):
			t.Fatal("written rtp packet not found")
		}
	}

	stream.ReceiveRTCP([]rtcp.Packet{
		&rtcp.TransportLayerNack{
			MediaSSRC:  1,
			SenderSSRC: 2,
			Nacks: []rtcp.NackPair{
				{PacketID: 11, LostPackets: 0b1011}, // sequence numbers: 11, 12, 13, 15
			},
		},
	})

	// seq number 13 was never sent, so it can't be resent
	for _, seqNum := range []uint16{11, 12, 15} {
		select {
		case p := <-stream.WrittenRTP():
			require.Equal(t, uint32(2), p.SSRC)
			require.Equal(t, uint8(2), p.PayloadType)
			require.Equal(t, binary.BigEndian.Uint16(p.Payload), seqNum)
		case <-time.After(10 * time.Millisecond):
			t.Fatal("written rtp packet not found")
		}
	}

	select {
	case p := <-stream.WrittenRTP():
		t.Errorf("no more rtp packets expected, found sequence number: %v", p.SequenceNumber)
	case <-time.After(10 * time.Millisecond):
	}
}
