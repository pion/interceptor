// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package rtpbuffer provides a buffer for storing RTP packets
package rtpbuffer

import (
	"github.com/pion/interceptor/internal/validation"
	"github.com/pion/rtp"
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

	// rtxSequencer numbers the retransmissions of this buffer's RTX stream.
	rtxSequencer rtp.Sequencer
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

// Add places the RetainablePacket in the RTPBuffer.
func (r *RTPBuffer) Add(packet *RetainablePacket) {
	seq := packet.sequenceNumber
	if !r.started {
		r.packets[seq%r.size] = packet
		r.highestAdded = seq
		r.started = true

		return
	}

	diff := seq - r.highestAdded
	if diff == 0 {
		return
	} else if diff < Uint16SizeHalf {
		for i := r.highestAdded + 1; i != seq; i++ {
			idx := i % r.size
			prevPacket := r.packets[idx]
			if prevPacket != nil {
				prevPacket.Release()
			}
			r.packets[idx] = nil
		}
		r.highestAdded = seq
	}

	idx := seq % r.size
	prevPacket := r.packets[idx]
	if prevPacket != nil {
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
//
// For a packet stored for RFC 4588 retransmission, every call is treated as a
// retransmission: it returns a copy with the next sequence number of the RTX
// stream, and leaves the stored packet unchanged.
func (r *RTPBuffer) Get(seq uint16) *RetainablePacket {
	diff := r.highestAdded - seq
	if diff >= Uint16SizeHalf {
		return nil
	}

	if diff >= r.size {
		return nil
	}

	pkt := r.packets[seq%r.size]
	if pkt == nil || pkt.sequenceNumber != seq {
		return nil
	}
	// already released
	if err := pkt.Retain(); err != nil {
		return nil
	}

	if !pkt.rtx {
		return pkt
	}

	if r.rtxSequencer == nil {
		r.rtxSequencer = rtp.NewRandomSequencer()
	}

	return pkt.newRTXCopy(r.rtxSequencer.NextSequenceNumber())
}
