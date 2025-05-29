// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestPriorityQueue(t *testing.T) {
	assert := assert.New(t)

	t.Run("Appends packets in order", func(*testing.T) {
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		q := NewQueue()
		q.Push(pkt, pkt.SequenceNumber)
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}}
		q.Push(pkt2, pkt2.SequenceNumber)
		assert.Equal(q.next.next.val, pkt2)
		assert.Equal(q.next.priority, uint16(5000))
		assert.Equal(q.next.next.priority, uint16(5004))
	})

	t.Run("Appends many in order", func(*testing.T) {
		queue := NewQueue()
		for i := 0; i < 100; i++ {
			//nolint:gosec // G115
			queue.Push(
				&rtp.Packet{
					Header: rtp.Header{
						SequenceNumber: uint16(5012 + i),
						Timestamp:      uint32(512 + i),
					},
					Payload: []byte{0x02},
				},
				uint16(5012+i),
			)
		}
		assert.Equal(uint16(100), queue.Length())
		last := (*node)(nil)
		cur := queue.next
		for cur != nil {
			last = cur
			cur = cur.next
			if cur != nil {
				assert.Equal(cur.priority, last.priority+1)
			}
		}
		assert.Equal(queue.next.priority, uint16(5012))
		assert.Equal(last.priority, uint16(5012+99))
	})

	t.Run("Can remove an element", func(*testing.T) {
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		queue := NewQueue()
		queue.Push(pkt, pkt.SequenceNumber)
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}}
		queue.Push(pkt2, pkt2.SequenceNumber)
		for i := 0; i < 100; i++ {
			//nolint:gosec // G115
			queue.Push(
				&rtp.Packet{
					Header:  rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)},
					Payload: []byte{0x02},
				},
				uint16(5012+i),
			)
		}
		popped, _ := queue.Pop()
		assert.Equal(popped.SequenceNumber, uint16(5000))
		_, _ = queue.Pop()
		nextPop, _ := queue.Pop()
		assert.Equal(nextPop.SequenceNumber, uint16(5012))
	})

	t.Run("Appends in order", func(*testing.T) {
		queue := NewQueue()
		for i := 0; i < 100; i++ {
			queue.Push(
				&rtp.Packet{
					Header: rtp.Header{
						SequenceNumber: uint16(5012 + i), //nolint:gosec // G115
						Timestamp:      uint32(512 + i),  //nolint:gosec // G115
					},
					Payload: []byte{0x02},
				},
				uint16(5012+i), //nolint:gosec // G115
			)
		}
		assert.Equal(uint16(100), queue.Length())
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		queue.Push(pkt, pkt.SequenceNumber)
		assert.Equal(pkt, queue.next.val)
		assert.Equal(uint16(101), queue.Length())
		assert.Equal(queue.next.priority, uint16(5000))
	})

	t.Run("Can find", func(*testing.T) {
		queue := NewQueue()
		for i := 0; i < 100; i++ {
			//nolint:gosec // G115
			queue.Push(
				&rtp.Packet{
					Header: rtp.Header{
						SequenceNumber: uint16(5012 + i),
						Timestamp:      uint32(512 + i),
					},
					Payload: []byte{0x02},
				},
				uint16(5012+i),
			)
		}
		pkt, err := queue.Find(5012)
		assert.Equal(pkt.SequenceNumber, uint16(5012))
		assert.Equal(err, nil)
	})

	t.Run("Updates the length when PopAt* are called", func(*testing.T) {
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		queue := NewQueue()
		queue.Push(pkt, pkt.SequenceNumber)
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}}
		queue.Push(pkt2, pkt2.SequenceNumber)
		for i := 0; i < 100; i++ {
			//nolint:gosec // G115
			queue.Push(
				&rtp.Packet{
					Header: rtp.Header{
						SequenceNumber: uint16(5012 + i),
						Timestamp:      uint32(512 + i),
					},
					Payload: []byte{0x02},
				},
				uint16(5012+i),
			)
		}
		assert.Equal(uint16(102), queue.Length())
		popped, _ := queue.PopAt(uint16(5012))
		assert.Equal(popped.SequenceNumber, uint16(5012))
		assert.Equal(uint16(101), queue.Length())

		popped, err := queue.PopAtTimestamp(uint32(500))
		assert.Equal(popped.SequenceNumber, uint16(5000))
		assert.Equal(uint16(100), queue.Length())
		assert.Equal(err, nil)
	})
}

func TestPriorityQueue_Find(t *testing.T) {
	packets := NewQueue()

	packets.Push(&rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 1000,
			Timestamp:      5,
			SSRC:           5,
		},
		Payload: []uint8{0xA},
	}, 1000)

	_, err := packets.PopAt(1000)
	assert.NoError(t, err)

	_, err = packets.Find(1001)
	assert.Error(t, err)
}

func TestPriorityQueue_Clean(t *testing.T) {
	packets := NewQueue()
	packets.Clear()
	packets.Push(&rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 1000,
			Timestamp:      5,
			SSRC:           5,
		},
		Payload: []uint8{0xA},
	}, 1000)
	assert.EqualValues(t, 1, packets.Length())
	packets.Clear()
}

func TestPriorityQueue_Unreference(t *testing.T) {
	packets := NewQueue()

	var refs int64
	finalizer := func(*rtp.Packet) {
		atomic.AddInt64(&refs, -1)
	}

	numPkts := 100
	for i := 0; i < numPkts; i++ {
		atomic.AddInt64(&refs, 1)
		seq := uint16(i) //nolint:gosec // G115
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: seq,
				Timestamp:      uint32(i + 42), //nolint:gosec // G115
			},
			Payload: []byte{byte(i)},
		}
		runtime.SetFinalizer(&p, finalizer)
		packets.Push(&p, seq)
	}
	for i := 0; i < numPkts-1; i++ {
		switch i % 3 {
		case 0:
			packets.Pop() //nolint
		case 1:
			packets.PopAt(uint16(i)) //nolint
		case 2:
			packets.PopAtTimestamp(uint32(i + 42)) //nolint
		}
	}

	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	remainedRefs := atomic.LoadInt64(&refs)
	runtime.KeepAlive(packets)

	// only the last packet should be still referenced
	assert.Equal(t, int64(1), remainedRefs)
}
