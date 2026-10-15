// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"bytes"
	"cmp"
	"slices"

	"github.com/pion/rtp"
)

type recoveryCandidate struct {
	block     Block
	extended  int64
	timestamp uint32
}

func (stream *receiverStream) recoveryCandidates(
	header rtp.Header, blocks []Block, position sequencePosition,
) []recoveryCandidate {
	usable := stream.usableRedundantBlocks(blocks)
	if len(usable) == 0 {
		return nil
	}
	if !stream.history.matchesPrimary(position, header.Timestamp) {
		return nil
	}

	if stream.timestampStep > 0 && stream.history.matchesTimestampStep(position, header.Timestamp, stream.timestampStep) {
		return stream.fixedDurationCandidates(header, usable, position)
	}

	// RED carries media timestamps, not original sequence numbers. Without an
	// explicit cadence contract, only a completely covered, anchored loss gap
	// identifies the packet numbers uniquely. A partial gap must remain missing.
	if position.reset {
		return nil
	}
	anchors := stream.history.mediaAnchors(position, header.Timestamp)

	candidates := make([]recoveryCandidate, 0, len(usable))
	for index := 1; index < len(anchors); index++ {
		candidates = append(candidates, stream.gapCandidates(header, usable, anchors[index-1], anchors[index])...)
	}

	return candidates
}

func (stream *receiverStream) usableRedundantBlocks(blocks []Block) []Block {
	usable := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		if block.PayloadType == stream.opusPayloadType && len(block.Payload) > 0 && block.TimestampOffset > 0 {
			usable = append(usable, block)
		}
	}
	slices.SortFunc(usable, func(left, right Block) int {
		return cmp.Compare(right.TimestampOffset, left.TimestampOffset)
	})

	// Identical copies identify one media packet. Conflicting copies at the
	// same timestamp cannot be safely selected and must not establish a mapping.
	unique := usable[:0]
	for index := 0; index < len(usable); {
		block := usable[index]
		next := index + 1
		conflict := false
		for next < len(usable) && usable[next].TimestampOffset == block.TimestampOffset {
			conflict = conflict || !bytes.Equal(block.Payload, usable[next].Payload)
			next++
		}
		if !conflict {
			unique = append(unique, block)
		}
		index = next
	}

	return unique
}

func (stream *receiverStream) fixedDurationCandidates(
	header rtp.Header, blocks []Block, position sequencePosition,
) []recoveryCandidate {
	candidates := make([]recoveryCandidate, 0, len(blocks))
	for _, block := range blocks {
		offset := uint32(block.TimestampOffset)
		if offset%stream.timestampStep != 0 {
			continue
		}
		candidates = append(candidates, recoveryCandidate{
			block:     block,
			extended:  position.extended - int64(offset/stream.timestampStep),
			timestamp: header.Timestamp - offset,
		})
	}

	return candidates
}

func (stream *receiverStream) gapCandidates(
	header rtp.Header, blocks []Block, left, right receiveHistoryEntry,
) []recoveryCandidate {
	if right.extended-left.extended <= 1 {
		return nil
	}
	timestampDistance := right.timestamp - left.timestamp
	if timestampDistance == 0 || timestampDistance >= 1<<31 {
		return nil
	}

	missing := stream.history.missingSequences(left.extended, right.extended)
	if len(missing) == 0 {
		return nil
	}

	candidates := make([]recoveryCandidate, 0, len(missing))
	for _, block := range blocks {
		timestamp := header.Timestamp - uint32(block.TimestampOffset)
		distance := timestamp - left.timestamp
		if distance > 0 && distance < timestampDistance {
			candidates = append(candidates, recoveryCandidate{block: block, timestamp: timestamp})
		}
	}
	if len(candidates) != len(missing) {
		return nil
	}
	for index := range candidates {
		candidates[index].extended = missing[index]
	}

	return candidates
}

func (history *receiveHistory) matchesPrimary(position sequencePosition, timestamp uint32) bool {
	if !history.seen(position) {
		return true
	}
	entry := history.entries[receiveHistoryIndex(position.extended)]

	return entry.media && entry.timestamp == timestamp
}

func (history *receiveHistory) mediaAnchors(position sequencePosition, timestamp uint32) []receiveHistoryEntry {
	anchors := make([]receiveHistoryEntry, 0, receiveWindowSize+1)
	for _, entry := range history.entries {
		if entry.valid && entry.media && !history.stale(position, entry.extended) {
			anchors = append(anchors, entry)
		}
	}
	if !history.seen(position) {
		anchors = append(anchors, receiveHistoryEntry{
			extended: position.extended, timestamp: timestamp, media: true,
		})
	}
	slices.SortFunc(anchors, func(left, right receiveHistoryEntry) int {
		return cmp.Compare(left.extended, right.extended)
	})

	return anchors
}

func (history *receiveHistory) missingSequences(left, right int64) []int64 {
	missing := make([]int64, 0, right-left-1)
	for sequence := left + 1; sequence < right; sequence++ {
		if !history.contains(sequence) {
			missing = append(missing, sequence)
		}
	}

	return missing
}

func (history *receiveHistory) matchesTimestampStep(
	position sequencePosition, timestamp, step uint32,
) bool {
	if position.reset {
		return true
	}
	for _, entry := range history.entries {
		if !entry.valid || history.stale(position, entry.extended) {
			continue
		}
		if !entry.media {
			return false
		}
		sequenceDistance := position.extended - entry.extended
		expected := uint32((sequenceDistance * int64(step)) & 0xffffffff) //nolint:gosec // Mask restores wrapped ticks.
		if timestamp-entry.timestamp != expected {
			return false
		}
	}

	return true
}
