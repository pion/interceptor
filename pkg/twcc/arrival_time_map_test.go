// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package twcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:maintidx
func TestArrivalTimeMap(t *testing.T) {
	t.Run("consistent when empty", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		assert.Equal(t, arrivalTimeMap.BeginSequenceNumber(), arrivalTimeMap.EndSequenceNumber())
		assert.False(t, arrivalTimeMap.HasReceived(0))
		assert.Equal(t, int64(0), arrivalTimeMap.Clamp(-5))
		assert.Equal(t, int64(0), arrivalTimeMap.Clamp(5))
	})

	t.Run("inserts first item into map", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 10)
		assert.Equal(t, int64(42), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(43), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(41))
		assert.True(t, arrivalTimeMap.HasReceived(42))
		assert.False(t, arrivalTimeMap.HasReceived(43))
		assert.False(t, arrivalTimeMap.HasReceived(44))

		assert.Equal(t, int64(42), arrivalTimeMap.Clamp(-100))
		assert.Equal(t, int64(42), arrivalTimeMap.Clamp(42))
		assert.Equal(t, int64(43), arrivalTimeMap.Clamp(100))
	})

	t.Run("inserts with gaps", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 0)
		arrivalTimeMap.AddPacket(45, 11)
		assert.Equal(t, int64(42), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(46), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(41))
		assert.True(t, arrivalTimeMap.HasReceived(42))
		assert.False(t, arrivalTimeMap.HasReceived(43))
		assert.False(t, arrivalTimeMap.HasReceived(44))
		assert.True(t, arrivalTimeMap.HasReceived(45))
		assert.False(t, arrivalTimeMap.HasReceived(46))

		assert.Equal(t, int64(0), arrivalTimeMap.get(42))
		assert.Less(t, arrivalTimeMap.get(43), int64(0))
		assert.Less(t, arrivalTimeMap.get(44), int64(0))
		assert.Equal(t, int64(11), arrivalTimeMap.get(45))

		assert.Equal(t, int64(42), arrivalTimeMap.Clamp(-100))
		assert.Equal(t, int64(44), arrivalTimeMap.Clamp(44))
		assert.Equal(t, int64(46), arrivalTimeMap.Clamp(100))
	})

	t.Run("find next at or after with gaps", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 0)
		arrivalTimeMap.AddPacket(45, 11)

		seq, ts, ok := arrivalTimeMap.FindNextAtOrAfter(42)
		assert.Equal(t, int64(42), seq)
		assert.Equal(t, int64(0), ts)
		assert.True(t, ok)

		seq, ts, ok = arrivalTimeMap.FindNextAtOrAfter(43)
		assert.Equal(t, int64(45), seq)
		assert.Equal(t, int64(11), ts)
		assert.True(t, ok)
	})

	t.Run("inserts within buffer", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(45, 11)

		arrivalTimeMap.AddPacket(43, 12)
		arrivalTimeMap.AddPacket(44, 13)

		assert.False(t, arrivalTimeMap.HasReceived(41))
		assert.True(t, arrivalTimeMap.HasReceived(42))
		assert.True(t, arrivalTimeMap.HasReceived(43))
		assert.True(t, arrivalTimeMap.HasReceived(44))
		assert.True(t, arrivalTimeMap.HasReceived(45))
		assert.False(t, arrivalTimeMap.HasReceived(46))

		assert.Equal(t, int64(10), arrivalTimeMap.get(42))
		assert.Equal(t, int64(12), arrivalTimeMap.get(43))
		assert.Equal(t, int64(13), arrivalTimeMap.get(44))
		assert.Equal(t, int64(11), arrivalTimeMap.get(45))
	})

	t.Run("grows buffer and removes old", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap

		var largeSeqNum int64 = 42 + maxNumberOfPackets
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(43, 11)
		arrivalTimeMap.AddPacket(44, 12)
		arrivalTimeMap.AddPacket(45, 13)
		arrivalTimeMap.AddPacket(largeSeqNum, 12)

		assert.Equal(t, int64(43), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, largeSeqNum+1, arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(41))
		assert.False(t, arrivalTimeMap.HasReceived(42))
		assert.True(t, arrivalTimeMap.HasReceived(43))
		assert.True(t, arrivalTimeMap.HasReceived(44))
		assert.True(t, arrivalTimeMap.HasReceived(45))
		assert.False(t, arrivalTimeMap.HasReceived(46))
		assert.True(t, arrivalTimeMap.HasReceived(largeSeqNum))
		assert.False(t, arrivalTimeMap.HasReceived(largeSeqNum+1))
	})

	t.Run("sequence number jump deletes all", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap

		var largeSeqNum int64 = 42 + 2*maxNumberOfPackets
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(largeSeqNum, 12)

		assert.Equal(t, largeSeqNum, arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, largeSeqNum+1, arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(42))
		assert.True(t, arrivalTimeMap.HasReceived(largeSeqNum))
		assert.False(t, arrivalTimeMap.HasReceived(largeSeqNum+1))
	})

	t.Run("expands before beginning", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(-1000, 13)
		assert.Equal(t, int64(-1000), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(43), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(-1001))
		assert.True(t, arrivalTimeMap.HasReceived(-1000))
		assert.False(t, arrivalTimeMap.HasReceived(-999))
		assert.True(t, arrivalTimeMap.HasReceived(42))
		assert.False(t, arrivalTimeMap.HasReceived(43))
	})

	t.Run("expanding before beginning keeps received", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap

		var smallSeqNum int64 = 42 - 2*maxNumberOfPackets
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(smallSeqNum, 13)

		assert.Equal(t, int64(42), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(43), arrivalTimeMap.EndSequenceNumber())
	})

	t.Run("erase to removes elements", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(43, 11)
		arrivalTimeMap.AddPacket(44, 12)
		arrivalTimeMap.AddPacket(45, 13)

		arrivalTimeMap.EraseTo(44)

		assert.Equal(t, int64(44), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(46), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(43))
		assert.True(t, arrivalTimeMap.HasReceived(44))
		assert.True(t, arrivalTimeMap.HasReceived(45))
		assert.False(t, arrivalTimeMap.HasReceived(46))
	})

	t.Run("erases in empty map", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap

		assert.Equal(t, arrivalTimeMap.BeginSequenceNumber(), arrivalTimeMap.EndSequenceNumber())

		arrivalTimeMap.EraseTo(arrivalTimeMap.EndSequenceNumber())
		assert.Equal(t, arrivalTimeMap.BeginSequenceNumber(), arrivalTimeMap.EndSequenceNumber())
	})

	t.Run("is tolerant to wrong arguments for erase", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(43, 11)

		arrivalTimeMap.EraseTo(1)

		assert.Equal(t, int64(42), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(44), arrivalTimeMap.EndSequenceNumber())

		arrivalTimeMap.EraseTo(100)

		assert.Equal(t, int64(44), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(44), arrivalTimeMap.EndSequenceNumber())
	})

	//nolint:dupl
	t.Run("erase all remembers beginning sequence number", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(42, 10)
		arrivalTimeMap.AddPacket(43, 11)
		arrivalTimeMap.AddPacket(44, 12)
		arrivalTimeMap.AddPacket(45, 13)

		arrivalTimeMap.EraseTo(46)
		arrivalTimeMap.AddPacket(50, 10)

		assert.Equal(t, int64(46), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(51), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(45))
		assert.False(t, arrivalTimeMap.HasReceived(46))
		assert.False(t, arrivalTimeMap.HasReceived(47))
		assert.False(t, arrivalTimeMap.HasReceived(48))
		assert.False(t, arrivalTimeMap.HasReceived(49))
		assert.True(t, arrivalTimeMap.HasReceived(50))
		assert.False(t, arrivalTimeMap.HasReceived(51))
	})

	//nolint:dupl
	t.Run("erase to missing sequence number", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(37, 10)
		arrivalTimeMap.AddPacket(39, 11)
		arrivalTimeMap.AddPacket(40, 12)
		arrivalTimeMap.AddPacket(41, 13)

		arrivalTimeMap.EraseTo(38)

		arrivalTimeMap.AddPacket(42, 40)

		assert.Equal(t, int64(38), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(43), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(37))
		assert.False(t, arrivalTimeMap.HasReceived(38))
		assert.True(t, arrivalTimeMap.HasReceived(39))
		assert.True(t, arrivalTimeMap.HasReceived(40))
		assert.True(t, arrivalTimeMap.HasReceived(41))
		assert.True(t, arrivalTimeMap.HasReceived(42))
		assert.False(t, arrivalTimeMap.HasReceived(43))
	})

	t.Run("remove old packets", func(t *testing.T) {
		var arrivalTimeMap packetArrivalTimeMap
		arrivalTimeMap.AddPacket(37, 10)
		arrivalTimeMap.AddPacket(39, 11)
		arrivalTimeMap.AddPacket(40, 12)
		arrivalTimeMap.AddPacket(41, 13)

		arrivalTimeMap.RemoveOldPackets(42, 11)

		assert.Equal(t, int64(40), arrivalTimeMap.BeginSequenceNumber())
		assert.Equal(t, int64(42), arrivalTimeMap.EndSequenceNumber())

		assert.False(t, arrivalTimeMap.HasReceived(39))
		assert.True(t, arrivalTimeMap.HasReceived(40))
		assert.True(t, arrivalTimeMap.HasReceived(41))
		assert.False(t, arrivalTimeMap.HasReceived(42))
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
