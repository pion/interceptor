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
		RTPFilter(func(pkt *rtp.Packet) bool {
			return false
		}),
		RTCPFilter(func(pkt []rtcp.Packet) bool {
			return false
		}),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

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
	assert.NoError(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})
	assert.NoError(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = i.Close()
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
		RTPFilter(func(pkt *rtp.Packet) bool {
			return true
		}),
		RTCPFilter(func(pkt []rtcp.Packet) bool {
			return true
		}),
	)
	assert.NoError(t, err)

	i, err := factory.NewInterceptor("")
	assert.NoError(t, err)

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
	assert.NoError(t, err)

	err = stream.WriteRTP(&rtp.Packet{Header: rtp.Header{
		SequenceNumber: uint16(0),
	}})
	assert.NoError(t, err)

	// Give time for packets to be handled and stream written to.
	time.Sleep(50 * time.Millisecond)

	err = i.Close()
	assert.NoError(t, err)

	assert.NotZero(t, buf.Len())
}
