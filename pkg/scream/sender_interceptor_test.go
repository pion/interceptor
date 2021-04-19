//+build scream

package scream

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestSenderInterceptor(t *testing.T) {
	i, err := NewSenderInterceptor()
	assert.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{
		RTCPFeedback: []interceptor.RTCPFeedback{{
			Type:      "ack",
			Parameter: "ccfb",
		}},
	}, i)
	defer func() {
		assert.NoError(t, stream.Close())
	}()

	for _, seqNum := range []uint16{10, 11, 12, 13, 14, 15} {
		assert.NoError(t, stream.WriteRTP(&rtp.Packet{Header: rtp.Header{SequenceNumber: seqNum}}))

		select {
		case p := <-stream.WrittenRTP():
			assert.Equal(t, seqNum, p.SequenceNumber)
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
