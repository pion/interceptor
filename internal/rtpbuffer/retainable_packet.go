// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpbuffer

import (
	"sync"

	"github.com/pion/rtp"
)

// RetainablePacket is a referenced counted RTP packet.
type RetainablePacket struct {
	onRelease func(*rtp.Header, *[]byte)

	countMu sync.Mutex
	count   int

	header  *rtp.Header
	buffer  *[]byte
	payload []byte

	sequenceNumber uint16

	// rtx is set when the header and payload were rewritten for RFC 4588 retransmission.
	rtx bool
	// parent is the stored packet a retransmission copy was made from, see RTPBuffer.Get.
	parent *RetainablePacket
}

// rtxPacket is a retransmission of a stored RTX packet. It shares the payload of the
// stored packet, but has its own header so it can carry its own sequence number.
type rtxPacket struct {
	RetainablePacket
	rtxHeader rtp.Header
}

// newRTXCopy returns a retransmission of p with the given sequence number.
// p must be retained, the returned packet releases it when it is released.
func (p *RetainablePacket) newRTXCopy(sequenceNumber uint16) *RetainablePacket {
	pkt := &rtxPacket{rtxHeader: *p.header}
	pkt.rtxHeader.SequenceNumber = sequenceNumber
	pkt.header = &pkt.rtxHeader
	pkt.payload = p.payload
	pkt.sequenceNumber = p.sequenceNumber
	pkt.count = 1
	pkt.parent = p

	return &pkt.RetainablePacket
}

// Header returns the RTP Header of the RetainablePacket.
func (p *RetainablePacket) Header() *rtp.Header {
	return p.header
}

// Payload returns the RTP Payload of the RetainablePacket.
func (p *RetainablePacket) Payload() []byte {
	return p.payload
}

// Retain increases the reference count of the RetainablePacket.
func (p *RetainablePacket) Retain() error {
	p.countMu.Lock()
	defer p.countMu.Unlock()
	if p.count == 0 {
		// already released
		return errPacketReleased
	}
	p.count++

	return nil
}

// Release decreases the reference count of the RetainablePacket and frees if needed.
func (p *RetainablePacket) Release() {
	p.countMu.Lock()
	defer p.countMu.Unlock()
	p.count--

	if p.count == 0 {
		if p.parent != nil {
			p.parent.Release()
			p.parent = nil
			p.header = nil
			p.payload = nil

			return
		}

		// release back to pool
		p.onRelease(p.header, p.buffer)
		p.header = nil
		p.buffer = nil
		p.payload = nil
	}
}
