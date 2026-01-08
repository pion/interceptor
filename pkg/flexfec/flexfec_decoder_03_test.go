// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"encoding/binary"
	"testing"

	"github.com/pion/logging"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDecoderSSRC         = uint32(1234)
	testProtectedStreamSSRC = uint32(5678)
)

func TestFECDecoderInsertPacketRemovesOldFEC(t *testing.T) {
	decoder := newFECDecoder(testDecoderSSRC, testProtectedStreamSSRC, logging.NewDefaultLoggerFactory())
	decoder.receivedFECPackets = []fecPacketState{
		newFecPacketState(1),
		newFecPacketState(500),
		newFecPacketState(1500),
		newFecPacketState(25000),
	}

	pkt := rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 40000,
			SSRC:           testDecoderSSRC,
		},
		Payload: buildTestFlexFecPayload(40000),
	}

	decoder.insertPacket(pkt)

	require.Len(t, decoder.receivedFECPackets, 2)
	assert.Equal(t, uint16(25000), decoder.receivedFECPackets[0].packet.SequenceNumber)
	assert.Equal(t, uint16(40000), decoder.receivedFECPackets[1].packet.SequenceNumber)
}

func TestFECDecoderInsertPacketKeepsRecentFEC(t *testing.T) {
	decoder := newFECDecoder(testDecoderSSRC, testProtectedStreamSSRC, logging.NewDefaultLoggerFactory())
	initialStates := []fecPacketState{
		newFecPacketState(1),
		newFecPacketState(500),
		newFecPacketState(1500),
	}
	decoder.receivedFECPackets = append(decoder.receivedFECPackets, initialStates...)

	pkt := rtp.Packet{
		Header: rtp.Header{
			SequenceNumber: 2000,
			SSRC:           testDecoderSSRC,
		},
		Payload: buildTestFlexFecPayload(2000),
	}

	decoder.insertPacket(pkt)

	require.Len(t, decoder.receivedFECPackets, len(initialStates)+1)
	for i, state := range initialStates {
		assert.Equal(t, state.packet.SequenceNumber, decoder.receivedFECPackets[i].packet.SequenceNumber)
	}
	assert.Equal(t, uint16(2000), decoder.receivedFECPackets[len(initialStates)].packet.SequenceNumber)
}

func newFecPacketState(seq uint16) fecPacketState {
	return fecPacketState{
		packet: rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: seq,
				SSRC:           testDecoderSSRC,
			},
		},
	}
}

func buildTestFlexFecPayload(seqNumBase uint16) []byte {
	payload := make([]byte, BaseFec03HeaderSize+4)
	payload[8] = 1
	binary.BigEndian.PutUint32(payload[12:], testProtectedStreamSSRC)
	binary.BigEndian.PutUint16(payload[16:], seqNumBase)
	binary.BigEndian.PutUint16(payload[18:], 0x8001)

	return payload
}
