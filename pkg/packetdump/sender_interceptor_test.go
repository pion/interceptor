package packetdump

import (
	"bytes"
	"testing"
	"time"

	"github.com/pion/interceptor/v2/pkg/rtpio"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func testSenderFilter(t *testing.T, filter bool) {
	buf := bytes.Buffer{}

	i, err := NewSenderInterceptor(
		RTPWriter(&buf),
		RTCPWriter(&buf),
		Log(logging.NewDefaultLoggerFactory().NewLogger("test")),
		RTPFilter(func(pkt *rtp.Packet) bool {
			return filter
		}),
		RTCPFilter(func(pkt []rtcp.Packet) bool {
			return filter
		}),
	)
	assert.NoError(t, err)

	assert.Zero(t, buf.Len())

	defer func() {
		assert.NoError(t, i.Close())
	}()

	rtpOut, rtpWriter := rtpio.RTPPipe()
	rtcpReader, rtcpIn := rtpio.RTCPPipe()

	rtpIn := i.Transform(rtpWriter, nil, rtcpReader)

	go func() {
		_, err2 := rtcpIn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
			SenderSSRC: 123,
			MediaSSRC:  456,
		}})
		assert.NoError(t, err2)
	}()

	go func() {
		_, err2 := rtpIn.WriteRTP(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: uint16(0),
		}})
		assert.NoError(t, err2)
	}()

	p := &rtp.Packet{}
	_, err = rtpOut.ReadRTP(p)
	assert.NoError(t, err)
	assert.Equal(t, uint16(0), p.Header.SequenceNumber)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = i.Close()
	assert.NoError(t, err)

	if !filter {
		// Every packet should have been filtered out – nothing should be written.
		assert.Zero(t, buf.Len())
	} else {
		// Only filtered packets should be written.
		assert.NotZero(t, buf.Len())
	}
}

func TestSenderFilterEverythingOut(t *testing.T) {
	testSenderFilter(t, false)
}

func TestSenderFilterNothing(t *testing.T) {
	testSenderFilter(t, true)
}
