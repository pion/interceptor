// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	payloadType         = uint8(49)
	ssrc                = uint32(867589674)
	protectedStreamSSRC = uint32(476325762)
)

func checkAnyPacketCanBeRecovered(t *testing.T, mediaPackets []rtp.Packet, fecPackets []rtp.Packet) {
	t.Helper()

	for lost := 0; lost < len(mediaPackets); lost++ {
		decoder := newFECDecoder(ssrc, protectedStreamSSRC)
		recoveredPackets := make([]rtp.Packet, 0)
		// lose one packet
		for _, mediaPacket := range mediaPackets[:lost] {
			recoveredPackets = append(recoveredPackets, decoder.DecodeFec(mediaPacket)...)
		}
		for _, mediaPacket := range mediaPackets[lost+1:] {
			recoveredPackets = append(recoveredPackets, decoder.DecodeFec(mediaPacket)...)
		}
		assert.Empty(t, recoveredPackets)

		for _, fecPacket := range fecPackets {
			recoveredPackets = append(recoveredPackets, decoder.DecodeFec(fecPacket)...)
		}

		require.Len(t, recoveredPackets, 1)
		assert.Equal(t, mediaPackets[lost], recoveredPackets[0])
	}
}

func generatePackets(t *testing.T, seqs []uint16) ([]rtp.Packet, []rtp.Packet) {
	t.Helper()

	mediaPackets := make([]rtp.Packet, 0)
	for i, seq := range seqs {
		payload := []byte{
			// Payload
			1, 2, 3, 4, 5, byte(i),
		}
		packet := rtp.Packet{
			Header: rtp.Header{
				Marker:      true,
				Extension:   false,
				Version:     2,
				PayloadType: 96,

				SequenceNumber: seq,
				Timestamp:      3653407706,
				SSRC:           protectedStreamSSRC,
				CSRC:           []uint32{},
			},
			Payload: payload,
		}

		extension := []byte{0xAA, 0xAA}
		err := packet.SetExtension(1, extension)
		require.NoError(t, err)

		mediaPackets = append(mediaPackets, packet)
	}

	encoder := FlexEncoder03Factory{}.NewEncoder(payloadType, ssrc)
	fecPackets := encoder.EncodeFec(mediaPackets, 2)

	return mediaPackets, fecPackets
}

func TestFlexFec03_SimpleRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		seqs []uint16
	}{
		{
			name: "first",
			seqs: []uint16{1, 2, 3, 4, 5},
		},
		{
			name: "last",
			seqs: []uint16{65531, 65532, 65533, 65534, 65535},
		},
		{
			name: "wrap",
			seqs: []uint16{65533, 65534, 65535, 0, 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mediaPackets, fecPackets := generatePackets(t, test.seqs)
			require.Len(t, mediaPackets, len(test.seqs))
			checkAnyPacketCanBeRecovered(t, mediaPackets, fecPackets)
		})
	}
}

func TestFlexFec03_WholeRangeRoundTrip(t *testing.T) {
	var seqs []uint16
	const maxFlexFEC03MediaPackets = 109
	for i := 0; i < maxFlexFEC03MediaPackets; i++ {
		seqs = append(seqs, uint16(i)) //nolint:gosec
	}

	mediaPackets, fecPackets := generatePackets(t, seqs)
	require.Len(t, mediaPackets, len(seqs))
	checkAnyPacketCanBeRecovered(t, mediaPackets, fecPackets)
}
