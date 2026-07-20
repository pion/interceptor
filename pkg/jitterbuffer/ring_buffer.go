// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// RingBuffer is a classic Ring Buffer.
package jitterbuffer

import (
	"github.com/pion/interceptor/internal/rtpbuffer"
)

type RingBuffer struct {
	buffer              []*rtpbuffer.RetainablePacket
	read, write, length uint16
}

func NewRingBuffer(size uint16) *RingBuffer {
	return &RingBuffer{buffer: make([]*rtpbuffer.RetainablePacket, size)}
}

func (r *RingBuffer) Push(rPacket *rtpbuffer.RetainablePacket) bool {
	if r.Full() {
		return false
	}
	r.buffer[r.write] = rPacket
	r.write = (r.write + 1) % uint16(len(r.buffer))
	r.length++
	return true
}

func (r *RingBuffer) Pop() *rtpbuffer.RetainablePacket {
	if r.Empty() {
		return nil
	}

	rPacket := r.buffer[r.read]
	r.read = (r.read + 1) % uint16(len(r.buffer))
	r.length--
	return rPacket
}

func (r *RingBuffer) Peek() *rtpbuffer.RetainablePacket {
	if r.Empty() {
		return nil
	}

	rPacket := r.buffer[r.read]
	return rPacket
}

func (r *RingBuffer) PopAt(sequenceNumber uint16) *rtpbuffer.RetainablePacket {
	if r.Empty() {
		return nil
	}

	extra := sequenceNumber - r.Peek().Header().SequenceNumber
	if extra >= r.length {
		return nil
	}

	for range extra {
		r.Pop().Release()
	}

	return r.Pop()
}

func (r *RingBuffer) PopAtTimestamp(timestamp uint32) *rtpbuffer.RetainablePacket {
	if r.Empty() {
		return nil
	}

	var extra uint16
	found := false
	for i := range r.length {
		rPacket := r.buffer[(r.read+i)%uint16(len(r.buffer))]
		if timestamp == rPacket.Header().Timestamp {
			extra = i
			found = true
			break
		}
	}

	if !found {
		return nil
	}

	for range extra {
		r.Pop().Release()
	}

	return r.Pop()
}

func (r *RingBuffer) Clear() {
	for i := range r.length {
		idx := (r.read + i) % uint16(len(r.buffer))
		if pkt := r.buffer[idx]; pkt != nil {
			pkt.Release()
			r.buffer[idx] = nil
		}
	}
	r.read = 0
	r.write = 0
	r.length = 0
}

func (r *RingBuffer) Full() bool     { return r.length == uint16(len(r.buffer)) }
func (r *RingBuffer) Empty() bool    { return r.length == 0 }
func (r *RingBuffer) Length() uint16 { return r.length }
