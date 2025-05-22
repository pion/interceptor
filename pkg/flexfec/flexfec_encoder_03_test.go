// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package flexfec

import (
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestFlexEncoder03_EncodeFec_EmptyMediaPackets(t *testing.T) {
	encoder := FlexEncoder03Factory{}.NewEncoder(96, 1234)
	var mediaPackets []rtp.Packet

	result := encoder.EncodeFec(mediaPackets, 1)

	assert.Nil(t, result, "EncodeFec should return nil when mediaPackets is empty")
}

func TestFlexEncoder03_EncodeFec_OutOfOrderPackets(t *testing.T) {
	encoder := FlexEncoder03Factory{}.NewEncoder(96, 1234)
	mediaPackets := []rtp.Packet{
		{
			Header: rtp.Header{
				SequenceNumber: 2,
				SSRC:           1234,
			},
		},
		{
			Header: rtp.Header{
				SequenceNumber: 1, // Out of order (should be 3)
				SSRC:           1234,
			},
		},
	}

	result := encoder.EncodeFec(mediaPackets, 1)

	assert.Nil(t, result, "EncodeFec should return nil when packets are out of order")
}

func TestFlexEncoder03_EncodeFec_MissingPackets(t *testing.T) {
	encoder := FlexEncoder03Factory{}.NewEncoder(96, 1234)
	mediaPackets := []rtp.Packet{
		{
			Header: rtp.Header{
				SequenceNumber: 1,
				SSRC:           1234,
			},
		},
		{
			Header: rtp.Header{
				SequenceNumber: 3, // Missing packet with sequence number 2
				SSRC:           1234,
			},
		},
	}

	result := encoder.EncodeFec(mediaPackets, 1)
	assert.Nil(t, result, "EncodeFec should return nil when there are missing packets")
}

func TestFlexEncoder03_EncodeFec_DifferentPayloadSizes(t *testing.T) {
	encoder := FlexEncoder03Factory{}.NewEncoder(96, 1234)

	smallPayload := []byte{1, 2, 3}
	largePayload := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	mediaPackets := []rtp.Packet{
		{
			Header: rtp.Header{
				SequenceNumber: 1,
				SSRC:           1234,
			},
			Payload: smallPayload,
		},
		{
			Header: rtp.Header{
				SequenceNumber: 2,
				SSRC:           1234,
			},
			Payload: largePayload,
		},
	}

	fecPackets := encoder.EncodeFec(mediaPackets, 1)

	assert.NotNil(t, fecPackets, "EncodeFec should return FEC packets")
	assert.Equal(t, 1, len(fecPackets), "EncodeFec should return 1 FEC packet")

	expectedPayloadSize := len(largePayload)
	actualPayloadSize := len(fecPackets[0].Payload) - BaseFec03HeaderSize

	assert.Equal(t, expectedPayloadSize, actualPayloadSize,
		"FEC payload size should match the size of the largest media packet payload")
}
