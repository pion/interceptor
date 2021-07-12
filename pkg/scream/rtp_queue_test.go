//+build scream

package scream

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pion/rtp"
)

func Test_queue_EnqueueDequeue(t *testing.T) {
	type input struct {
		pkt *rtp.Packet
		ts  uint64
	}
	tests := []struct {
		name string
		data []input
	}{
		{
			name: "dequeue-empty",
		},
		{
			name: "enqueue-dequeue-1",
			data: []input{{pkt: &rtp.Packet{}, ts: 100000}},
		},
		{
			name: "enqueue-dequeue-2",
			data: []input{{pkt: &rtp.Packet{}, ts: 10000000}, {pkt: &rtp.Packet{}, ts: 200000}},
		},
		{
			name: "enqueue-dequeue-3",
			data: []input{{pkt: &rtp.Packet{Payload: []byte{0x01, 0x02, 0x03}}, ts: 120000}, {pkt: &rtp.Packet{Payload: []byte{0x01, 0x02, 0x03}}, ts: 163456}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueue()

			bytes := 0
			for i, pkt := range tt.data {
				frameSize := 12 + len(pkt.pkt.Payload)
				bytes += frameSize

				q.Enqueue(pkt.pkt, pkt.ts)

				assert.Equal(t, i+1, q.SizeOfQueue())
				assert.Equal(t, bytes, q.BytesInQueue())
				assert.Equal(t, frameSize, q.GetSizeOfLastFrame())
				assert.Equal(t, tt.data[0].pkt.SequenceNumber, q.SeqNrOfNextRTP())
				assert.Equal(t, 12+len(tt.data[0].pkt.Payload), q.SizeOfNextRTP())
			}

			for i := range tt.data {
				got := q.Dequeue()

				frameSize := 12 + len(got.Payload)
				bytes -= frameSize

				assert.Equal(t, tt.data[i].pkt, got)
				assert.Equal(t, len(tt.data)-(i+1), q.SizeOfQueue())
				assert.Equal(t, bytes, q.BytesInQueue())
			}

			for _, pkt := range tt.data {
				q.Enqueue(pkt.pkt, pkt.ts)
			}

			q.Clear()

			assert.Equal(t, 0, q.SizeOfQueue())
			assert.Equal(t, 0, q.BytesInQueue())
		})
	}
}

func Test_queue_GetDelay(t *testing.T) {
	tests := []struct {
		name  string
		input uint64
		ts    float32
		want  float32
	}{
		{
			name: "zero",
		},
		{
			name:  "shift-65536-zero-delay",
			input: 65536 << 16,
			ts:    1,
		},
		{
			name:  "shift-1-zero-delay",
			input: 1 << 32,
			ts:    1,
		},
		{
			name:  "shift-2-1-delay",
			input: 2 << 32,
			ts:    3,
			want:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueue()
			q.Enqueue(&rtp.Packet{}, tt.input)

			got := q.GetDelay(tt.ts)
			assert.Equal(t, tt.want, got)
		})
	}
}
