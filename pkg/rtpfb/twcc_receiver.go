// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"time"

	"github.com/pion/rtcp"
)

// nolint
func convertTWCC(feedback *rtcp.TransportLayerCC) []acknowledgement {
	if feedback == nil {
		return nil
	}
	var acks []acknowledgement

	nextTimestamp := time.Time{}.Add(time.Duration(feedback.ReferenceTime) * 64 * time.Millisecond)
	recvDeltaIndex := 0

	offset := 0
	processed := uint16(0)
	for _, pc := range feedback.PacketChunks {
		if processed >= feedback.PacketStatusCount {
			break
		}
		switch chunk := pc.(type) {
		case *rtcp.RunLengthChunk:
			runLength := chunk.RunLength
			if remaining := feedback.PacketStatusCount - processed; runLength > remaining {
				runLength = remaining
			}
			for i := uint16(0); i < runLength; i++ {
				seqNr := feedback.BaseSequenceNumber + uint16(offset) // nolint:gosec
				offset++
				processed++
				switch chunk.PacketStatusSymbol {
				case rtcp.TypeTCCPacketNotReceived:
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        false,
						arrival:        time.Time{},
						ecn:            0,
					})
				case rtcp.TypeTCCPacketReceivedSmallDelta, rtcp.TypeTCCPacketReceivedLargeDelta:
					if recvDeltaIndex >= len(feedback.RecvDeltas) {
						continue
					}
					delta := feedback.RecvDeltas[recvDeltaIndex]
					nextTimestamp = nextTimestamp.Add(time.Duration(delta.Delta) * time.Microsecond)
					recvDeltaIndex++
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        true,
						arrival:        nextTimestamp,
						ecn:            0,
					})
				case rtcp.TypeTCCPacketReceivedWithoutDelta:
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        true,
						arrival:        time.Time{},
						ecn:            0,
					})
				}
			}
		case *rtcp.StatusVectorChunk:
			for _, s := range chunk.SymbolList {
				if processed >= feedback.PacketStatusCount {
					break
				}
				seqNr := feedback.BaseSequenceNumber + uint16(offset) // nolint:gosec
				offset++
				processed++
				switch s {
				case rtcp.TypeTCCPacketNotReceived:
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        false,
						arrival:        time.Time{},
						ecn:            0,
					})
				case rtcp.TypeTCCPacketReceivedSmallDelta, rtcp.TypeTCCPacketReceivedLargeDelta:
					if recvDeltaIndex >= len(feedback.RecvDeltas) {
						continue
					}
					delta := feedback.RecvDeltas[recvDeltaIndex]
					nextTimestamp = nextTimestamp.Add(time.Duration(delta.Delta) * time.Microsecond)
					recvDeltaIndex++
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        true,
						arrival:        nextTimestamp,
						ecn:            0,
					})
				case rtcp.TypeTCCPacketReceivedWithoutDelta:
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        true,
						arrival:        time.Time{},
						ecn:            0,
					})
				}
			}
		}
	}

	return acks
}
