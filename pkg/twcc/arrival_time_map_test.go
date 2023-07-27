// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package twcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArrivalTimeMap(t *testing.T) {
	t.Run("consistent when empty", func(t *testing.T) {
		var m packetArrivalTimeMap
		assert.Equal(t, m.BeginSequenceNumber(), m.EndSequenceNumber())
		assert.False(t, m.HasReceived(0))
		assert.Equal(t, int64(0), m.Clamp(-5))
		assert.Equal(t, int64(0), m.Clamp(5))
	})

	t.Run("inserts first item into map", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 10)
		assert.Equal(t, int64(42), m.BeginSequenceNumber())
		assert.Equal(t, int64(43), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(41))
		assert.True(t, m.HasReceived(42))
		assert.False(t, m.HasReceived(43))
		assert.False(t, m.HasReceived(44))

		assert.Equal(t, int64(42), m.Clamp(-100))
		assert.Equal(t, int64(42), m.Clamp(42))
		assert.Equal(t, int64(43), m.Clamp(100))
	})

	t.Run("inserts with gaps", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 0)
		m.AddPacket(45, 11)
		assert.Equal(t, int64(42), m.BeginSequenceNumber())
		assert.Equal(t, int64(46), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(41))
		assert.True(t, m.HasReceived(42))
		assert.False(t, m.HasReceived(43))
		assert.False(t, m.HasReceived(44))
		assert.True(t, m.HasReceived(45))
		assert.False(t, m.HasReceived(46))

		assert.Equal(t, int64(0), m.get(42))
		assert.Less(t, m.get(43), int64(0))
		assert.Less(t, m.get(44), int64(0))
		assert.Equal(t, int64(11), m.get(45))

		assert.Equal(t, int64(42), m.Clamp(-100))
		assert.Equal(t, int64(44), m.Clamp(44))
		assert.Equal(t, int64(46), m.Clamp(100))
	})

	t.Run("find next at or after with gaps", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 0)
		m.AddPacket(45, 11)

		seq, ts, ok := m.FindNextAtOrAfter(42)
		assert.Equal(t, int64(42), seq)
		assert.Equal(t, int64(0), ts)
		assert.True(t, ok)

		seq, ts, ok = m.FindNextAtOrAfter(43)
		assert.Equal(t, int64(45), seq)
		assert.Equal(t, int64(11), ts)
		assert.True(t, ok)
	})

	t.Run("inserts within buffer", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 10)
		m.AddPacket(45, 11)

		m.AddPacket(43, 12)
		m.AddPacket(44, 13)

		assert.False(t, m.HasReceived(41))
		assert.True(t, m.HasReceived(42))
		assert.True(t, m.HasReceived(43))
		assert.True(t, m.HasReceived(44))
		assert.True(t, m.HasReceived(45))
		assert.False(t, m.HasReceived(46))

		assert.Equal(t, int64(10), m.get(42))
		assert.Equal(t, int64(12), m.get(43))
		assert.Equal(t, int64(13), m.get(44))
		assert.Equal(t, int64(11), m.get(45))
	})

	t.Run("grows buffer and removes old", func(t *testing.T) {
		var m packetArrivalTimeMap

		var largeSeqNum int64 = 42 + maxNumberOfPackets
		m.AddPacket(42, 10)
		m.AddPacket(43, 11)
		m.AddPacket(44, 12)
		m.AddPacket(45, 13)
		m.AddPacket(largeSeqNum, 12)

		assert.Equal(t, int64(43), m.BeginSequenceNumber())
		assert.Equal(t, largeSeqNum+1, m.EndSequenceNumber())

		assert.False(t, m.HasReceived(41))
		assert.False(t, m.HasReceived(42))
		assert.True(t, m.HasReceived(43))
		assert.True(t, m.HasReceived(44))
		assert.True(t, m.HasReceived(45))
		assert.False(t, m.HasReceived(46))
		assert.True(t, m.HasReceived(largeSeqNum))
		assert.False(t, m.HasReceived(largeSeqNum+1))
	})

	t.Run("sequence number jump deletes all", func(t *testing.T) {
		var m packetArrivalTimeMap

		var largeSeqNum int64 = 42 + 2*maxNumberOfPackets
		m.AddPacket(42, 10)
		m.AddPacket(largeSeqNum, 12)

		assert.Equal(t, largeSeqNum, m.BeginSequenceNumber())
		assert.Equal(t, largeSeqNum+1, m.EndSequenceNumber())

		assert.False(t, m.HasReceived(42))
		assert.True(t, m.HasReceived(largeSeqNum))
		assert.False(t, m.HasReceived(largeSeqNum+1))
	})

	t.Run("expands before beginning", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 10)
		m.AddPacket(-1000, 13)
		assert.Equal(t, int64(-1000), m.BeginSequenceNumber())
		assert.Equal(t, int64(43), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(-1001))
		assert.True(t, m.HasReceived(-1000))
		assert.False(t, m.HasReceived(-999))
		assert.True(t, m.HasReceived(42))
		assert.False(t, m.HasReceived(43))
	})

	t.Run("expanding before beginning keeps received", func(t *testing.T) {
		var m packetArrivalTimeMap

		var smallSeqNum int64 = 42 - 2*maxNumberOfPackets
		m.AddPacket(42, 10)
		m.AddPacket(smallSeqNum, 13)

		assert.Equal(t, int64(42), m.BeginSequenceNumber())
		assert.Equal(t, int64(43), m.EndSequenceNumber())
	})

	t.Run("erase to removes elements", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 10)
		m.AddPacket(43, 11)
		m.AddPacket(44, 12)
		m.AddPacket(45, 13)

		m.EraseTo(44)

		assert.Equal(t, int64(44), m.BeginSequenceNumber())
		assert.Equal(t, int64(46), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(43))
		assert.True(t, m.HasReceived(44))
		assert.True(t, m.HasReceived(45))
		assert.False(t, m.HasReceived(46))
	})

	t.Run("erases in empty map", func(t *testing.T) {
		var m packetArrivalTimeMap

		assert.Equal(t, m.BeginSequenceNumber(), m.EndSequenceNumber())

		m.EraseTo(m.EndSequenceNumber())
		assert.Equal(t, m.BeginSequenceNumber(), m.EndSequenceNumber())
	})

	t.Run("is tolerant to wrong arguments for erase", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 10)
		m.AddPacket(43, 11)

		m.EraseTo(1)

		assert.Equal(t, int64(42), m.BeginSequenceNumber())
		assert.Equal(t, int64(44), m.EndSequenceNumber())

		m.EraseTo(100)

		assert.Equal(t, int64(44), m.BeginSequenceNumber())
		assert.Equal(t, int64(44), m.EndSequenceNumber())
	})

	//nolint:dupl
	t.Run("erase all remembers beginning sequence number", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(42, 10)
		m.AddPacket(43, 11)
		m.AddPacket(44, 12)
		m.AddPacket(45, 13)

		m.EraseTo(46)
		m.AddPacket(50, 10)

		assert.Equal(t, int64(46), m.BeginSequenceNumber())
		assert.Equal(t, int64(51), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(45))
		assert.False(t, m.HasReceived(46))
		assert.False(t, m.HasReceived(47))
		assert.False(t, m.HasReceived(48))
		assert.False(t, m.HasReceived(49))
		assert.True(t, m.HasReceived(50))
		assert.False(t, m.HasReceived(51))
	})

	//nolint:dupl
	t.Run("erase to missing sequence number", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(37, 10)
		m.AddPacket(39, 11)
		m.AddPacket(40, 12)
		m.AddPacket(41, 13)

		m.EraseTo(38)

		m.AddPacket(42, 40)

		assert.Equal(t, int64(38), m.BeginSequenceNumber())
		assert.Equal(t, int64(43), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(37))
		assert.False(t, m.HasReceived(38))
		assert.True(t, m.HasReceived(39))
		assert.True(t, m.HasReceived(40))
		assert.True(t, m.HasReceived(41))
		assert.True(t, m.HasReceived(42))
		assert.False(t, m.HasReceived(43))
	})

	t.Run("remove old packets", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(37, 10)
		m.AddPacket(39, 11)
		m.AddPacket(40, 12)
		m.AddPacket(41, 13)

		m.RemoveOldPackets(42, 11)

		assert.Equal(t, int64(40), m.BeginSequenceNumber())
		assert.Equal(t, int64(42), m.EndSequenceNumber())

		assert.False(t, m.HasReceived(39))
		assert.True(t, m.HasReceived(40))
		assert.True(t, m.HasReceived(41))
		assert.False(t, m.HasReceived(42))
	})

	t.Run("shrinks buffer when necessary", func(t *testing.T) {
		var m packetArrivalTimeMap
		var largeSeqNum int64 = 100 + maxNumberOfPackets - 1
		m.AddPacket(100, 10)
		m.AddPacket(largeSeqNum, 11)

		m.EraseTo(largeSeqNum - 1)

		assert.Equal(t, largeSeqNum-1, m.BeginSequenceNumber())
		assert.Equal(t, largeSeqNum+1, m.EndSequenceNumber())

		assert.Equal(t, minCapacity, m.capacity())
	})

	t.Run("find next at or after with invalid sequence", func(t *testing.T) {
		var m packetArrivalTimeMap
		m.AddPacket(100, 10)

		_, _, ok := m.FindNextAtOrAfter(101)
		assert.False(t, ok)
	})
}
