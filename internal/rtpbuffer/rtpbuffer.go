// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package rtpbuffer provides a buffer for storing RTP packets
package rtpbuffer

import (
	"github.com/pion/interceptor/internal/validation"
)

const (
	// Uint16SizeHalf is half of a math.Uint16.
	Uint16SizeHalf = 1 << 15

	maxPayloadLen = 1460
)

// RTPBuffer stores RTP packets and allows custom logic
// around the lifetime of them via the PacketFactory.
type RTPBuffer struct {
	packets      []*RetainablePacket
	size         uint16
	highestAdded uint16
	started      bool
}

// NewRTPBuffer constructs a new RTPBuffer.
func NewRTPBuffer(size uint16) (*RTPBuffer, error) {
	if err := validation.IsPowerOfTwo(size); err != nil {
		return nil, err
	}

	return &RTPBuffer{
		packets: make([]*RetainablePacket, size),
		size:    size,
	}, nil
}

// Add places the RetainablePacket in the RTPBuffer, taking ownership of the
// caller's reference. A packet for a sequence number that is already buffered,
// or that is too old to fit the window, is released instead of stored.
func (r *RTPBuffer) Add(packet *RetainablePacket) {
	seq := packet.sequenceNumber
	if !r.started {
		r.packets[seq%r.size] = packet
		r.highestAdded = seq
		r.started = true

		return
	}

	diff := seq - r.highestAdded
	switch {
	case diff == 0:
	case diff < Uint16SizeHalf:
		for i := r.highestAdded + 1; i != seq; i++ {
			idx := i % r.size
			prevPacket := r.packets[idx]
			if prevPacket != nil {
				prevPacket.Release()
			}
			r.packets[idx] = nil
		}
		r.highestAdded = seq
	case r.highestAdded-seq >= r.size:
		// Get could never return it, and its slot belongs to a newer packet.
		packet.Release()

		return
	}

	idx := seq % r.size
	prevPacket := r.packets[idx]
	if prevPacket != nil {
		if prevPacket.sequenceNumber == seq {
			packet.Release()

			return
		}
		prevPacket.Release()
	}
	r.packets[idx] = packet
}

// Clear releases all retained packets in the buffer.
func (r *RTPBuffer) Clear() {
	for i, pkt := range r.packets {
		if pkt != nil {
			pkt.Release()
			r.packets[i] = nil
		}
	}
	r.started = false
}

// Get returns the RetainablePacket for the requested sequence number.
func (r *RTPBuffer) Get(seq uint16) *RetainablePacket {
	diff := r.highestAdded - seq
	if diff >= Uint16SizeHalf {
		return nil
	}

	if diff >= r.size {
		return nil
	}

	pkt := r.packets[seq%r.size]
	if pkt != nil {
		if pkt.sequenceNumber != seq {
			return nil
		}
		// already released
		if err := pkt.Retain(); err != nil {
			return nil
		}
	}

	return pkt
}

// Peek returns the RetainablePacket for the requested sequence number without
// retaining it. The result is only valid until the packet is next released, so
// callers must not use it concurrently with Add, Remove or Clear.
func (r *RTPBuffer) Peek(seq uint16) *RetainablePacket {
	idx, ok := r.index(seq)
	if !ok {
		return nil
	}

	return r.packets[idx]
}

// Remove takes the RetainablePacket for the requested sequence number out of
// the buffer, transferring the buffer's reference to the caller.
func (r *RTPBuffer) Remove(seq uint16) *RetainablePacket {
	idx, ok := r.index(seq)
	if !ok {
		return nil
	}
	pkt := r.packets[idx]
	r.packets[idx] = nil

	return pkt
}

// index returns the slot holding seq, or false if seq is not buffered.
func (r *RTPBuffer) index(seq uint16) (uint16, bool) {
	diff := r.highestAdded - seq
	if diff >= Uint16SizeHalf || diff >= r.size {
		return 0, false
	}
	idx := seq % r.size
	pkt := r.packets[idx]
	if pkt == nil || pkt.sequenceNumber != seq {
		return 0, false
	}

	return idx, true
}

func (r *RTPBuffer) Started() bool { return r.started }
