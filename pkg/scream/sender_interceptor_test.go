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

func TestSenderInterceptor(t *testing.T) {
	t.Run("sends RTP", func(t *testing.T) {
		i, err := NewSenderInterceptor()
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
	})

	t.Run("doesn't crash on unsupported stream", func(t *testing.T) {
		i, err := NewSenderInterceptor()
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{SSRC: 4294967295}, i)
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

		_, err = i.GetTargetBitrate(0)
		assert.Error(t, err)

		stream.ReceiveRTCP([]rtcp.Packet{
			&rtcp.RawPacket{128, 205, 0, 9, 0, 0, 0, 0, 255, 255, 255, 255, 255, 255, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255, 255, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0},
		})

		time.Sleep(1 * time.Second)

		select {
		case p := <-stream.WrittenRTP():
			t.Errorf("no more rtp packets expected, found sequence number: %v", p.SequenceNumber)
		case <-time.After(10 * time.Millisecond):
		}
	})
}

func Test_extractSSRCs(t *testing.T) {
	header := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	timestamp := []byte{0, 0, 0, 0}
	tests := []struct {
		name   string
		packet []byte
		want   []uint32
	}{
		{
			name:   "empty",
			packet: append(append(header, []byte{}...), timestamp...),
		},
		{
			name:   "one ssrc",
			packet: append(append(header, []byte{255, 255, 255, 255, 0, 1, 0, 2, 0, 0, 0, 0}...), timestamp...),
			want:   []uint32{4294967295},
		},
		{
			name:   "two ssrcs",
			packet: append(append(header, []byte{255, 255, 255, 255, 0, 1, 0, 1, 0, 0, 0, 0, 255, 255, 255, 254, 0, 1, 0, 1, 0, 0, 0, 0}...), timestamp...),
			want:   []uint32{4294967295, 4294967294},
		},
		{
			name:   "duplicate ssrcs",
			packet: append(append(header, []byte{255, 255, 255, 255, 0, 1, 0, 1, 0, 0, 0, 0, 255, 255, 255, 255, 0, 1, 0, 1, 0, 0, 0, 0}...), timestamp...),
			want:   []uint32{4294967295},
		},
		{
			name:   "overflow seq num",
			packet: append(append(header, []byte{255, 255, 255, 255, 255, 255, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255, 254, 0, 1, 0, 1, 0, 0, 0, 0}...), timestamp...),
			want:   []uint32{4294967295, 4294967294},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSSRCs(tt.packet)
			assert.Equal(t, tt.want, got)
		})
	}
}
