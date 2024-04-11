// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"testing"

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
		q := NewQueue()
		for i := 0; i < 100; i++ {
			q.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)}, Payload: []byte{0x02}}, uint16(5012+i))
		}
		assert.Equal(uint16(100), q.Length())
		last := (*node)(nil)
		cur := q.next
		for cur != nil {
			last = cur
			cur = cur.next
			if cur != nil {
				assert.Equal(cur.priority, last.priority+1)
			}
		}
		assert.Equal(q.next.priority, uint16(5012))
		assert.Equal(last.priority, uint16(5012+99))
	})

	t.Run("Can remove an element", func(*testing.T) {
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		q := NewQueue()
		q.Push(pkt, pkt.SequenceNumber)
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}}
		q.Push(pkt2, pkt2.SequenceNumber)
		for i := 0; i < 100; i++ {
			q.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)}, Payload: []byte{0x02}}, uint16(5012+i))
		}
		popped, _ := q.Pop()
		assert.Equal(popped.SequenceNumber, uint16(5000))
		_, _ = q.Pop()
		nextPop, _ := q.Pop()
		assert.Equal(nextPop.SequenceNumber, uint16(5012))
	})

	t.Run("Appends in order", func(*testing.T) {
		q := NewQueue()
		for i := 0; i < 100; i++ {
			q.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)}, Payload: []byte{0x02}}, uint16(5012+i))
		}
		assert.Equal(uint16(100), q.Length())
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		q.Push(pkt, pkt.SequenceNumber)
		assert.Equal(pkt, q.next.val)
		assert.Equal(uint16(101), q.Length())
		assert.Equal(q.next.priority, uint16(5000))
	})

	t.Run("Can find", func(*testing.T) {
		q := NewQueue()
		for i := 0; i < 100; i++ {
			q.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)}, Payload: []byte{0x02}}, uint16(5012+i))
		}
		pkt, err := q.Find(5012)
		assert.Equal(pkt.SequenceNumber, uint16(5012))
		assert.Equal(err, nil)
	})

	t.Run("Updates the length when PopAt* are called", func(*testing.T) {
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		q := NewQueue()
		q.Push(pkt, pkt.SequenceNumber)
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}}
		q.Push(pkt2, pkt2.SequenceNumber)
		for i := 0; i < 100; i++ {
			q.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)}, Payload: []byte{0x02}}, uint16(5012+i))
		}
		assert.Equal(uint16(102), q.Length())
		popped, _ := q.PopAt(uint16(5012))
		assert.Equal(popped.SequenceNumber, uint16(5012))
		assert.Equal(uint16(101), q.Length())

		popped, err := q.PopAtTimestamp(uint32(500))
		assert.Equal(popped.SequenceNumber, uint16(5000))
		assert.Equal(uint16(100), q.Length())
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
