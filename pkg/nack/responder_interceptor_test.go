package nack

import (
	"testing"
	"time"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestResponderInterceptor(t *testing.T) {
	i, err := NewResponderInterceptor(
		ResponderSize(8),
		ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	defer func() {
		assert.NoError(t, i.Close())
	}()

	rtpOut, rtpWriter := rtpio.RTPPipe()
	rtcpReader, rtcpIn := rtpio.RTCPPipe()

	rtpIn := i.Transform(rtpWriter, nil, rtcpReader)

	for _, seqNum := range []uint16{10, 11, 12, 14, 15} {
		go func() {
			_, err := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{SSRC: 1, SequenceNumber: seqNum}})
			assert.NoError(t, err)
		}()

		p := &rtp.Packet{}
		_, err := rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, seqNum, p.SequenceNumber)
	}

	go func() {
		_, err := rtcpIn.WriteRTCP([]rtcp.Packet{
			&rtcp.TransportLayerNack{
				MediaSSRC:  1,
				SenderSSRC: 2,
				Nacks: []rtcp.NackPair{
					{PacketID: 11, LostPackets: 0b1011}, // sequence numbers: 11, 12, 13, 15
				},
			},
		})
		assert.NoError(t, err)
	}()

	// seq number 13 was never sent, so it can't be resent
	for _, seqNum := range []uint16{11, 12, 15} {
		p := &rtp.Packet{}
		_, err := rtpOut.ReadRTP(p)
		assert.NoError(t, err)
		assert.Equal(t, seqNum, p.SequenceNumber)
	}
}

func TestResponderInterceptor_InvalidSize(t *testing.T) {
	_, err := NewResponderInterceptor(ResponderSize(5))
	assert.Error(t, err, ErrInvalidSize)
}

// this test is only useful when being run with the race detector, it won't fail otherwise:
//
//     go test -race ./pkg/nack/
func TestResponderInterceptor_Race(t *testing.T) {
	i, err := NewResponderInterceptor(
		ResponderSize(32768),
		ResponderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	defer func() {
		assert.NoError(t, i.Close())
	}()

	rtcpReader, rtcpIn := rtpio.RTCPPipe()

	rtpIn := i.Transform(nil, nil, rtcpReader)

	for seqNum := uint16(0); seqNum < 500; seqNum++ {
		go func(seqNum uint16) {
			_, err := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{SSRC: 1, SequenceNumber: seqNum}})
			assert.NoError(t, err)
		}(seqNum)

		// 25% packet loss
		if seqNum%4 == 0 {
			time.Sleep(time.Duration(seqNum%23) * time.Millisecond)
			_, err := rtcpIn.WriteRTCP([]rtcp.Packet{
				&rtcp.TransportLayerNack{
					MediaSSRC:  1,
					SenderSSRC: 2,
					Nacks: []rtcp.NackPair{
						{PacketID: seqNum, LostPackets: 0},
					},
				},
			})
			assert.NoError(t, err)
		}
	}
}
