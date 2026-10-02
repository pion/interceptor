// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpbuffer

import (
	"sync/atomic"

	"github.com/pion/rtp"
)

// RetainablePacket is a referenced counted RTP packet.
type RetainablePacket struct {
	onRelease func(*rtp.Header, *[]byte)

	count atomic.Int32

	header  *rtp.Header
	buffer  *[]byte
	payload []byte

	// unwrapped caches an *rtp.Packet view so repeated reads of the same
	// buffered packet (e.g. many PeekAtSequence calls) reuse one allocation
	// instead of building a fresh *rtp.Packet each time. Cleared on release.
	unwrapped *rtp.Packet

	sequenceNumber uint16
}

// Header returns the RTP Header of the RetainablePacket.
func (p *RetainablePacket) Header() *rtp.Header {
	return p.header
}

// Payload returns the RTP Payload of the RetainablePacket.
func (p *RetainablePacket) Payload() []byte {
	return p.payload
}

// Unwrapped returns the cached *rtp.Packet view, or nil if none is cached yet.
func (p *RetainablePacket) Unwrapped() *rtp.Packet {
	return p.unwrapped
}

// SetUnwrapped caches an *rtp.Packet view for reuse by later reads.
func (p *RetainablePacket) SetUnwrapped(pkt *rtp.Packet) {
	p.unwrapped = pkt
}

// Retain increases the reference count of the RetainablePacket.
func (p *RetainablePacket) Retain() error {
	for {
		n := p.count.Load()
		if n == 0 {
			// already released
			return errPacketReleased
		}
		if p.count.CompareAndSwap(n, n+1) {
			return nil
		}
	}
}

// Release decreases the reference count of the RetainablePacket and frees if needed.
func (p *RetainablePacket) Release() {
	if p.count.Add(-1) == 0 {
		// release back to pool
		p.onRelease(p.header, p.buffer)
		p.header = nil
		p.buffer = nil
		p.payload = nil
		p.unwrapped = nil
	}
}
