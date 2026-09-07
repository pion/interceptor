// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"fmt"
	"io"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

const (
	receiveWindowSize     = 64
	sequenceNumberModulus = int64(1 << 16)
	sequenceNumberHalf    = sequenceNumberModulus / 2
)

type receiverOutput struct {
	raw        []byte
	attributes interceptor.Attributes
}

type receiverStream struct {
	reader          interceptor.RTPReader
	ssrc            uint32
	opusPayloadType uint8
	redPayloadType  uint8
	history         receiveHistory
	pending         []receiverOutput
}

type sequencePosition struct {
	extended int64
	reset    bool
	stale    bool
}

type receiveHistoryEntry struct {
	extended int64
	valid    bool
}

type receiveHistory struct {
	entries     [receiveWindowSize]receiveHistoryEntry
	highest     int64
	initialized bool
}

func (stream *receiverStream) Read(
	buffer []byte, attributes interceptor.Attributes,
) (int, interceptor.Attributes, error) {
	for {
		if len(stream.pending) > 0 {
			return stream.emitPending(buffer)
		}

		n, receivedAttributes, ready, err := stream.readIncoming(buffer, attributes)
		if err != nil || ready {
			return n, receivedAttributes, err
		}
	}
}

func (stream *receiverStream) readIncoming(
	buffer []byte, attributes interceptor.Attributes,
) (int, interceptor.Attributes, bool, error) {
	readAttributes := cloneAttributes(attributes)
	readAttributes.SetRTPHeader(nil)
	n, receivedAttributes, err := stream.reader.Read(buffer, readAttributes)
	if err != nil {
		return n, receivedAttributes, false, err
	}

	header, err := receivedAttributes.GetRTPHeader(buffer[:n])
	if err != nil {
		return 0, receivedAttributes, false, err
	}
	if header.SSRC != stream.ssrc {
		return n, receivedAttributes, true, nil
	}
	if header.PayloadType == stream.redPayloadType {
		return stream.readRED(buffer[:n], receivedAttributes)
	}

	return stream.readPlain(n, receivedAttributes, header.SequenceNumber)
}

func (stream *receiverStream) readPlain(
	n int, attributes interceptor.Attributes, sequenceNumber uint16,
) (int, interceptor.Attributes, bool, error) {
	position := stream.history.position(sequenceNumber)
	if position.stale || stream.history.seen(position) {
		return 0, attributes, false, nil
	}

	stream.history.commit(position, nil)

	return n, attributes, true, nil
}

func (stream *receiverStream) readRED(
	raw []byte, attributes interceptor.Attributes,
) (int, interceptor.Attributes, bool, error) {
	packet, redPayload, err := parseREDPacket(raw, stream.opusPayloadType)
	if err != nil {
		return 0, attributes, false, err
	}

	position := stream.history.position(packet.SequenceNumber)
	if position.stale || stream.history.seen(position) {
		return 0, attributes, false, nil
	}

	outputs, recoveredSequences, err := stream.buildOutputs(packet, redPayload, position, attributes)
	if err != nil {
		return 0, attributes, false, err
	}

	stream.history.commit(position, recoveredSequences)
	stream.pending = outputs

	return 0, attributes, false, nil
}

func (stream *receiverStream) emitPending(buffer []byte) (int, interceptor.Attributes, error) {
	output := stream.pending[0]
	if len(buffer) < len(output.raw) {
		return 0, output.attributes, io.ErrShortBuffer
	}

	copy(buffer, output.raw)
	stream.pending[0] = receiverOutput{}
	stream.pending = stream.pending[1:]
	if len(stream.pending) == 0 {
		stream.pending = nil
	}

	return len(output.raw), output.attributes, nil
}

func (stream *receiverStream) buildOutputs(
	packet rtp.Packet,
	redPayload Payload,
	position sequencePosition,
	attributes interceptor.Attributes,
) ([]receiverOutput, []int64, error) {
	outputs := make([]receiverOutput, 0, len(redPayload.RedundantBlocks)+1)
	recoveredSequences := make([]int64, 0, len(redPayload.RedundantBlocks))
	for index, block := range redPayload.RedundantBlocks {
		distance := len(redPayload.RedundantBlocks) - index
		extendedSequence := position.extended - int64(distance) //nolint:gosec // RED blocks are capped at 32.
		if !stream.shouldRecover(block, position, extendedSequence) {
			continue
		}

		output, err := newReceiverOutput(
			recoveredPacket(packet.Header, block, extendedSequence),
			attributes,
		)
		if err != nil {
			return nil, nil, err
		}

		outputs = append(outputs, output)
		recoveredSequences = append(recoveredSequences, extendedSequence)
	}

	primaryPacket := rtp.Packet{
		Header:  packet.Header.Clone(),
		Payload: redPayload.PrimaryBlock.Payload,
	}
	primaryPacket.PayloadType = stream.opusPayloadType
	primaryOutput, err := newReceiverOutput(primaryPacket, attributes)
	if err != nil {
		return nil, nil, err
	}

	return append(outputs, primaryOutput), recoveredSequences, nil
}

func (stream *receiverStream) shouldRecover(
	block Block, position sequencePosition, extendedSequence int64,
) bool {
	return block.PayloadType == stream.opusPayloadType &&
		len(block.Payload) > 0 &&
		block.TimestampOffset > 0 &&
		!stream.history.stale(position, extendedSequence) &&
		!stream.history.containsForPosition(position, extendedSequence)
}

func parseREDPacket(raw []byte, opusPayloadType uint8) (rtp.Packet, Payload, error) {
	var packet rtp.Packet
	if err := packet.Unmarshal(raw); err != nil {
		return rtp.Packet{}, Payload{}, err
	}

	var redPayload Payload
	if err := redPayload.Unmarshal(packet.Payload); err != nil {
		return rtp.Packet{}, Payload{}, fmt.Errorf("%w: %w", errInvalidREDPayload, err)
	}
	if redPayload.PrimaryBlock.PayloadType != opusPayloadType {
		return rtp.Packet{}, Payload{}, fmt.Errorf(
			"%w: got %d, expected %d",
			errUnexpectedPrimaryPayloadType,
			redPayload.PrimaryBlock.PayloadType,
			opusPayloadType,
		)
	}
	if len(redPayload.PrimaryBlock.Payload) == 0 {
		return rtp.Packet{}, Payload{}, errEmptyPrimaryPayload
	}

	return packet, redPayload, nil
}

func recoveredPacket(header rtp.Header, block Block, extendedSequence int64) rtp.Packet {
	return rtp.Packet{
		Header: rtp.Header{
			Version:        header.Version,
			PayloadType:    block.PayloadType,
			SequenceNumber: uint16(extendedSequence & 0xffff), //nolint:gosec // Masking restores the wrapped value.
			Timestamp:      header.Timestamp - uint32(block.TimestampOffset),
			SSRC:           header.SSRC,
			CSRC:           append([]uint32(nil), header.CSRC...),
		},
		Payload: block.Payload,
	}
}

func newReceiverOutput(packet rtp.Packet, attributes interceptor.Attributes) (receiverOutput, error) {
	raw, err := packet.Marshal()
	if err != nil {
		return receiverOutput{}, err
	}

	outputAttributes := cloneAttributes(attributes)
	if outputAttributes == nil {
		outputAttributes = interceptor.Attributes{}
	}
	cachedHeader := packet.Header.Clone()
	outputAttributes.SetRTPHeader(&cachedHeader)

	return receiverOutput{raw: raw, attributes: outputAttributes}, nil
}

func (history *receiveHistory) position(sequenceNumber uint16) sequencePosition {
	if !history.initialized {
		return sequencePosition{extended: int64(sequenceNumber)}
	}

	extended := extendSequenceNumber(sequenceNumber, history.highest)
	position := sequencePosition{extended: extended}
	switch {
	case extended-history.highest >= receiveWindowSize:
		position.reset = true
	case history.highest-extended >= receiveWindowSize:
		position.stale = true
	}

	return position
}

func (history *receiveHistory) seen(position sequencePosition) bool {
	return !position.reset && history.contains(position.extended)
}

func (history *receiveHistory) containsForPosition(position sequencePosition, extended int64) bool {
	return !position.reset && history.contains(extended)
}

func (history *receiveHistory) contains(extended int64) bool {
	entry := history.entries[receiveHistoryIndex(extended)]

	return entry.valid && entry.extended == extended
}

func (history *receiveHistory) stale(position sequencePosition, extended int64) bool {
	reference := history.highest
	if !history.initialized || position.reset || position.extended > reference {
		reference = position.extended
	}

	return reference-extended >= receiveWindowSize
}

func (history *receiveHistory) commit(position sequencePosition, recovered []int64) {
	if !history.initialized || position.reset {
		history.entries = [receiveWindowSize]receiveHistoryEntry{}
		history.highest = position.extended
		history.initialized = true
	} else if position.extended > history.highest {
		history.highest = position.extended
	}

	for _, extended := range recovered {
		history.mark(extended)
	}
	history.mark(position.extended)
}

func (history *receiveHistory) mark(extended int64) {
	history.entries[receiveHistoryIndex(extended)] = receiveHistoryEntry{
		extended: extended,
		valid:    true,
	}
}

func receiveHistoryIndex(extended int64) int {
	index := extended % receiveWindowSize
	if index < 0 {
		index += receiveWindowSize
	}

	return int(index) //nolint:gosec // The result is between zero and receiveWindowSize-1.
}

func extendSequenceNumber(sequenceNumber uint16, reference int64) int64 {
	extended := reference&^(sequenceNumberModulus-1) | int64(sequenceNumber)
	if extended-reference > sequenceNumberHalf {
		extended -= sequenceNumberModulus
	} else if reference-extended > sequenceNumberHalf {
		extended += sequenceNumberModulus
	}

	return extended
}
