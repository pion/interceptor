// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package rtpbuffer provides a buffer for storing RTP packets
package rtpbuffer

import (
	"fmt"
)

// RTPBuffer stores RTP packets and allows custom logic around the lifetime of them via the PacketFactory
type RTPBuffer struct {
	packets   []*RetainablePacket
	size      uint16
	lastAdded uint16
}

// NewRTPBuffer constructs a new RTPBuffer
func NewRTPBuffer(size uint16) (*RTPBuffer, error) {
	allowedSizes := []uint16{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65535}
	correctSize := false
	for _, v := range allowedSizes {
		if v == size {
			correctSize = true
			break
		}
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
	idx := seq % r.size

	if prevPacket := r.packets[idx]; prevPacket != nil {
		prevPacket.Release(false)
	}

	r.packets[idx] = packet
}

// Get returns the RetainablePacket for the requested sequence number
func (r *RTPBuffer) Get(seq uint16) *RetainablePacket {
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
	r.packets = make([]*RetainablePacket, r.size)
	r.lastAdded = 0
}
