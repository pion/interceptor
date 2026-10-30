// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"time"

	"github.com/pion/rtcp"
)

// nolint
func convertTWCC(feedback *rtcp.TransportLayerCC, inRange func(uint16, uint16) []uint16) []acknowledgement {
	if feedback == nil {
		return nil
	}
	var acks []acknowledgement

	nextTimestamp := time.Time{}.Add(time.Duration(feedback.ReferenceTime) * 64 * time.Millisecond)
	recvDeltaIndex := 0
	processed := uint16(0)
	for _, pc := range feedback.PacketChunks {
		if processed >= feedback.PacketStatusCount {
			break
		}
		var symbol, count uint16
		var symbols []uint16
		switch chunk := pc.(type) {
		case *rtcp.RunLengthChunk:
			symbol = chunk.PacketStatusSymbol
			count = min(chunk.RunLength, feedback.PacketStatusCount-processed)
			if symbol == rtcp.TypeTCCPacketNotReceived || symbol == rtcp.TypeTCCPacketReceivedWithoutDelta {
				for _, seqNr := range inRange(feedback.BaseSequenceNumber+processed, count) {
					acks = append(acks, acknowledgement{
						sequenceNumber: seqNr,
						arrived:        symbol == rtcp.TypeTCCPacketReceivedWithoutDelta,
					})
				}
				processed += count

				continue
			}
		case *rtcp.StatusVectorChunk:
			symbols = chunk.SymbolList
			count = uint16(min(len(symbols), int(feedback.PacketStatusCount-processed)))
		}
		for index := range count {
			if symbols != nil {
				symbol = symbols[index]
			}
			ack := acknowledgement{
				sequenceNumber: feedback.BaseSequenceNumber + processed,
				arrived:        symbol != rtcp.TypeTCCPacketNotReceived,
			}
			processed++
			switch symbol {
			case rtcp.TypeTCCPacketReceivedSmallDelta, rtcp.TypeTCCPacketReceivedLargeDelta:
				if recvDeltaIndex >= len(feedback.RecvDeltas) {
					continue
				}
				nextTimestamp = nextTimestamp.Add(time.Duration(feedback.RecvDeltas[recvDeltaIndex].Delta) * time.Microsecond)
				recvDeltaIndex++
				ack.arrival = nextTimestamp
			case rtcp.TypeTCCPacketNotReceived, rtcp.TypeTCCPacketReceivedWithoutDelta:
			default:
				continue
			}
			acks = append(acks, ack)
		}
	}

	return acks
}
