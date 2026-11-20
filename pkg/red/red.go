// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package red provides interceptors for RFC 2198 RED packets.
package red

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	followBit           = 0x80
	primaryHeaderSize   = 1
	redundantHeaderSize = 4
	maxPayloadType      = 0x7f
	maxTimestampOffset  = 0x3fff
	maxBlockLength      = 0x03ff
	maxRedundantBlocks  = 32
)

var (
	errMalformedPayload          = errors.New("malformed RED payload")
	errTooManyBlocks             = errors.New("too many RED blocks")
	errPayloadTypeOutOfRange     = errors.New("RED payload type exceeds 7 bits")
	errTimestampOffsetOutOfRange = errors.New("RED timestamp offset exceeds 14 bits")
	errBlockLengthOutOfRange     = errors.New("RED block payload exceeds 10-bit length")
)

// Block is a media block carried in a RED payload. TimestampOffset is used only
// for redundant blocks; it is zero for the primary block.
//
// Payload returned by Unmarshal references the input buffer.
type Block struct {
	PayloadType     uint8
	TimestampOffset uint16
	Payload         []byte
}

// Payload contains the redundant blocks followed by the primary media block
// carried in an RFC 2198 payload. RedundantBlocks preserve wire order, which need
// not be chronological; TimestampOffset identifies each block's media time.
type Payload struct {
	RedundantBlocks []Block
	PrimaryBlock    Block
}

// Marshal serializes a RED payload.
func (p Payload) Marshal() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	raw := make([]byte, p.MarshalSize())
	headerOffset := 0
	payloadOffset := len(p.RedundantBlocks)*redundantHeaderSize + primaryHeaderSize

	for _, block := range p.RedundantBlocks {
		raw[headerOffset] = followBit | block.PayloadType
		field := uint32(block.TimestampOffset)<<10 | uint32(len(block.Payload)) //nolint:gosec // Length is validated as 10 bits.
		raw[headerOffset+1] = byte((field >> 16) & 0xff)
		raw[headerOffset+2] = byte((field >> 8) & 0xff)
		raw[headerOffset+3] = byte(field & 0xff)
		headerOffset += redundantHeaderSize

		payloadOffset += copy(raw[payloadOffset:], block.Payload)
	}

	raw[headerOffset] = p.PrimaryBlock.PayloadType
	copy(raw[payloadOffset:], p.PrimaryBlock.Payload)

	return raw, nil
}

// MarshalSize returns the number of bytes needed to serialize the payload.
func (p Payload) MarshalSize() int {
	size := len(p.RedundantBlocks)*redundantHeaderSize + primaryHeaderSize + len(p.PrimaryBlock.Payload)
	for _, block := range p.RedundantBlocks {
		size += len(block.Payload)
	}

	return size
}

// Unmarshal parses an RFC 2198 payload. Parsed block payloads reference raw and
// remain valid only while raw remains unchanged.
func (p *Payload) Unmarshal(raw []byte) error {
	*p = Payload{}
	if len(raw) == 0 {
		return errMalformedPayload
	}

	primaryOffset := 0
	for primaryOffset < len(raw) && raw[primaryOffset]&followBit != 0 {
		if primaryOffset == maxRedundantBlocks*redundantHeaderSize {
			return errTooManyBlocks
		}
		primaryOffset += redundantHeaderSize
	}
	if primaryOffset >= len(raw) {
		return fmt.Errorf("%w: truncated block headers", errMalformedPayload)
	}

	blocks := make([]Block, primaryOffset/redundantHeaderSize)
	payloadOffset := primaryOffset + primaryHeaderSize
	for i := range blocks {
		header := binary.BigEndian.Uint32(raw[i*redundantHeaderSize:])
		length := int(header & maxBlockLength)
		if len(raw)-payloadOffset < length {
			return fmt.Errorf("%w: redundant block length exceeds payload", errMalformedPayload)
		}
		blocks[i] = Block{
			PayloadType:     uint8((header >> 24) & maxPayloadType),
			TimestampOffset: uint16((header >> 10) & maxTimestampOffset),
			Payload:         raw[payloadOffset : payloadOffset+length],
		}
		payloadOffset += length
	}

	p.RedundantBlocks = blocks
	p.PrimaryBlock = Block{PayloadType: raw[primaryOffset], Payload: raw[payloadOffset:]}

	return nil
}

func (p Payload) validate() error {
	if len(p.RedundantBlocks) > maxRedundantBlocks {
		return fmt.Errorf("%w: got %d, maximum is %d", errTooManyBlocks, len(p.RedundantBlocks), maxRedundantBlocks)
	}
	if p.PrimaryBlock.PayloadType > maxPayloadType {
		return fmt.Errorf("%w: %d", errPayloadTypeOutOfRange, p.PrimaryBlock.PayloadType)
	}

	for _, block := range p.RedundantBlocks {
		if block.PayloadType > maxPayloadType {
			return fmt.Errorf("%w: %d", errPayloadTypeOutOfRange, block.PayloadType)
		}
		if block.TimestampOffset > maxTimestampOffset {
			return fmt.Errorf("%w: %d", errTimestampOffsetOutOfRange, block.TimestampOffset)
		}
		if len(block.Payload) > maxBlockLength {
			return fmt.Errorf("%w: got %d bytes, maximum is %d", errBlockLengthOutOfRange, len(block.Payload), maxBlockLength)
		}
	}

	return nil
}
