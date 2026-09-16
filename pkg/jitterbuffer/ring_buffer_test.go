// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestRingBuffer(t *testing.T) {
	assert := assert.New(t)

	t.Run("Appends packets in order", func(t *testing.T) {
		q := NewRingBuffer(16)
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		assert.True(q.Push(buildRetainablePacket(t, pkt)))
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 500}, Payload: []byte{0x02}}
		assert.True(q.Push(buildRetainablePacket(t, pkt2)))
		assert.Equal(uint16(5000), q.Peek().Header().SequenceNumber)
		assert.Equal(uint16(2), q.Length())
		assert.Equal(uint16(5000), q.Pop().Header().SequenceNumber)
		assert.Equal(uint16(5001), q.Peek().Header().SequenceNumber)
	})

	t.Run("Appends many in order", func(t *testing.T) {
		queue := NewRingBuffer(128)
		for i := range 100 {
			assert.True(queue.Push(buildRetainablePacket(t, &rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(5012 + i),
					Timestamp:      uint32(512 + i),
				},
				Payload: []byte{0x02},
			})))
		}
		assert.Equal(uint16(100), queue.Length())
		prev := queue.Peek().Header().SequenceNumber
		assert.Equal(uint16(5012), prev)
		for range 99 {
			popped := queue.Pop()
			assert.NotNil(popped)
			assert.Equal(prev, popped.Header().SequenceNumber)
			assert.Equal(prev+1, queue.Peek().Header().SequenceNumber)
			prev = queue.Peek().Header().SequenceNumber
		}
		assert.Equal(uint16(5012+99), queue.Peek().Header().SequenceNumber)
	})

	t.Run("Can remove an element", func(t *testing.T) {
		queue := NewRingBuffer(128)
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		assert.True(queue.Push(buildRetainablePacket(t, pkt)))
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 500}, Payload: []byte{0x02}}
		assert.True(queue.Push(buildRetainablePacket(t, pkt2)))
		for i := range 100 {
			assert.True(queue.Push(buildRetainablePacket(t, &rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(5002 + i),
					Timestamp:      uint32(512 + i),
				},
				Payload: []byte{0x02},
			})))
		}
		popped := queue.Pop()
		assert.Equal(uint16(5000), popped.Header().SequenceNumber)
		_ = queue.Pop()
		nextPop := queue.Pop()
		assert.Equal(uint16(5002), nextPop.Header().SequenceNumber)
	})

	t.Run("Appends at end", func(t *testing.T) {
		queue := NewRingBuffer(128)
		for i := range 100 {
			assert.True(queue.Push(buildRetainablePacket(t, &rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(5012 + i),
					Timestamp:      uint32(512 + i),
				},
				Payload: []byte{0x02},
			})))
		}
		assert.Equal(uint16(100), queue.Length())
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		assert.True(queue.Push(buildRetainablePacket(t, pkt)))
		assert.Equal(uint16(5012), queue.Peek().Header().SequenceNumber)
		assert.Equal(uint16(101), queue.Length())
	})

	t.Run("Can peek", func(t *testing.T) {
		queue := NewRingBuffer(128)
		for i := range 100 {
			assert.True(queue.Push(buildRetainablePacket(t, &rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(5012 + i),
					Timestamp:      uint32(512 + i),
				},
				Payload: []byte{0x02},
			})))
		}
		pkt := queue.Peek()
		assert.NotNil(pkt)
		assert.Equal(uint16(5012), pkt.Header().SequenceNumber)
		assert.Equal(uint16(100), queue.Length())
	})

	t.Run("Updates the length when PopAt* are called", func(t *testing.T) {
		queue := NewRingBuffer(128)
		pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}}
		assert.True(queue.Push(buildRetainablePacket(t, pkt)))
		pkt2 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}}
		assert.True(queue.Push(buildRetainablePacket(t, pkt2)))
		for i := range 100 {
			assert.True(queue.Push(buildRetainablePacket(t, &rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(5002 + i),
					Timestamp:      uint32(502 + i),
				},
				Payload: []byte{0x02},
			})))
		}
		assert.Equal(uint16(102), queue.Length())
		popped := queue.PopAt(uint16(5002))
		assert.NotNil(popped)
		assert.Equal(uint16(5002), popped.Header().SequenceNumber)
		assert.Equal(uint16(99), queue.Length())

		popped = queue.PopAtTimestamp(uint32(504))
		assert.NotNil(popped)
		assert.Equal(uint16(5004), popped.Header().SequenceNumber)
		assert.Equal(uint16(97), queue.Length())
	})
}

func TestRingBuffer_Find(t *testing.T) {
	packets := NewRingBuffer(16)

	assert.True(t, packets.Push(buildRetainablePacket(t, &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 1000,
			Timestamp:      5,
			SSRC:           5,
		},
		Payload: []uint8{0xA},
	})))

	popped := packets.PopAt(1000)
	assert.NotNil(t, popped)

	assert.Nil(t, packets.PopAt(1001))
	assert.Nil(t, packets.Peek())
}

func TestRingBuffer_Clean(t *testing.T) {
	packets := NewRingBuffer(16)
	packets.Clear()
	assert.True(t, packets.Push(buildRetainablePacket(t, &rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 1000,
			Timestamp:      5,
			SSRC:           5,
		},
		Payload: []uint8{0xA},
	})))
	assert.EqualValues(t, 1, packets.Length())
	packets.Clear()
	assert.EqualValues(t, 0, packets.Length())
	assert.True(t, packets.Empty())
}

func TestRingBuffer_Unreference(t *testing.T) {
	packets := NewRingBuffer(128)
	factory := &rtpbuffer.PacketFactoryNoOp{}

	var refs int64
	finalizer := func(*rtp.Packet) {
		atomic.AddInt64(&refs, -1)
	}

	numPkts := 100
	for i := range numPkts {
		atomic.AddInt64(&refs, 1)
		seq := uint16(i)
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: seq,
				Timestamp:      uint32(i + 42),
			},
			Payload: []byte{byte(i)},
		}
		runtime.SetFinalizer(&p, finalizer)
		rPacket, err := factory.NewPacket(&p.Header, p.Payload, 0, 0)
		assert.NoError(t, err)
		assert.True(t, packets.Push(rPacket))
	}
	for i := 0; i < numPkts-1; i++ {
		var popped *rtpbuffer.RetainablePacket
		switch i % 3 {
		case 0:
			popped = packets.Pop()
		case 1:
			popped = packets.PopAt(uint16(i))
		case 2:
			popped = packets.PopAtTimestamp(uint32(i + 42))
		}
		if popped != nil {
			popped.Release()
		}
	}

	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	remainedRefs := atomic.LoadInt64(&refs)
	runtime.KeepAlive(packets)

	// only the last packet should be still referenced
	assert.Equal(t, int64(1), remainedRefs)
}

// Release odd packets and keep even ones, then check if that happened.
func TestRingBuffer_Release_Copy(t *testing.T) {
	packets := NewRingBuffer(128)
	factory := rtpbuffer.NewPacketFactoryCopy()

	retained := make([]*rtpbuffer.RetainablePacket, 0, 100)
	for i := range 100 {
		rPacket, err := factory.NewPacket(&rtp.Header{
			SequenceNumber: uint16(i),
			Timestamp:      uint32(i + 42),
		}, []byte{byte(i)}, 0, 0)
		assert.NoError(t, err)
		assert.True(t, packets.Push(rPacket))
		retained = append(retained, rPacket)
	}

	for i := range 100 {
		popped := packets.Pop()
		assert.NotNil(t, popped)
		if i%2 == 1 {
			popped.Release()
		}
	}

	for i, pkt := range retained {
		if i%2 == 1 {
			assert.Nil(t, pkt.Header())
			assert.Nil(t, pkt.Payload())
		} else {
			assert.NotNil(t, pkt.Header())
			assert.NotNil(t, pkt.Payload())
		}
	}
}

func buildRetainablePacket(t *testing.T, pkt *rtp.Packet) *rtpbuffer.RetainablePacket {
	t.Helper()
	factory := &rtpbuffer.PacketFactoryNoOp{}
	rPacket, err := factory.NewPacket(&pkt.Header, pkt.Payload, 0, 0)
	assert.NoError(t, err)

	return rPacket
}
