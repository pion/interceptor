// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"math"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func safeUint16(i int) uint16 {
	if i < 0 {
		return 0
	}
	if i > math.MaxUint16 {
		return math.MaxUint16
	}

	return uint16(i)
}

func safeUint32(i int) uint32 {
	if i < 0 {
		return 0
	}
	if i > math.MaxInt32 {
		return math.MaxUint32
	}

	return uint32(i)
}

func TestJitterBufferInOrderPackets(t *testing.T) {
	assert := assert.New(t)
	jb := New()
	assert.Equal(jb.lastSequence, uint16(0))

	// Push packets in order
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

	assert.Equal(jb.lastSequence, uint16(5002))

	// Push out of order packet
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5012, Timestamp: 512}, Payload: []byte{0x02}})

	assert.Equal(jb.stats.outOfOrderCount, uint32(1))
	assert.Equal(jb.packets.Length(), uint16(4))
	assert.Equal(jb.lastSequence, uint16(5012))
}

func TestJitterBufferSequenceWrapping(t *testing.T) {
	assert := assert.New(t)
	jb := New(WithMinimumPacketCount(1))
	assert.Equal(jb.lastSequence, uint16(0))

	// Push packet at max sequence
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: math.MaxUint16, Timestamp: 500}, Payload: []byte{0x02}})
	assert.Equal(jb.lastSequence, uint16(math.MaxUint16))

	// Push packet at sequence 0 (wrapping)
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 0, Timestamp: 512}, Payload: []byte{0x02}})

	assert.Equal(jb.packets.Length(), uint16(2))
	assert.Equal(jb.lastSequence, uint16(0))

	// Verify packets are popped in correct order
	head, err := jb.Pop()
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(math.MaxUint16))

	head, err = jb.Pop()
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(0))
}

func TestJitterBufferPlayout(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push 100 packets
	for i := 0; i < 100; i++ {
		jb.Push(
			&rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: safeUint16(5012 + i),
					Timestamp:      safeUint32(512 + i),
				},
				Payload: []byte{0x02},
			},
		)
	}

	assert.Equal(jb.packets.Length(), uint16(100))
	assert.Equal(jb.state, Emitting)
	assert.Equal(jb.playoutHead, uint16(5012))

	head, err := jb.Pop()
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(5012))
}

func TestJitterBufferPlayoutEvents(t *testing.T) {
	assert := assert.New(t)
	jb := New(WithMinimumPacketCount(1))
	events := make([]Event, 0)

	jb.Listen(BeginPlayback, func(event Event, _ *JitterBuffer) {
		events = append(events, event)
	})

	// Push 2 packets
	for i := 0; i < 2; i++ {
		jb.Push(
			&rtp.Packet{
				Header: rtp.Header{
					SequenceNumber: safeUint16(5012 + i),
					Timestamp:      safeUint32(512 + i),
				},
				Payload: []byte{0x02},
			},
		)
	}

	assert.Equal(jb.packets.Length(), uint16(2))
	assert.Equal(jb.state, Emitting)
	assert.Equal(jb.playoutHead, uint16(5012))

	head, err := jb.Pop()
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(5012))
	assert.Equal(1, len(events))
	assert.Equal(Event(BeginPlayback), events[0])
}

func TestJitterBufferPlayoutWrapping(t *testing.T) {
	assert := assert.New(t)
	jb := New(WithMinimumPacketCount(1))

	// Push packets near max sequence
	var i uint16
	for i = 0; i < 100; i++ {
		sqnum := safeUint16(int(math.MaxUint16) - 32 + int(i))
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
				Timestamp:      uint32(512 + i),
			},
			Payload: []byte{0x02},
		})
	}

	assert.Equal(jb.packets.Length(), uint16(100))
	assert.Equal(jb.state, Emitting)
	assert.Equal(jb.playoutHead, uint16(math.MaxUint16-32))

	// Wait for buffer to transition to emitting state
	for jb.state == Buffering {
		time.Sleep(time.Millisecond)
	}

	// Pop packets and verify sequence numbers
	for i := 0; i < 100; i++ {
		expectedSeq := safeUint16(int(math.MaxUint16) - 32 + i)
		head, err := jb.PopAtSequence(expectedSeq)
		assert.NoError(err, "expected seq %d to be found", i)
		assert.NotNil(head)
		assert.Equal(expectedSeq, head.SequenceNumber)
	}
}

func TestJitterBufferPopAtTimestamp(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push packets near max sequence
	for i := 0; i < 100; i++ {
		sqnum := safeUint16(math.MaxUint16 - 32 + i)
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
				Timestamp:      safeUint32(512 + i),
			},
			Payload: []byte{0x02},
		})
	}

	assert.Equal(jb.packets.Length(), uint16(100))
	assert.Equal(jb.state, Emitting)

	// Test pop at specific timestamp
	head, err := jb.PopAtTimestamp(513)
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, safeUint16(math.MaxUint16-32+1))

	// Test pop at same timestamp again (should fail)
	head, err = jb.PopAtTimestamp(513)
	assert.Nil(head)
	assert.Error(err)

	// Test normal pop
	head, err = jb.Pop()
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
}

func TestJitterBufferPeek(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push initial packets
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

	// Test peek at latest
	pkt, err := jb.Peek(false)
	assert.NoError(err)
	assert.Equal(pkt.SequenceNumber, uint16(5002))

	// Push more packets
	for i := 0; i < 100; i++ {
		sqnum := safeUint16(math.MaxUint16 - 32 + i)
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
				Timestamp:      safeUint32(512 + i),
			},
			Payload: []byte{0x02},
		})
	}

	// Test peek at oldest
	pkt, err = jb.Peek(true)
	assert.NoError(err)
	assert.Equal(pkt.SequenceNumber, uint16(5000))
}

func TestJitterBufferInvalidSequence(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push packets near max sequence
	for i := 0; i < 50; i++ {
		sqnum := safeUint16(math.MaxUint16 - 32 + i)
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
				Timestamp:      safeUint32(512 + i),
			},
			Payload: []byte{0x02},
		})
	}

	// Push some additional packets
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1019, Timestamp: 9000}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1020, Timestamp: 9000}, Payload: []byte{0x02}})

	assert.Equal(jb.packets.Length(), uint16(52))
	assert.Equal(jb.state, Emitting)

	// Test pop with invalid sequence
	head, err := jb.PopAtSequence(9000)
	assert.Nil(head)
	assert.Error(err)
}

func TestJitterBufferMultiplePacketsAtTimestamp(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push packets near max sequence
	for i := 0; i < 50; i++ {
		sqnum := safeUint16(math.MaxUint16 - 32 + i)
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
				Timestamp:      safeUint32(512 + i),
			},
			Payload: []byte{0x02},
		})
	}

	// Push packets with same timestamp
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1019, Timestamp: 9000}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1020, Timestamp: 9000}, Payload: []byte{0x02}})

	assert.Equal(jb.packets.Length(), uint16(52))
	assert.Equal(jb.state, Emitting)

	// Test pop at timestamp
	head, err := jb.PopAtTimestamp(9000)
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(1019))

	head, err = jb.PopAtTimestamp(9000)
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(1020))

	// Test normal pop
	head, err = jb.Pop()
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
}

func TestJitterBufferPeekAtSequence(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push packets near max sequence
	for i := 0; i < 50; i++ {
		sqnum := safeUint16(math.MaxUint16 - 32 + i)
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
				Timestamp:      safeUint32(512 + i),
			},
			Payload: []byte{0x02},
		})
	}

	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1019, Timestamp: 9000}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1020, Timestamp: 9000}, Payload: []byte{0x02}})

	assert.Equal(jb.packets.Length(), uint16(52))
	assert.Equal(jb.state, Emitting)

	head, err := jb.PeekAtSequence(1019)
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(1019))

	head, err = jb.PeekAtSequence(1020)
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(1020))

	// Test peek at sequence near max
	head, err = jb.PeekAtSequence(safeUint16(math.MaxUint16 - 32))
	assert.NoError(err)
	assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
}

func TestJitterBufferSetPlayoutHead(t *testing.T) {
	assert := assert.New(t)
	jb := New(WithMinimumPacketCount(1))

	// Push packets 0-9, but skip packet 4
	for i := uint16(0); i < 10; i++ {
		if i == 4 {
			continue
		}
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: i,
				Timestamp:      safeUint32(512 + int(i)),
			},
			Payload: []byte{0x00},
		})
	}

	// First 3 packets should be poppable
	for i := 0; i < 4; i++ {
		pkt, err := jb.Pop()
		assert.NoError(err)
		assert.NotNil(pkt)
	}

	// Next pop should fail due to gap
	pkt, err := jb.Pop()
	assert.ErrorIs(err, ErrNotFound)
	assert.Nil(pkt)
	assert.Equal(jb.PlayoutHead(), uint16(4))

	// Verify PlayoutHead isn't modified by pushing/popping
	jb.Push(&rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 10,
			Timestamp:      522,
		},
		Payload: []byte{0x00},
	})
	pkt, err = jb.Pop()
	assert.ErrorIs(err, ErrNotFound)
	assert.Nil(pkt)
	assert.Equal(jb.PlayoutHead(), uint16(4))

	// Increment PlayoutHead and verify popping works again
	jb.SetPlayoutHead(jb.PlayoutHead() + 1)
	for i := 0; i < 6; i++ {
		pkt, err := jb.Pop()
		assert.NoError(err)
		assert.NotNil(pkt)
	}
}

func TestJitterBufferClear(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Test initial clear
	jb.Clear(false)
	assert.Equal(jb.lastSequence, uint16(0))

	// Push some packets
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
	jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

	assert.Equal(jb.lastSequence, uint16(5002))

	// Clear with reset
	jb.Clear(true)
	assert.Equal(jb.lastSequence, uint16(0))
	assert.Equal(jb.stats.outOfOrderCount, uint32(0))
	assert.Equal(jb.packets.Length(), uint16(0))
}

func TestJitterBuffer(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Test sequence number wrapping
	for i := 0; i < 64; i++ {
		sqnum := safeUint16(math.MaxUint16 - 32 + i)
		pkt := &rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: sqnum,
			},
		}
		jb.Push(pkt)
	}

	// Verify packets are read in order
	for i := 0; i < 64; i++ {
		expectedSeq := safeUint16(math.MaxUint16 - 32 + i)
		pkt, err := jb.PopAtSequence(expectedSeq)
		assert.NoError(err)
		assert.Equal(expectedSeq, pkt.SequenceNumber)
	}
}

func TestJitterBufferLength(t *testing.T) {
	assert := assert.New(t)
	jb := New(WithMinimumPacketCount(1))

	// Push 10 packets
	for i := 0; i < 10; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: safeUint16(1000 + i),
				Timestamp:      safeUint32(500 + i),
			},
			Payload: []byte{0x01},
		})
	}
	assert.Equal(uint16(10), jb.packets.Length(), "JitterBuffer should have 10 packets after push")

	// Wait for buffer to transition to emitting state
	for jb.state == Buffering {
		time.Sleep(time.Millisecond)
	}

	// Pop 3 packets
	for i := 0; i < 3; i++ {
		_, err := jb.Pop()
		assert.NoError(err)
	}
	assert.Equal(uint16(7), jb.packets.Length(), "JitterBuffer should have 7 packets after popping 3")
}

func TestJitterBufferPeekAtSequenceError(t *testing.T) {
	assert := assert.New(t)
	jb := New()

	// Push some packets
	for i := 0; i < 5; i++ {
		jb.Push(&rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: safeUint16(1000 + i),
				Timestamp:      safeUint32(500 + i),
			},
			Payload: []byte{0x01},
		})
	}

	// Try to peek at a sequence number that doesn't exist
	pkt, err := jb.PeekAtSequence(2000)
	assert.Nil(pkt, "PeekAtSequence should return nil for non-existent sequence")
	assert.Error(err, "PeekAtSequence should return error for non-existent sequence")
	assert.ErrorIs(err, ErrNotFound, "Error should be ErrNotFound")
}
