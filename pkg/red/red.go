// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package red implements the RTP payload format for redundant audio data described by RFC 2198.
package red

import (
	"errors"
	"fmt"
	"io"
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
// carried in an RFC 2198 payload. RedundantBlocks are ordered oldest to newest.
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
	n := p.marshalTo(raw)

	return raw[:n], nil
}

// MarshalTo serializes a RED payload into raw.
func (p Payload) MarshalTo(raw []byte) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}

	if len(raw) < p.MarshalSize() {
		return 0, io.ErrShortBuffer
	}

	return p.marshalTo(raw), nil
}

func (p Payload) marshalTo(raw []byte) int {
	headerOffset := 0
	payloadOffset := len(p.RedundantBlocks)*redundantHeaderSize + primaryHeaderSize

	for _, block := range p.RedundantBlocks {
		blockLength := len(block.Payload)
		raw[headerOffset] = followBit | block.PayloadType
		raw[headerOffset+1] = byte(block.TimestampOffset >> 6) //nolint:gosec // Offset is validated as 14 bits.
		raw[headerOffset+2] = byte((block.TimestampOffset&0x3f)<<2) | byte(blockLength>>8)
		raw[headerOffset+3] = byte(blockLength)
		headerOffset += redundantHeaderSize

		payloadOffset += copy(raw[payloadOffset:], block.Payload)
	}

	raw[headerOffset] = p.PrimaryBlock.PayloadType
	copy(raw[payloadOffset:], p.PrimaryBlock.Payload)

	return p.MarshalSize()
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
	if len(raw) < primaryHeaderSize {
		return errMalformedPayload
	}

	type blockHeader struct {
		payloadType     uint8
		timestampOffset uint16
		blockLength     int
	}

	var headers [maxRedundantBlocks]blockHeader
	headerCount := 0
	headerOffset := 0
	primaryPayloadType := uint8(0)

	for {
		if headerOffset >= len(raw) {
			return fmt.Errorf("%w: missing primary block header", errMalformedPayload)
		}

		if raw[headerOffset]&followBit == 0 {
			primaryPayloadType = raw[headerOffset] & maxPayloadType
			headerOffset += primaryHeaderSize

			break
		}

		if headerCount == maxRedundantBlocks {
			return errTooManyBlocks
		}
		if len(raw)-headerOffset < redundantHeaderSize {
			return fmt.Errorf("%w: truncated redundant block header", errMalformedPayload)
		}

		headers[headerCount] = blockHeader{
			payloadType:     raw[headerOffset] & maxPayloadType,
			timestampOffset: uint16(raw[headerOffset+1])<<6 | uint16(raw[headerOffset+2])>>2,
			blockLength:     int(raw[headerOffset+2]&0x03)<<8 | int(raw[headerOffset+3]),
		}
		headerCount++
		headerOffset += redundantHeaderSize
	}

	parsed := Payload{
		RedundantBlocks: make([]Block, headerCount),
		PrimaryBlock: Block{
			PayloadType: primaryPayloadType,
		},
	}
	payloadOffset := headerOffset
	for i := range headerCount {
		header := headers[i]
		if len(raw)-payloadOffset < header.blockLength {
			return fmt.Errorf("%w: redundant block length exceeds payload", errMalformedPayload)
		}

		parsed.RedundantBlocks[i] = Block{
			PayloadType:     header.payloadType,
			TimestampOffset: header.timestampOffset,
			Payload:         raw[payloadOffset : payloadOffset+header.blockLength],
		}
		payloadOffset += header.blockLength
	}
	parsed.PrimaryBlock.Payload = raw[payloadOffset:]

	*p = parsed

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
