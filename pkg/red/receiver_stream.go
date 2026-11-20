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
	timestampStep   uint32
	history         receiveHistory
	pending         []receiverOutput
}

type receiveHistoryEntry struct {
	extended  int64
	timestamp uint32
	media     bool
}

type receiveHistory struct {
	entries  [receiveWindowSize]receiveHistoryEntry
	highest  int64
	received uint64 // Bit i records the packet at highest-i.
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
	if header.Padding {
		var packet rtp.Packet
		if err := packet.Unmarshal(buffer[:n]); err != nil {
			return 0, receivedAttributes, false, err
		}
		if len(packet.Payload) == 0 {
			position := stream.history.position(packet.SequenceNumber)
			if !stream.history.stale(position, position) {
				stream.history.commit(position, 0, false, nil)
			}

			return 0, receivedAttributes, false, nil
		}
	}
	if header.PayloadType == stream.redPayloadType {
		return stream.readRED(buffer[:n], receivedAttributes)
	}

	return stream.readPlain(n, receivedAttributes, header)
}

func (stream *receiverStream) readPlain(
	n int, attributes interceptor.Attributes, header *rtp.Header,
) (int, interceptor.Attributes, bool, error) {
	position := stream.history.position(header.SequenceNumber)
	if stream.history.stale(position, position) || stream.history.contains(position) {
		return 0, attributes, false, nil
	}

	stream.history.commit(position, header.Timestamp, header.PayloadType == stream.opusPayloadType, nil)

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
	if stream.history.stale(position, position) {
		return 0, attributes, false, nil
	}

	outputs, recoveredSequences, err := stream.buildOutputs(
		packet,
		redPayload,
		position,
		attributes,
		!stream.history.contains(position),
	)
	if err != nil {
		return 0, attributes, false, err
	}
	if len(outputs) == 0 {
		return 0, attributes, false, nil
	}

	stream.history.commit(position, packet.Timestamp, true, recoveredSequences)
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
	position int64,
	attributes interceptor.Attributes,
	includePrimary bool,
) ([]receiverOutput, []receiveHistoryEntry, error) {
	candidates := stream.recoveryCandidates(packet.Header, redPayload.RedundantBlocks, position)
	outputs := make([]receiverOutput, 0, len(candidates)+1)
	recoveredSequences := make([]receiveHistoryEntry, 0, len(candidates))
	for _, candidate := range candidates {
		if stream.history.stale(position, candidate.extended) || stream.history.contains(candidate.extended) {
			continue
		}

		output, err := newReceiverOutput(
			recoveredPacket(packet.Header, candidate.block, candidate.extended),
			attributes,
		)
		if err != nil {
			return nil, nil, err
		}

		outputs = append(outputs, output)
		recoveredSequences = append(recoveredSequences, receiveHistoryEntry{
			extended:  candidate.extended,
			timestamp: candidate.timestamp,
			media:     true,
		})
	}

	if includePrimary {
		primaryPacket := rtp.Packet{
			Header:  packet.Header.Clone(),
			Payload: redPayload.PrimaryBlock.Payload,
		}
		primaryPacket.PayloadType = stream.opusPayloadType
		primaryOutput, err := newReceiverOutput(primaryPacket, attributes)
		if err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, primaryOutput)
	}

	return outputs, recoveredSequences, nil
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

func (history *receiveHistory) position(sequenceNumber uint16) int64 {
	if history.received == 0 {
		return int64(sequenceNumber)
	}

	return extendSequenceNumber(sequenceNumber, history.highest)
}

func (history *receiveHistory) contains(extended int64) bool {
	age := history.highest - extended

	return age >= 0 && history.received>>age&1 == 1
}

// entry returns the slot for extended; ok reports whether that sequence number was received.
// Slots are only meaningful when ok is true; unwritten slots carry a zero sequence number.
func (history *receiveHistory) entry(extended int64) (receiveHistoryEntry, bool) {
	return history.entries[extended&(receiveWindowSize-1)], history.contains(extended)
}

func (history *receiveHistory) stale(position, extended int64) bool {
	return max(history.highest, position)-extended >= receiveWindowSize
}

func (history *receiveHistory) receivedWith(extended int64) uint64 {
	if extended > history.highest {
		return history.received<<(extended-history.highest) | 1
	}

	return history.received | 1<<(history.highest-extended)
}

func (history *receiveHistory) commit(
	position int64, timestamp uint32, media bool, recovered []receiveHistoryEntry,
) {
	history.received = history.receivedWith(position)
	history.highest = max(history.highest, position)

	for _, entry := range recovered {
		history.received |= 1 << (history.highest - entry.extended)
		history.mark(entry)
	}
	history.mark(receiveHistoryEntry{extended: position, timestamp: timestamp, media: media})
}

func (history *receiveHistory) mark(entry receiveHistoryEntry) {
	history.entries[entry.extended&(receiveWindowSize-1)] = entry
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
