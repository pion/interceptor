// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpbuffer

import (
	"sync"
	"sync/atomic"
	"time"

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
	lastResend     time.Time
	pending        bool

	// rtx is set when the header and payload were rewritten for RFC 4588 retransmission.
	rtx bool
	// parent is the stored packet a retransmission copy was made from, see RTPBuffer.Get.
	parent *RetainablePacket
	// rtxSequencer numbers the RTX stream of a retransmission copy, see AssignRTXSequenceNumber.
	rtxSequencer rtp.Sequencer
}

// rtxPacket is a retransmission of a stored RTX packet. It shares the payload of the
// stored packet, but has its own header so it can carry its own sequence number.
type rtxPacket struct {
	RetainablePacket
	rtxHeader rtp.Header
}

// newRTXCopy returns a retransmission of p numbered by sequencer.
// p must be retained, the returned packet releases it when it is released.
func (p *RetainablePacket) newRTXCopy(sequencer rtp.Sequencer) *RetainablePacket {
	// Clone so that CSRC and extensions are not shared with p or other copies.
	pkt := &rtxPacket{rtxHeader: p.header.Clone()}
	pkt.header = &pkt.rtxHeader
	pkt.payload = p.payload
	pkt.sequenceNumber = p.sequenceNumber
	pkt.count = 1
	pkt.parent = p
	pkt.rtxSequencer = sequencer

	return &pkt.RetainablePacket
}

// AssignRTXSequenceNumber gives a retransmission copy returned by RTPBuffer.Get
// the next sequence number of its RTX stream. Call it once the retransmission
// was admitted, right before writing it, so rejected retries leave no gaps.
// It does nothing for other packets.
func (p *RetainablePacket) AssignRTXSequenceNumber() {
	if p.rtxSequencer == nil {
		return
	}
	p.header.SequenceNumber = p.rtxSequencer.NextSequenceNumber()
	p.rtxSequencer = nil
}

// BeginResend reserves a packet unless it is pending or was recently resent.
// Retransmission copies share this state with the stored packet.
func (p *RetainablePacket) BeginResend(now time.Time, interval time.Duration) bool {
	if p.parent != nil {
		return p.parent.BeginResend(now, interval)
	}
	p.countMu.Lock()
	defer p.countMu.Unlock()
	if p.pending || (!p.lastResend.IsZero() && now.Sub(p.lastResend) < interval) {
		return false
	}
	p.pending = true

	return true
}

// EndResend clears pending and records successful retransmissions.
// Retransmission copies share this state with the stored packet.
func (p *RetainablePacket) EndResend(now time.Time, sent bool) {
	if p.parent != nil {
		p.parent.EndResend(now, sent)

		return
	}
	p.countMu.Lock()
	defer p.countMu.Unlock()
	p.pending = false
	if sent {
		p.lastResend = now
	}
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

// ResendKey identifies retransmission completion in interceptor attributes.
type ResendKey struct{}

// Resend tracks a retransmission through asynchronous writers.
type Resend struct {
	owners   atomic.Int32
	canceled atomic.Bool
	finish   func(bool)
}

// NewResend creates a resend with one owner and a completion callback.
func NewResend(complete func(bool)) *Resend {
	resend := &Resend{finish: complete}
	resend.owners.Store(1)

	return resend
}

// Retain adds an owner before passing the resend to another writer.
func (r *Resend) Retain() {
	if r != nil {
		r.owners.Add(1)
	}
}

// Release completes the resend when its last owner finishes.
func (r *Resend) Release(sent bool) {
	if r != nil && !sent {
		r.canceled.Store(true)
	}
	if r != nil && r.owners.Add(-1) == 0 {
		r.finish(!r.canceled.Load())
	}
}
