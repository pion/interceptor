// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package util_test

import (
	"testing"

	"github.com/pion/interceptor/pkg/flexfec/util"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func createTestMediaPackets() []rtp.Packet {
	return []rtp.Packet{
		{Header: rtp.Header{SequenceNumber: 1, SSRC: 1}},
		{Header: rtp.Header{SequenceNumber: 2, SSRC: 1}},
		{Header: rtp.Header{SequenceNumber: 3, SSRC: 1}},
		{Header: rtp.Header{SequenceNumber: 4, SSRC: 1}},
		{Header: rtp.Header{SequenceNumber: 5, SSRC: 1}},
	}
}

func TestNewMediaPacketIterator(t *testing.T) {
	mediaPackets := createTestMediaPackets()
	coveredIndices := []uint32{1, 2, 4}
	iterator := util.NewMediaPacketIterator(mediaPackets, coveredIndices)

	assert.NotNil(t, iterator)
}

func TestMediaPacketIteratorReset(t *testing.T) {
	mediaPackets := createTestMediaPackets()
	coveredIndices := []uint32{1, 2, 4}
	iterator := util.NewMediaPacketIterator(mediaPackets, coveredIndices)

	// Advance the iterator
	_ = iterator.Next()
	_ = iterator.Next()

	// Reset the iterator
	result := iterator.Reset()

	// After reset, HasNext should be true again
	assert.True(t, iterator.HasNext())

	// Reset should return the iterator for chaining
	assert.Equal(t, iterator, result, "Reset should return the iterator for chaining")
}

func TestMediaPacketIteratorHasNext(t *testing.T) {
	mediaPackets := createTestMediaPackets()
	coveredIndices := []uint32{1, 2, 4}
	iterator := util.NewMediaPacketIterator(mediaPackets, coveredIndices)

	// Initially, HasNext should be true
	assert.True(t, iterator.HasNext())

	// Advance to the last element
	_ = iterator.Next()
	_ = iterator.Next()
	assert.True(t, iterator.HasNext())

	// Advance past the last element
	_ = iterator.Next()
	assert.False(t, iterator.HasNext())
}

func TestMediaPacketIteratorNext(t *testing.T) {
	mediaPackets := createTestMediaPackets()
	coveredIndices := []uint32{1, 2, 4}
	iterator := util.NewMediaPacketIterator(mediaPackets, coveredIndices)

	// First call to Next should return the first covered packet
	packet := iterator.Next()
	assert.NotNil(t, packet)
	assert.Equal(t, uint16(2), packet.SequenceNumber)

	// Second call to Next should return the second covered packet
	packet = iterator.Next()
	assert.NotNil(t, packet)
	assert.Equal(t, uint16(3), packet.SequenceNumber)

	// Third call to Next should return the third covered packet
	packet = iterator.Next()
	assert.NotNil(t, packet)
	assert.Equal(t, uint16(5), packet.SequenceNumber)

	// Fourth call to Next should return nil
	packet = iterator.Next()
	assert.Nil(t, packet)
}

func TestMediaPacketIteratorFirst(t *testing.T) {
	mediaPackets := createTestMediaPackets()
	coveredIndices := []uint32{1, 2, 4}
	iterator := util.NewMediaPacketIterator(mediaPackets, coveredIndices)

	// First should return the first covered packet
	packet := iterator.First()
	assert.NotNil(t, packet)
	assert.Equal(t, uint16(2), packet.SequenceNumber)

	// First should not advance the iterator, so Next should still return the first packet
	nextPacket := iterator.Next()
	assert.NotNil(t, nextPacket)
	assert.Equal(t, uint16(2), nextPacket.SequenceNumber)

	// Even after advancing the iterator, First should still return the first packet
	_ = iterator.Next()
	packet = iterator.First()
	assert.NotNil(t, packet)
	assert.Equal(t, uint16(2), packet.SequenceNumber)
}

func TestMediaPacketIteratorEmptyCoveredIndices(t *testing.T) {
	mediaPackets := createTestMediaPackets()
	coveredIndices := []uint32{}
	iterator := util.NewMediaPacketIterator(mediaPackets, coveredIndices)

	assert.False(t, iterator.HasNext())
	assert.Nil(t, iterator.Next())
}
