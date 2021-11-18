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

func TestGeneratorInterceptor(t *testing.T) {
	const interval = time.Millisecond * 10
	i, err := NewGeneratorInterceptor(
		GeneratorSize(64),
		GeneratorSkipLastN(2),
		GeneratorInterval(interval),
		GeneratorLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
	)
	assert.NoError(t, err)

	defer func() {
		assert.NoError(t, i.Close())
	}()

	rtpRead, rtpIn := rtpio.RTPPipe()
	rtcpOut, rtcpWrite := rtpio.RTCPPipe()

	rtpOut := i.Transform(rtcpWrite, rtpRead, nil)

	for _, seqNum := range []uint16{10, 11, 12, 14, 16, 18} {
		go func() {
			_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})
			assert.NoError(t, err2)
		}()

		p := &rtp.Packet{}
		_, err2 := rtpOut.ReadRTP(p)
		assert.NoError(t, err2)
		assert.Equal(t, seqNum, p.SequenceNumber)
	}

	time.Sleep(interval * 2) // wait for at least 2 nack packets

	pkts := make([]rtcp.Packet, 15)
	// ignore the first nack, it might only contain the sequence id 13 as missing
	_, err = rtcpOut.ReadRTCP(pkts)
	assert.NoError(t, err)

	_, err = rtcpOut.ReadRTCP(pkts)
	assert.NoError(t, err)
	assert.Equal(t, nil, pkts[1], "single packet RTCP Compound Packet expected")

	p, ok := pkts[0].(*rtcp.TransportLayerNack)
	assert.True(t, ok, "TransportLayerNack rtcp packet expected, found: %T", pkts[0])

	assert.Equal(t, uint16(13), p.Nacks[0].PacketID)
	assert.Equal(t, rtcp.PacketBitmap(0b10), p.Nacks[0].LostPackets) // we want packets: 13, 15 (not packet 17, because skipLastN is setReceived to 2)
}

func TestGeneratorInterceptor_InvalidSize(t *testing.T) {
	_, err := NewGeneratorInterceptor(GeneratorSize(5))

	assert.Error(t, err, ErrInvalidSize)
}
