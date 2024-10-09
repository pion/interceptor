// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package rtpbuffer provides a buffer for storing RTP packets
package rtpbuffer

import (
	"fmt"
)

const (
	// Uint16SizeHalf is half of a math.Uint16
	Uint16SizeHalf = 1 << 15

	maxPayloadLen = 1460
)

// RTPBuffer stores RTP packets and allows custom logic around the lifetime of them via the PacketFactory
type RTPBuffer struct {
	packets   []*RetainablePacket
	size      uint16
	lastAdded uint16
	started   bool
}

// NewRTPBuffer constructs a new RTPBuffer
func NewRTPBuffer(size uint16) (*RTPBuffer, error) {
	allowedSizes := make([]uint16, 0)
	correctSize := false
	for i := 0; i < 16; i++ {
		if size == 1<<i {
			correctSize = true
			break
		}
		allowedSizes = append(allowedSizes, 1<<i)
	}

	if !correctSize {
		return nil, fmt.Errorf("%w: %d is not a valid size, allowed sizes: %v", ErrInvalidSize, size, allowedSizes)
	}

	return &RTPBuffer{
		packets: make([]*RetainablePacket, size),
		size:    size,
	}, nil
}

// Add places the RetainablePacket in the RTPBuffer
func (r *RTPBuffer) Add(packet *RetainablePacket) {
	seq := packet.sequenceNumber
	if !r.started {
		r.packets[seq%r.size] = packet
		r.lastAdded = seq
		r.started = true
		return
	}

	diff := seq - r.lastAdded
	if diff == 0 {
		return
	} else if diff < Uint16SizeHalf {
		for i := r.lastAdded + 1; i != seq; i++ {
			idx := i % r.size
			prevPacket := r.packets[idx]
			if prevPacket != nil {
				prevPacket.Release(false)
			}
			r.packets[idx] = nil
		}
	}

	idx := seq % r.size
	prevPacket := r.packets[idx]
	if prevPacket != nil {
		prevPacket.Release(false)
	}
	r.packets[idx] = packet
	r.lastAdded = seq
}

// Get returns the RetainablePacket for the requested sequence number
func (r *RTPBuffer) Get(seq uint16) *RetainablePacket {
	diff := r.lastAdded - seq
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

// GetTimestamp returns a RetainablePacket for the requested timestamp
func (r *RTPBuffer) GetTimestamp(timestamp uint32) *RetainablePacket {
	for i := range r.packets {
		pkt := r.packets[i]
		if pkt != nil && pkt.Header() != nil && pkt.Header().Timestamp == timestamp {
			if err := pkt.Retain(); err != nil {
				return nil
			}

			return pkt
		}
	}
	return nil
}

// Length returns the count of valid RetainablePackets in the RTPBuffer
func (r *RTPBuffer) Length() (length uint16) {
	for i := range r.packets {
		if r.packets[i] != nil && r.packets[i].getCount() != 0 {
			length++
		}
	}

	return
}

// Clear erases all the packets in the RTPBuffer
func (r *RTPBuffer) Clear() {
	r.lastAdded = 0
	r.started = false
	r.packets = make([]*RetainablePacket, r.size)
}
