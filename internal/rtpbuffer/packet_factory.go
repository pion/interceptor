// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpbuffer

import (
	"encoding/binary"
	"io"
	"sync"

	"github.com/pion/rtp"
)

// PacketFactory allows custom logic around the handle of RTP Packets before they added to the RTPBuffer.
// The NoOpPacketFactory doesn't copy packets, while the RetainablePacket will take a copy before adding
type PacketFactory interface {
	NewPacket(header *rtp.Header, payload []byte, rtxSsrc uint32, rtxPayloadType uint8) (*RetainablePacket, error)
}

// PacketFactoryCopy is PacketFactory that takes a copy of packets when added to the RTPBuffer
type PacketFactoryCopy struct {
	headerPool   *sync.Pool
	payloadPool  *sync.Pool
	rtxSequencer rtp.Sequencer
}

// NewPacketFactoryCopy constructs a PacketFactory that takes a copy of packets when added to the RTPBuffer
func NewPacketFactoryCopy() *PacketFactoryCopy {
	return &PacketFactoryCopy{
		headerPool: &sync.Pool{
			New: func() interface{} {
				return &rtp.Header{}
			},
		},
		payloadPool: &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, maxPayloadLen)
				return &buf
			},
		},
		rtxSequencer: rtp.NewRandomSequencer(),
	}
}

const maxPayloadLen = 1460

// NewPacket constructs a new RetainablePacket that can be added to the RTPBuffer
func (m *PacketFactoryCopy) NewPacket(header *rtp.Header, payload []byte, rtxSsrc uint32, rtxPayloadType uint8) (*RetainablePacket, error) {
	if len(payload) > maxPayloadLen {
		return nil, io.ErrShortBuffer
	}

	p := &RetainablePacket{
		onRelease:      m.releasePacket,
		sequenceNumber: header.SequenceNumber,
		// new packets have retain count of 1
		count: 1,
	}

	var ok bool
	p.header, ok = m.headerPool.Get().(*rtp.Header)
	if !ok {
		return nil, errFailedToCastHeaderPool
	}

	*p.header = header.Clone()

	if payload != nil {
		p.buffer, ok = m.payloadPool.Get().(*[]byte)
		if !ok {
			return nil, errFailedToCastPayloadPool
		}

		size := copy(*p.buffer, payload)
		p.payload = (*p.buffer)[:size]
	}

	if rtxSsrc != 0 && rtxPayloadType != 0 {
		// Store the original sequence number and rewrite the sequence number.
		originalSequenceNumber := p.header.SequenceNumber
		p.header.SequenceNumber = m.rtxSequencer.NextSequenceNumber()

		// Rewrite the SSRC.
		p.header.SSRC = rtxSsrc
		// Rewrite the payload type.
		p.header.PayloadType = rtxPayloadType

		// Remove padding if present.
		paddingLength := 0
		if p.header.Padding && p.payload != nil && len(p.payload) > 0 {
			paddingLength = int(p.payload[len(p.payload)-1])
			p.header.Padding = false
		}

		// Write the original sequence number at the beginning of the payload.
		payload := make([]byte, 2)
		binary.BigEndian.PutUint16(payload, originalSequenceNumber)
		p.payload = append(payload, p.payload[:len(p.payload)-paddingLength]...)
	}

	return p, nil
}

func (m *PacketFactoryCopy) releasePacket(header *rtp.Header, payload *[]byte) {
	m.headerPool.Put(header)
	if payload != nil {
		m.payloadPool.Put(payload)
	}
}

// PacketFactoryNoOp is a PacketFactory implementation that doesn't copy packets
type PacketFactoryNoOp struct{}

// NewPacket constructs a new RetainablePacket that can be added to the RTPBuffer
func (f *PacketFactoryNoOp) NewPacket(header *rtp.Header, payload []byte, _ uint32, _ uint8) (*RetainablePacket, error) {
	return &RetainablePacket{
		onRelease:      f.releasePacket,
		count:          1,
		header:         header,
		payload:        payload,
		sequenceNumber: header.SequenceNumber,
	}, nil
}

func (f *PacketFactoryNoOp) releasePacket(_ *rtp.Header, _ *[]byte) {
	// no-op
}

// NewRetainablePacketFromRTPPacket creates a RetainablePacket that embeds a RTP Packet directly
func NewRetainablePacketFromRTPPacket(pkt *rtp.Packet) *RetainablePacket {
	return &RetainablePacket{
		onRelease:      func(*rtp.Header, *[]byte) {},
		count:          1,
		packet:         pkt,
		header:         &pkt.Header,
		sequenceNumber: pkt.Header.SequenceNumber,
	}
}
