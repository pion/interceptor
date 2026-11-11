// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:cyclop,maintidx
func TestJitterBuffer(t *testing.T) {
	assert := assert.New(t)

	t.Run("Appends packets in order", func(*testing.T) {
		jb := New()
		assert.Equal(jb.lastSequence, uint16(0))
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

		assert.Equal(jb.lastSequence, uint16(5002))

		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5012, Timestamp: 512}, Payload: []byte{0x02}})

		assert.Equal(jb.stats.outOfOrderCount, uint32(1))
		assert.Equal(jb.playbackBuffer.Length(), uint16(3))
		assert.Equal(jb.lastSequence, uint16(5012))
	})
	t.Run("Appends packets and wraps", func(*testing.T) {
		jb := New(WithMinimumPacketCount(1))
		assert.Equal(jb.lastSequence, uint16(0))
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 65535, Timestamp: 500}, Payload: []byte{0x02}})

		assert.Equal(jb.lastSequence, uint16(65535))

		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 0, Timestamp: 512}, Payload: []byte{0x02}})

		assert.Equal(jb.playbackBuffer.Length(), uint16(2))
		assert.Equal(jb.lastSequence, uint16(0))

		head, err := jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(65535))
		assert.Equal(err, nil)
		head, err = jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(0))
		assert.Equal(err, nil)
	})

	t.Run("Appends packets and begins playout", func(*testing.T) {
		jb := New()
		for i := range 100 {
			jb.Push(
				&rtp.Packet{
					Header: rtp.Header{
						SequenceNumber: uint16(5012 + i), //nolint:gosec // G115
						Timestamp:      uint32(512 + i),  //nolint:gosec // G115
					},
					Payload: []byte{0x02},
				},
			)
		}
		assert.Equal(jb.playbackBuffer.Length(), uint16(100))
		assert.Equal(jb.state, Emitting)
		assert.Equal(jb.playoutHead, uint16(5012))
		head, err := jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(5012))
		assert.Equal(err, nil)
	})
	t.Run("Appends packets and begins playout", func(*testing.T) {
		jb := New(WithMinimumPacketCount(1))
		events := make([]Event, 0)
		jb.Listen(BeginPlayback, func(event Event, _ *JitterBuffer) {
			events = append(events, event)
		})
		for i := range 2 {
			//nolint:gosec // G115
			jb.Push(
				&rtp.Packet{
					Header: rtp.Header{
						SequenceNumber: uint16(5012 + i),
						Timestamp:      uint32(512 + i),
					},
					Payload: []byte{0x02},
				},
			)
		}
		assert.Equal(jb.playbackBuffer.Length(), uint16(2))
		assert.Equal(jb.state, Emitting)
		assert.Equal(jb.playoutHead, uint16(5012))
		head, err := jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(5012))
		assert.Equal(err, nil)
		assert.Equal(1, len(events))
		assert.Equal(Event(BeginPlayback), events[0])
	})

	t.Run("Wraps playout correctly", func(*testing.T) {
		jb := New()
		for i := range 100 {
			sqnum := uint16(math.MaxUint16 - 32 + i) //nolint:gosec // G115
			//nolint:gosec // G115
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		assert.Equal(jb.playbackBuffer.Length(), uint16(100))
		assert.Equal(jb.state, Emitting)
		assert.Equal(jb.playoutHead, uint16(math.MaxUint16-32))
		head, err := jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
		assert.Equal(err, nil)
		for i := range 100 {
			head, err := jb.Pop()
			if i < 99 {
				assert.Equal(head.SequenceNumber, uint16((math.MaxUint16 - 31 + i))) //nolint:gosec // G115
				assert.Equal(err, nil)
			} else {
				assert.Equal(head, (*rtp.Packet)(nil))
			}
		}
	})

	t.Run("Pops at timestamp correctly", func(*testing.T) {
		jb := New()
		for i := range 100 {
			sqnum := uint16((math.MaxUint16 - 32 + i)) //nolint:gosec // G115
			//nolint:gosec // G115
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		assert.Equal(jb.playbackBuffer.Length(), uint16(100))
		assert.Equal(jb.state, Emitting)
		// this will discard (one) ts=512 because it is older
		// and return the oldest ts=513
		head, err := jb.PopAtTimestamp(uint32(513))
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32+1))
		assert.Equal(err, nil)
		head, err = jb.PopAtTimestamp(uint32(513)) // 32+1
		assert.Equal(head, (*rtp.Packet)(nil))     // there is only one ts=513
		assert.NotEqual(err, nil)

		// PopAtTimestamp establishes a new playoutHead
		assert.Equal(jb.playoutHead, uint16(math.MaxUint16-32+2))
		head, err = jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32+2))
		assert.Equal(err, nil)
	})

	t.Run("Can peek at a packet", func(*testing.T) {
		jb := New()
		for i := range 100 {
			sqnum := uint16((math.MaxUint16 - 32 + i)) //nolint:gosec // G115
			//nolint:gosec // G115
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})
		pkt, err := jb.Peek(false)
		assert.Equal(pkt.SequenceNumber, uint16(5002))
		assert.Equal(err, nil)
		pkt, err = jb.Peek(true)

		assert.Equal(pkt.SequenceNumber, uint16(math.MaxUint16-32))
		assert.Equal(err, nil)
	})

	t.Run("Pops at sequence with an invalid sequence number", func(*testing.T) {
		jb := New()
		for i := range 50 {
			sqnum := uint16((math.MaxUint16 - 32 + i)) //nolint:gosec // G115
			//nolint:gosec // G115
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1019, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1020, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		assert.Equal(jb.playbackBuffer.Length(), uint16(50))
		assert.Equal(jb.state, Emitting)
		head, err := jb.PopAtSequence(uint16(9000))
		assert.Equal(head, (*rtp.Packet)(nil))
		assert.NotEqual(err, nil)
	})

	t.Run("Pops at timestamp with multiple packets", func(*testing.T) {
		jb := New()
		for i := range 50 {
			sqnum := uint16((math.MaxUint16 - 32 + i)) //nolint:gosec // G115
			//nolint:gosec // G115
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 17, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 18, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 19, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		assert.Equal(jb.playbackBuffer.Length(), uint16(53))
		assert.Equal(jb.state, Emitting)
		head, err := jb.PopAtTimestamp(uint32(9000))
		assert.Equal(head.SequenceNumber, uint16(17))
		assert.Equal(err, nil)
		head, err = jb.PopAtTimestamp(uint32(9000))
		assert.Equal(head.SequenceNumber, uint16(18))
		assert.Equal(err, nil)

		head, err = jb.Pop()
		assert.Equal(head.SequenceNumber, uint16(19))
		assert.Equal(err, nil)
	})

	t.Run("Peeks at timestamp with multiple packets", func(*testing.T) {
		jb := New()
		for i := range 50 {
			sqnum := uint16((math.MaxUint16 - 32 + i)) //nolint:gosec // G115
			//nolint:gosec // G115
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: sqnum, Timestamp: uint32(512 + i)}, Payload: []byte{0x02}})
		}
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1019, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1020, Timestamp: uint32(9000)}, Payload: []byte{0x02}})
		assert.Equal(jb.playbackBuffer.Length(), uint16(50))
		assert.Equal(jb.state, Emitting)
		head, err := jb.PeekAtSequence(uint16(1019))
		assert.Equal(head.SequenceNumber, uint16(1019))
		assert.Equal(err, nil)
		head, err = jb.PeekAtSequence(uint16(1020))
		assert.Equal(head.SequenceNumber, uint16(1020))
		assert.Equal(err, nil)

		head, err = jb.PopAtSequence(uint16(math.MaxUint16 - 32))
		assert.Equal(head.SequenceNumber, uint16(math.MaxUint16-32))
		assert.Equal(err, nil)
	})

	t.Run("SetPlayoutHead", func(*testing.T) {
		jb := New(WithMinimumPacketCount(1))

		// Push packets 0-9, but no packet 4
		for i := range uint16(10) {
			if i == 4 {
				continue
			}
			jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: i, Timestamp: uint32(512 + i)}, Payload: []byte{0x00}})
		}

		// The first 3 packets will be able to popped
		for range 4 {
			pkt, err := jb.Pop()
			assert.NoError(err)
			assert.NotNil(pkt)
		}

		// The next pop will fail because of gap
		pkt, err := jb.Pop()
		assert.ErrorIs(err, ErrNotFound)
		assert.Nil(pkt)
		assert.Equal(jb.PlayoutHead(), uint16(4))

		// Assert that PlayoutHead isn't modified with pushing/popping again
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 10, Timestamp: uint32(522)}, Payload: []byte{0x00}})
		pkt, err = jb.Pop()
		assert.ErrorIs(err, ErrNotFound)
		assert.Nil(pkt)
		assert.Equal(jb.PlayoutHead(), uint16(4))

		// Increment the PlayoutHead and popping will work again
		jb.SetPlayoutHead(jb.PlayoutHead() + 1)
		for range 6 {
			pkt, err := jb.Pop()
			assert.NoError(err)
			assert.NotNil(pkt)
		}
	})

	t.Run("Allows clearing the buffer", func(*testing.T) {
		jb := New()
		jb.Clear(false)

		assert.Equal(jb.lastSequence, uint16(0))
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
		jb.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

		assert.Equal(jb.lastSequence, uint16(5002))
		jb.Clear(true)
		assert.Equal(jb.lastSequence, uint16(0))
		assert.Equal(jb.stats.outOfOrderCount, uint32(0))
		assert.Equal(jb.playbackBuffer.Length(), uint16(0))
	})
}

func seqPacket(seq uint16, marker byte) *rtp.Packet {
	return &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: seq, CSRC: []uint32{uint32(seq)}},
		Payload: []byte{byte(seq >> 8), byte(seq & 0xff), marker},
	}
}

func pushRange(jb *JitterBuffer, start uint16, count int) []*rtp.Packet {
	pkts := make([]*rtp.Packet, count)
	for i := range pkts {
		pkts[i] = seqPacket(start+uint16(i), 0) //nolint:gosec // G115, test counts fit uint16
		jb.Push(pkts[i])
	}

	return pkts
}

func TestJitterBufferRemoveThenRePush(t *testing.T) {
	for _, start := range []uint16{0, 1000, 65534} {
		for _, offset := range []uint16{0, 2, 4} { // oldest, middle, highest
			jb := New(WithMinimumPacketCount(1))
			pushRange(jb, start, 5)
			seq := start + offset

			removed, err := jb.RemoveAtSequence(seq)
			require.NoError(t, err)
			require.NotNil(t, removed)

			again := seqPacket(seq, 1)
			jb.Push(again)
			got, err := jb.PeekAtSequence(seq)
			require.NoError(t, err, "start %d offset %d", start, offset)
			assert.Same(t, again, got, "start %d offset %d", start, offset)
		}
	}
}

func TestJitterBufferDuplicatePushKeepsFirst(t *testing.T) {
	jb := New(WithMinimumPacketCount(1))
	pkts := pushRange(jb, 10, 3)
	for _, pkt := range pkts {
		jb.Push(seqPacket(pkt.SequenceNumber, 1))
		got, err := jb.PeekAtSequence(pkt.SequenceNumber)
		require.NoError(t, err)
		assert.Same(t, pkt, got)
	}
}

func TestJitterBufferRemoveIgnoresPlayoutState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		minStart uint16
		state    State
	}{
		{"buffering", 50, Buffering},
		{"emitting", 1, Emitting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jb := New(WithMinimumPacketCount(tc.minStart))
			pkts := pushRange(jb, 100, 3)
			require.Equal(t, tc.state, jb.state)

			got, err := jb.PeekAtSequence(101)
			require.NoError(t, err)
			assert.Same(t, pkts[1], got)

			got, err = jb.RemoveAtSequence(101)
			require.NoError(t, err)
			assert.Same(t, pkts[1], got)

			_, err = jb.PeekAtSequence(101)
			require.ErrorIs(t, err, ErrNotFound)
			_, err = jb.RemoveAtSequence(101)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestJitterBufferRemoveAdvancesPlayout(t *testing.T) {
	for _, tc := range []struct {
		name       string
		remove     uint16
		nextPop    uint16
		queueAfter uint16
	}{
		{"queue head", 20, 21, 4},
		{"inside queue", 22, 23, 2},
		{"queue tail", 24, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jb := New(WithMinimumPacketCount(1))
			pushRange(jb, 20, 5)
			require.Equal(t, uint16(5), jb.playbackBuffer.Length())

			_, err := jb.RemoveAtSequence(tc.remove)
			require.NoError(t, err)
			assert.Equal(t, tc.queueAfter, jb.playbackBuffer.Length())

			pkt, err := jb.Pop()
			if tc.queueAfter == 0 {
				require.ErrorIs(t, err, ErrNotFound)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.nextPop, pkt.SequenceNumber)

			// Packets skipped in playback are still held until removed.
			for seq := uint16(20); seq < tc.remove; seq++ {
				_, err = jb.PeekAtSequence(seq)
				assert.NoError(t, err, "seq %d", seq)
			}
		})
	}
}

func TestJitterBufferRemoveAfterGap(t *testing.T) {
	jb := New(WithMinimumPacketCount(1))
	pushRange(jb, 1, 2) // 1, 2 are playable
	late := seqPacket(5, 0)
	jb.Push(late) // 3, 4 missing
	require.Equal(t, uint16(2), jb.playbackBuffer.Length())

	got, err := jb.RemoveAtSequence(5)
	require.NoError(t, err)
	assert.Same(t, late, got)
	// 1 and 2 precede 5, so they leave the playback queue too.
	assert.Equal(t, uint16(0), jb.playbackBuffer.Length())
	_, err = jb.PeekAtSequence(1)
	assert.NoError(t, err)
}

func TestJitterBufferRemoveFindsQueuedPacketAfterSlotReuse(t *testing.T) {
	for _, copyMode := range []bool{false, true} {
		opts := []Option{WithSize(2), WithMinimumPacketCount(1)}
		if copyMode {
			opts = append(opts, WithCopy())
		}
		jb := New(opts...)
		pkts := pushRange(jb, 1, 3) // 3 overwrites 1's slot; 1 and 2 stay queued

		peeked, err := jb.PeekAtSequence(1)
		require.NoError(t, err, "copy %v", copyMode)
		assert.Equal(t, pkts[0].Payload, peeked.Payload, "copy %v", copyMode)

		got, err := jb.RemoveAtSequence(1)
		require.NoError(t, err, "copy %v", copyMode)
		assert.Equal(t, pkts[0].Payload, got.Payload, "copy %v", copyMode)
		if !copyMode {
			assert.Same(t, pkts[0], got)
		}

		got, err = jb.Pop()
		require.NoError(t, err, "copy %v", copyMode)
		assert.Equal(t, uint16(2), got.SequenceNumber, "copy %v", copyMode)
	}
}

func TestJitterBufferPlayoutHeadTracksQueueAfterRemove(t *testing.T) {
	jb := New(WithMinimumPacketCount(1))
	pushRange(jb, 10, 3)
	require.Equal(t, uint16(10), jb.PlayoutHead())

	_, err := jb.RemoveAtSequence(11) // discards queued 10 and 11
	require.NoError(t, err)
	assert.Equal(t, uint16(12), jb.PlayoutHead())

	got, err := jb.Pop()
	require.NoError(t, err)
	assert.Equal(t, uint16(12), got.SequenceNumber)
	assert.Equal(t, uint16(13), jb.PlayoutHead())
}

func TestJitterBufferSmallBufferResumesAfterClear(t *testing.T) {
	jb := New(WithSize(8), WithMinimumPacketCount(1))
	jb.Push(seqPacket(0, 0))
	_, err := jb.Pop()
	require.NoError(t, err)

	jb.Clear(true)
	for seq := uint16(100); seq < 120; seq++ {
		jb.Push(seqPacket(seq, 0))
		got, err := jb.Pop()
		require.NoError(t, err, "seq %d", seq)
		assert.Equal(t, seq, got.SequenceNumber)
	}
}

func TestJitterBufferRemoveResumesPlayoutAfterRemovedSequence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove uint16
		found  bool
	}{
		// 2 is missing, so drain is stopped there when it is removed.
		{"missing sequence at the drain point", 2, false},
		// 3 is buffered but not yet playable behind the gap at 2.
		{"buffered sequence beyond the drain point", 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jb := New(WithMinimumPacketCount(1))
			jb.Push(seqPacket(1, 0))
			jb.Push(seqPacket(3, 0))
			jb.Push(seqPacket(4, 0))

			_, err := jb.RemoveAtSequence(tc.remove)
			assert.Equal(t, tc.found, err == nil)
			assert.Equal(t, tc.remove+1, jb.PlayoutHead())

			got, err := jb.Pop()
			require.NoError(t, err, "playout must resume after the removed sequence")
			assert.Equal(t, tc.remove+1, got.SequenceNumber)
		})
	}
}

func TestJitterBufferWindowAcrossWrap(t *testing.T) {
	const size = 4
	for _, highest := range []uint16{65534, 65535, 0, 1, 2} {
		jb := New(WithMinimumPacketCount(1), WithSize(size))
		pushRange(jb, highest-(size-1), size)

		for _, tc := range []struct {
			seq   uint16
			found bool
		}{
			{highest, true},
			{highest - (size - 1), true},
			{highest - size, false},
			{highest + 1, false},
		} {
			_, err := jb.PeekAtSequence(tc.seq)
			assert.Equal(t, tc.found, err == nil, "highest %d seq %d", highest, tc.seq)
		}
	}
}

func TestJitterBufferHalfSpaceJumpKeepsBufferedPackets(t *testing.T) {
	jb := New(WithMinimumPacketCount(1))
	pkts := pushRange(jb, 100, 3)
	jb.Push(seqPacket(102+0x8000, 0))
	for _, pkt := range pkts {
		got, err := jb.PeekAtSequence(pkt.SequenceNumber)
		require.NoError(t, err)
		assert.Same(t, pkt, got)
	}
}

func TestJitterBufferStartThresholdFitsCapacity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		opts      []Option
		threshold uint16 // packets needed before Pop succeeds
	}{
		{"default threshold above small size", []Option{WithSize(16)}, 16},
		{"explicit threshold above size", []Option{WithSize(4), WithMinimumPacketCount(10)}, 4},
		{"option order does not matter", []Option{WithMinimumPacketCount(10), WithSize(4)}, 4},
		{"threshold within size is kept", []Option{WithSize(16), WithMinimumPacketCount(3)}, 3},
		{"default size and threshold", nil, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jb := New(tc.opts...)
			for cycle := range 2 { // Clear(true) must restore the same threshold
				pushRange(jb, 0, int(tc.threshold)-1)
				_, err := jb.Pop()
				require.ErrorIs(t, err, ErrPopWhileBuffering, "cycle %d: below threshold", cycle)

				jb.Push(seqPacket(tc.threshold-1, 0))
				got, err := jb.Pop()
				require.NoError(t, err, "cycle %d: threshold reached", cycle)
				assert.Equal(t, uint16(0), got.SequenceNumber)

				jb.Clear(true)
			}
		})
	}
}

func TestJitterBufferWithSize(t *testing.T) {
	for _, tc := range []struct {
		size, want uint16
	}{
		{0, 1024},
		{1, 1},
		{3, 4},
		{1024, 1024},
		{1025, 2048},
		{32768, 32768},
		{32769, 32768},
		{65535, 32768},
	} {
		jb := New(WithMinimumPacketCount(1), WithSize(tc.size))
		pushRange(jb, 0, int(tc.want)+1)
		// Empty the playback queue so the peeks below only see the window.
		for range tc.want {
			_, err := jb.Pop()
			require.NoError(t, err)
		}

		_, err := jb.PeekAtSequence(0)
		assert.ErrorIs(t, err, ErrNotFound, "size %d: window must hold %d", tc.size, tc.want)
		_, err = jb.PeekAtSequence(1)
		assert.NoError(t, err, "size %d: window must hold %d", tc.size, tc.want)
	}
}

func TestJitterBufferWithoutCopyReturnsCallerPacket(t *testing.T) {
	for _, opts := range [][]Option{nil, {WithCopy(), DisableCopy()}} {
		jb := New(append(opts, WithMinimumPacketCount(1))...)
		pkts := pushRange(jb, 7, 2)

		got, err := jb.PeekAtSequence(7)
		require.NoError(t, err)
		assert.Same(t, pkts[0], got)

		got, err = jb.Pop()
		require.NoError(t, err)
		assert.Same(t, pkts[0], got)

		got, err = jb.RemoveAtSequence(8)
		require.NoError(t, err)
		assert.Same(t, pkts[1], got)
	}
}

func TestJitterBufferWithCopyIsolatesCallerMemory(t *testing.T) {
	jb := New(WithCopy(), WithMinimumPacketCount(1))
	pkts := pushRange(jb, 7, 3)
	for _, pkt := range pkts {
		pkt.Payload[2] = 0xEE
		pkt.CSRC[0] = 0xEE
	}

	check := func(op string, got *rtp.Packet, err error, seq uint16) {
		t.Helper()
		require.NoError(t, err, op)
		want := seqPacket(seq, 0)
		assert.Equal(t, want.Payload, got.Payload, op)
		assert.Equal(t, want.CSRC, got.CSRC, op)
	}

	got, err := jb.PeekAtSequence(7)
	check("peek", got, err, 7)
	got, err = jb.Pop()
	check("pop", got, err, 7)
	got, err = jb.RemoveAtSequence(9)
	check("remove", got, err, 9)
}

func TestJitterBufferWithCopyResultsSurviveBufferReuse(t *testing.T) {
	jb := New(WithCopy(), WithMinimumPacketCount(1), WithSize(4))
	pushRange(jb, 0, 4)

	peeked, err := jb.PeekAtSequence(0)
	require.NoError(t, err)
	removed, err := jb.RemoveAtSequence(1)
	require.NoError(t, err)

	// Evict everything so the pooled buffers backing 0 and 1 are reused.
	for seq := uint16(100); seq < 140; seq++ {
		jb.Push(&rtp.Packet{
			Header:  rtp.Header{SequenceNumber: seq, CSRC: []uint32{0xFFFFFFFF}},
			Payload: bytes.Repeat([]byte{0xFF}, 3),
		})
	}

	assert.Equal(t, seqPacket(0, 0).Payload, peeked.Payload)
	assert.Equal(t, []uint32{0}, peeked.CSRC)
	assert.Equal(t, seqPacket(1, 0).Payload, removed.Payload)
	assert.Equal(t, []uint32{1}, removed.CSRC)
}

func TestJitterBufferWithCopyDropsOversizedPayload(t *testing.T) {
	big := &rtp.Packet{Header: rtp.Header{SequenceNumber: 1}, Payload: make([]byte, 1461)}
	fits := &rtp.Packet{Header: rtp.Header{SequenceNumber: 2}, Payload: make([]byte, 1460)}

	jb := New(WithCopy())
	jb.Push(big)
	jb.Push(fits)
	_, err := jb.PeekAtSequence(1)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = jb.PeekAtSequence(2)
	assert.NoError(t, err)

	jb = New()
	jb.Push(big)
	_, err = jb.PeekAtSequence(1)
	assert.NoError(t, err, "without copying any payload size is kept")
}

// jbModel is the reference behavior of the JitterBuffer's sequence-indexed
// store: the newest sequence number seen defines a window of size entries,
// duplicates keep the first packet, and packets older than the window are
// dropped.
type jbModel struct {
	size    uint16
	started bool
	highest uint16
	pkts    map[uint16]jbEntry
}

type jbEntry struct {
	orig    *rtp.Packet
	payload []byte // payload as pushed, before the caller scribbles on it
}

func (m *jbModel) push(seq uint16, entry jbEntry) {
	if !m.started {
		m.started, m.highest = true, seq
		m.pkts[seq] = entry

		return
	}
	switch diff := seq - m.highest; {
	case diff == 0:
	case diff < 0x8000:
		m.highest = seq
		for q := range m.pkts {
			if m.highest-q >= m.size {
				delete(m.pkts, q)
			}
		}
	case m.highest-seq >= m.size:
		return
	}
	if _, ok := m.pkts[seq]; !ok {
		m.pkts[seq] = entry
	}
}

// fuzzPayload encodes the sequence number so any returned packet can be
// checked against its header, plus a push id to tell duplicates apart.
func fuzzPayload(seq uint16, id byte) []byte {
	return []byte{byte(seq >> 8), byte(seq & 0xff), id}
}

type fuzzReturned struct {
	pkt  *rtp.Packet
	want []byte
}

// FuzzJitterBuffer drives Push/Peek/Remove/Pop/Clear from fuzz input and
// checks the JitterBuffer against jbModel after every step.
//
// Input layout: flags, base sequence (2 bytes), then 2-byte operations.
//
//nolint:gocognit,cyclop,maintidx
func FuzzJitterBuffer(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x00, 0, 0, 0, 1, 0, 2, 4, 0, 2, 1, 4, 0, 0, 2, 4, 0})
	f.Add([]byte{0x01, 0xff, 0xf0, 0, 0, 0, 10, 0, 20, 2, 10, 0, 10, 4, 0, 3, 10})
	f.Add([]byte{0x47, 0x80, 0x00, 0, 0, 0, 1, 6, 0, 0, 1, 3, 0, 3, 1, 7, 0, 0, 5, 4, 0})
	f.Add([]byte{0x0b, 0x12, 0x34, 0, 0, 0, 1, 0, 2, 5, 100, 0, 0, 3, 0, 2, 0, 0, 0})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		copyMode := data[0]&1 == 1
		size := uint16(1) << ((data[0] >> 1) % 11)   // 1..1024
		minStart := uint16(data[0]>>6) + 1           // 1..4
		base := uint16(data[1])<<8 | uint16(data[2]) //nolint:gosec // G115
		data = data[3:]

		opts := []Option{WithSize(size), WithMinimumPacketCount(minStart)}
		if copyMode {
			opts = append(opts, WithCopy())
		}
		jb := New(opts...)
		model := &jbModel{size: size, pkts: map[uint16]jbEntry{}}
		pushed := map[*rtp.Packet]bool{}
		var returned []fuzzReturned

		var lastPop uint16
		havePop := false
		// removedSincePop breaks contiguity between pops. popFloor is the latest
		// removal that advanced playout (was at or past the drain point): the
		// next Pop must come after it. Removals behind the drain point, or more
		// than half the sequence space away, consume nothing new.
		removedSincePop := false
		var popFloor uint16
		haveFloor := false
		var pushID byte
		// mustPop is set only by the operation immediately before.
		var mustPop bool

		checkReturned := func(op string, seq uint16, got *rtp.Packet) {
			t.Helper()
			require.Equalf(t, seq, got.SequenceNumber, "%s(%d) header", op, seq)
			require.Lenf(t, got.Payload, 3, "%s(%d) payload", op, seq)
			require.Equalf(t, seq, uint16(got.Payload[0])<<8|uint16(got.Payload[1]), "%s(%d) payload", op, seq)
			if copyMode {
				require.Falsef(t, pushed[got], "%s(%d) returned the caller's packet in copy mode", op, seq)
			} else {
				require.Truef(t, pushed[got], "%s(%d) returned a packet that was never pushed", op, seq)
			}
			returned = append(returned, fuzzReturned{got, append([]byte(nil), got.Payload...)})
		}

		checkAgainstModel := func(op string, seq uint16, got *rtp.Packet, err error) {
			t.Helper()
			entry, ok := model.pkts[seq]
			if !ok {
				require.ErrorIsf(t, err, ErrNotFound, "%s(%d) returned %v", op, seq, got)

				return
			}
			require.NoErrorf(t, err, "%s(%d)", op, seq)
			checkReturned(op, seq, got)
			if !copyMode {
				require.Samef(t, entry.orig, got, "%s(%d) returned a different packet than the one buffered", op, seq)
			}
			require.Equalf(t, entry.payload, got.Payload, "%s(%d) payload", op, seq)
		}

		// queuedAt reports whether seq is still waiting in the playback queue,
		// which can outlive the window when newer packets reuse its slot.
		queuedAt := func(seq uint16) bool {
			queue := jb.playbackBuffer
			for i := range queue.length {
				if queue.buffer[(queue.read+i)%queue.size].Header().SequenceNumber == seq {
					return true
				}
			}

			return false
		}

		// checkLookup checks a Peek/RemoveAtSequence result. queued must be
		// sampled before the call, since removal changes the queue.
		checkLookup := func(op string, seq uint16, got *rtp.Packet, err error, queued bool) {
			t.Helper()
			if _, inWindow := model.pkts[seq]; !inWindow && queued {
				require.NoErrorf(t, err, "%s(%d) of a packet still queued for playout", op, seq)
				checkReturned(op, seq, got)

				return
			}
			checkAgainstModel(op, seq, got, err)
		}

		// PlayoutHead names the next packet Pop would return: the queue head, or
		// with nothing queued, the next sequence drain will move in.
		checkPlayoutHead := func() {
			t.Helper()
			if head := jb.playbackBuffer.Peek(); head != nil {
				require.Equal(t, head.Header().SequenceNumber, jb.PlayoutHead(), "PlayoutHead vs queue head")
			} else if model.started {
				require.Equal(t, jb.expectedSequence, jb.PlayoutHead(), "PlayoutHead vs drain point")
			}
		}

		for ; len(data) >= 2; data = data[2:] {
			checkPlayoutHead()
			arg := int8(data[1])             //nolint:gosec // G115
			seq := base + uint16(int16(arg)) //nolint:gosec // G115

			switch data[0] % 8 {
			case 0, 1:
				pushID++
				pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: seq}, Payload: fuzzPayload(seq, pushID)}
				pushed[pkt] = true
				entry := jbEntry{orig: pkt, payload: append([]byte(nil), pkt.Payload...)}
				if !model.started {
					// The first packet after Clear restarts playout from itself.
					haveFloor = false
				}
				jb.Push(pkt)
				model.push(seq, entry)
				if copyMode {
					// The buffer must not depend on the caller's memory.
					for i := range pkt.Payload {
						pkt.Payload[i] = 0xEE
					}
				}
			case 2:
				advancesPlayout := int16(seq-jb.expectedSequence) >= 0 //nolint:gosec // G115
				queued := queuedAt(seq)
				got, err := jb.RemoveAtSequence(seq)
				checkLookup("RemoveAtSequence", seq, got, err, queued)
				delete(model.pkts, seq)
				removedSincePop = true
				if advancesPlayout {
					popFloor, haveFloor = seq, true
				}
				// A removal at or past the drain point consumes playout through
				// seq, so a buffered seq+1 must be poppable straight away. A late
				// packet behind the drain point stays unplayable, as after a Pop.
				_, mustPop = model.pkts[seq+1]
				mustPop = mustPop && advancesPlayout && jb.state == Emitting

				continue
			case 3:
				got, err := jb.PeekAtSequence(seq)
				checkLookup("PeekAtSequence", seq, got, err, queuedAt(seq))
			case 4:
				got, err := jb.Pop()
				if mustPop {
					require.NoErrorf(t, err, "Pop stalled after RemoveAtSequence with the next sequence buffered")
				}
				if err != nil {
					require.Truef(t, errors.Is(err, ErrNotFound) || errors.Is(err, ErrPopWhileBuffering),
						"Pop: unexpected error %v", err)

					break
				}
				checkReturned("Pop", got.SequenceNumber, got)
				// Removals may skip playout arbitrarily far, so ordering across them
				// is checked against popFloor rather than lastPop.
				if havePop && !removedSincePop {
					require.Equalf(t, lastPop+1, got.SequenceNumber, "Pop after %d with no removal in between", lastPop)
				}
				if haveFloor {
					require.Positivef(t, int16(got.SequenceNumber-popFloor), //nolint:gosec // G115
						"Pop returned %d after RemoveAtSequence(%d)", got.SequenceNumber, popFloor)
				}
				lastPop, havePop, removedSincePop, haveFloor = got.SequenceNumber, true, false, false
			case 5:
				base += uint16(int16(arg)) * 97 //nolint:gosec // G115
			case 6:
				base += 0x8000 + uint16(int16(arg)) //nolint:gosec // G115
			case 7:
				jb.Clear(false)
				model.started = false
				model.pkts = map[uint16]jbEntry{}
				havePop, removedSincePop, haveFloor = false, false, false
			}
			mustPop = false
		}
		checkPlayoutHead()

		// Every sequence number near the window agrees with the model.
		if model.started {
			for d := -int(size) - 2; d <= 2; d++ {
				seq := model.highest + uint16(int16(d)) //nolint:gosec // G115
				got, err := jb.PeekAtSequence(seq)
				checkLookup("final PeekAtSequence", seq, got, err, queuedAt(seq))
			}
		}

		// Packets handed out earlier are unaffected by later buffer reuse.
		for _, r := range returned {
			require.Equalf(t, r.want, r.pkt.Payload, "returned packet %d changed", r.pkt.SequenceNumber)
		}
	})
}
