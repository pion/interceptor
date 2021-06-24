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

	i, err := NewSenderInterceptor(
		SenderWriter(&buf),
		SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
		SenderRTPFilter(func(pkt *rtp.Packet) bool {
			return false
		}),
		SenderRTCPFilter(func(pkt *rtcp.Packet) bool {
			return false
		}),
	)
	assert.Nil(t, err)

	assert.Zero(t, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, i)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	assert.Nil(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})
	assert.Nil(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	// Every packet should have been filtered out – nothing should be written.
	assert.Zero(t, buf.Len())
}

func TestSenderFilterNothing(t *testing.T) {
	buf := bytes.Buffer{}

	i, err := NewSenderInterceptor(
		SenderWriter(&buf),
		SenderLog(logging.NewDefaultLoggerFactory().NewLogger("test")),
		SenderRTPFilter(func(pkt *rtp.Packet) bool {
			return true
		}),
		SenderRTCPFilter(func(pkt *rtcp.Packet) bool {
			return true
		}),
	)
	assert.Nil(t, err)

	assert.EqualValues(t, 0, buf.Len())

	stream := test.NewMockStream(&interceptor.StreamInfo{
		SSRC:      123456,
		ClockRate: 90000,
	}, i)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	err = stream.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 123,
		MediaSSRC:  456,
	}})
	assert.Nil(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})
	assert.Nil(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	assert.NotZero(t, buf.Len())
}
