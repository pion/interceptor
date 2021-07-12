//+build scream

package scream

import (
	"testing"
	"time"

	"github.com/pion/rtcp"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtp"

	"github.com/stretchr/testify/assert"
)

func TestReceiverInterceptor(t *testing.T) {
	t.Run("sends-feedback", func(t *testing.T) {
		const interval = time.Millisecond * 10

		i, err := NewReceiverInterceptor()
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{
			SSRC: 0,
			RTCPFeedback: []interceptor.RTCPFeedback{{
				Type:      "ack",
				Parameter: "ccfb",
			}},
		}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		for _, seqNum := range []uint16{10, 11, 12, 13} {
			stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})

			select {
			case r := <-stream.ReadRTP():
				assert.NoError(t, r.Err)
				assert.Equal(t, seqNum, r.Packet.SequenceNumber)
			case <-time.After(10 * time.Millisecond):
				t.Fatal("receiver rtp packet not found")
			}
		}

		select {
		case pkts := <-stream.WrittenRTCP():
			assert.Equal(t, 1, len(pkts), "single packet RTCP Compound Packet expected")

			p, ok := pkts[0].(*rtcp.RawPacket)
			assert.True(t, ok, "RawPacket rtcp packet expected, found: %T", pkts[0])

			assert.Equal(t, rtcp.PacketType(205), p.Header().Type)
		case <-time.After(2 * interval):
			t.Fatal("written rtcp packet not found")
		}
	})

	t.Run("doesn't crash on unsupported stream", func(t *testing.T) {
		i, err := NewReceiverInterceptor()
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{SSRC: 0}, i)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		for _, seqNum := range []uint16{10, 11, 12, 13} {
			stream.ReceiveRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}})

			select {
			case r := <-stream.ReadRTP():
				assert.NoError(t, r.Err)
				assert.Equal(t, seqNum, r.Packet.SequenceNumber)
			case <-time.After(10 * time.Millisecond):
				t.Fatal("receiver rtp packet not found")
			}
		}
	})
}
