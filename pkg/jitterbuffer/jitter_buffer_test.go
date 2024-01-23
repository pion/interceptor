// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"math"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestJitterBuffer(t *testing.T) {
	assert := assert.New(t)
	t.Run("Appends packets in order", func(t *testing.T) {
		jb := New()
		assert.Equal(jb.lastSequence, uint16(0))
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

		assert.Equal(jb.lastSequence, uint16(5002))

		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5012, Timestamp: 512}, Payload: []byte{0x02}})

		assert.Equal(jb.lastSequence, uint16(5012))
		assert.Equal(jb.stats.outOfOrderCount, uint32(1))
		assert.Equal(jb.packets.Length(), uint16(4))
		assert.Equal(jb.lastSequence, uint16(5012))
	})

	t.Run("Appends packets and begins playout", func(t *testing.T) {
		jb := New()
		for i := 0; i < 100; i++ {
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(5012 + i), Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		assert.Equal(jb.packets.Length(), uint16(100))
		assert.Equal(jb.state, Emitting)
		assert.Equal(jb.playoutHead, uint16(5012))
		head, err := jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(5012))
		assert.Equal(err, nil)
	})
	t.Run("Wraps playout correctly", func(t *testing.T) {
		jb := New()
		for i := 0; i < 100; i++ {
			sqnum := uint16((math.MaxUint16 - 32 + i) % math.MaxUint16)
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		assert.Equal(jb.packets.Length(), uint16(100))
		assert.Equal(jb.state, Emitting)
		assert.Equal(jb.playoutHead, uint16(math.MaxUint16-32))
		head, err := jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
		assert.Equal(err, nil)
		for i := 0; i < 100; i++ {
			head, err := jb.Pop()
			if i < 99 {
				assert.Equal(head.SequenceNumber, uint16((math.MaxUint16-31+i)%math.MaxUint16))
				assert.Equal(err, nil)
			} else {
				assert.Equal(head, (*rtp.Packet)(nil))
			}
		}
	})
	t.Run("Pops at timestamp correctly", func(t *testing.T) {
		jb := New()
		for i := 0; i < 100; i++ {
			sqnum := uint16((math.MaxUint16 - 32 + i) % math.MaxUint16)
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		assert.Equal(jb.packets.Length(), uint16(100))
		assert.Equal(jb.state, Emitting)
		head, err := jb.PopAtTimestamp(uint32(513))
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32+1))
		assert.Equal(err, nil)
		head, err = jb.PopAtTimestamp(uint32(513))
		assert.Equal(head, (*rtp.Packet)(nil))
		assert.NotEqual(err, nil)

		head, err = jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
		assert.Equal(err, nil)
	})
	t.Run("Can peek at a packet", func(t *testing.T) {
		jb := New()
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})
		pkt, err := jb.Peek(false)
		assert.Equal(pkt.SequenceNumber, uint16(5002))
		assert.Equal(err, nil)
		for i := 0; i < 100; i++ {
			sqnum := uint16((math.MaxUint16 - 32 + i) % math.MaxUint16)
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		pkt, err = jb.Peek(true)
		assert.Equal(pkt.SequenceNumber, uint16(5000))
		assert.Equal(err, nil)
	})
	t.Run("Pops at timestamp with multiple packets", func(t *testing.T) {
		jb := New()
		for i := 0; i < 50; i++ {
			sqnum := uint16((math.MaxUint16 - 32 + i) % math.MaxUint16)
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1019, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1020, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		assert.Equal(jb.packets.Length(), uint16(52))
		assert.Equal(jb.state, Emitting)
		head, err := jb.PopAtTimestamp(uint32(9000))
		assert.Equal(head.SequenceNumber, uint16(1019))
		assert.Equal(err, nil)
		head, err = jb.PopAtTimestamp(uint32(9000))
		assert.Equal(head.SequenceNumber, uint16(1020))
		assert.Equal(err, nil)

		head, err = jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
		assert.Equal(err, nil)
	})
}
