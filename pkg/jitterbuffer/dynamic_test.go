// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/pion/interceptor/internal/rtpbuffer"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBWE struct{ bitrate int }

func (f *fakeBWE) GetTargetBitrate() int { return f.bitrate }

const dynPacketSize = 1000

func dynPacket(seq uint16) *rtp.Packet {
	return &rtp.Packet{
		Header:  rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: uint32(seq) * 90},
		Payload: make([]byte, dynPacketSize-12),
	}
}

func withMockClock(jb *JitterBuffer) *test.MockTime {
	clock := &test.MockTime{}
	clock.SetNow(time.Unix(0, 0))
	jb.dynamic.now = clock.Now

	return clock
}

func pushSeqs(jb *JitterBuffer, seqs ...uint16) {
	for _, seq := range seqs {
		jb.Push(dynPacket(seq))
	}
}

func popSeqs(t *testing.T, jb *JitterBuffer, want ...uint16) {
	t.Helper()
	for _, seq := range want {
		pkt, err := jb.Pop()
		require.NoError(t, err)
		require.Equal(t, seq, pkt.SequenceNumber)
	}
}

// underflowCycle pushes count packets from seq, pops them all, then pops once
// more at the given offset from time zero. It returns the next sequence number.
func underflowCycle(t *testing.T, jb *JitterBuffer, clock *test.MockTime, seq, count uint16, at time.Duration) uint16 {
	t.Helper()
	for i := range count {
		jb.Push(dynPacket(seq + i))
	}
	require.Equal(t, Emitting, jb.state)
	for i := range count {
		popSeqs(t, jb, seq+i)
	}
	clock.SetNow(time.Unix(0, 0).Add(at))
	_, err := jb.Pop()
	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, Buffering, jb.state)

	return seq + count
}

func TestDynamicLengthBandwidthSizing(t *testing.T) {
	assert.Equal(t, dynPacketSize, dynPacket(0).MarshalSize())

	for _, tc := range []struct {
		bitrate  int
		max      uint16
		expected uint16
	}{
		{500_000, 0, 7},
		{1_000_000, 0, 13},
		{4_000_000, 0, 50},
		{0, 0, 50},
		{100_000_000, 256, 256},
		{1_000_000_000, 0, 1024},
	} {
		jb := New(
			WithDynamicLength(DynamicLengthConfig{MinPackets: 1, MaxPackets: tc.max, TargetDelay: 100 * time.Millisecond}),
			WithBandwidthEstimator(&fakeBWE{tc.bitrate}),
		)
		jb.Push(dynPacket(0))
		assert.Equal(t, tc.expected, jb.minStartCount, "bitrate %d", tc.bitrate)
	}

	t.Run("bitrate change while buffering", func(t *testing.T) {
		bwe := &fakeBWE{1_000_000}
		jb := New(WithBandwidthEstimator(bwe))
		jb.Push(dynPacket(0))
		assert.Equal(t, uint16(13), jb.minStartCount)

		bwe.bitrate = 4_000_000
		for seq := range uint16(49) {
			jb.Push(dynPacket(seq + 1))
			if seq < 48 {
				assert.Equal(t, Buffering, jb.state)
			}
		}
		assert.Equal(t, uint16(50), jb.minStartCount)
		assert.Equal(t, Emitting, jb.state)
	})

	t.Run("bitrate change while emitting", func(t *testing.T) {
		bwe := &fakeBWE{1_000_000}
		jb := New(WithBandwidthEstimator(bwe))
		for seq := range uint16(13) {
			jb.Push(dynPacket(seq))
		}
		require.Equal(t, Emitting, jb.state)

		bwe.bitrate = 4_000_000
		jb.Push(dynPacket(13))
		assert.Equal(t, uint16(13), jb.minStartCount)
		assert.Equal(t, Emitting, jb.state)
	})
}

func newGrowthBuffer(cfg DynamicLengthConfig) (*JitterBuffer, *test.MockTime) {
	jb := New(WithMinimumPacketCount(4), WithDynamicLength(cfg))

	return jb, withMockClock(jb)
}

func TestDynamicLengthUnderflowGrowth(t *testing.T) {
	cfg := DynamicLengthConfig{UnderflowsPerMinute: 3, StepPackets: 5}

	t.Run("growth", func(t *testing.T) {
		jb, clock := newGrowthBuffer(cfg)
		seq := underflowCycle(t, jb, clock, 0, 4, 0)
		assert.Equal(t, uint16(4), jb.minStartCount)
		seq = underflowCycle(t, jb, clock, seq, 4, 10*time.Second)
		assert.Equal(t, uint16(4), jb.minStartCount)
		seq = underflowCycle(t, jb, clock, seq, 4, 20*time.Second)
		assert.Equal(t, uint16(9), jb.minStartCount)

		for i := range uint16(8) {
			jb.Push(dynPacket(seq + i))
		}
		assert.Equal(t, Buffering, jb.state)
		jb.Push(dynPacket(seq + 8))
		assert.Equal(t, Emitting, jb.state)
	})

	t.Run("window expiry", func(t *testing.T) {
		jb, clock := newGrowthBuffer(cfg)
		seq := uint16(0)
		for _, at := range []time.Duration{0, 30 * time.Second, 61 * time.Second} {
			seq = underflowCycle(t, jb, clock, seq, 4, at)
		}
		assert.Equal(t, uint16(4), jb.minStartCount)
		underflowCycle(t, jb, clock, seq, 4, 62*time.Second)
		assert.Equal(t, uint16(9), jb.minStartCount)
	})

	t.Run("cap", func(t *testing.T) {
		jb, clock := newGrowthBuffer(DynamicLengthConfig{MaxPackets: 10, UnderflowsPerMinute: 1, StepPackets: 5})
		seq := uint16(0)
		for _, want := range []uint16{9, 10, 10} {
			seq = underflowCycle(t, jb, clock, seq, jb.minStartCount, 0)
			assert.Equal(t, want, jb.minStartCount)
		}
	})

	t.Run("PopAtTimestamp drain", func(t *testing.T) {
		jb := New(WithMinimumPacketCount(3), WithDynamicLength(cfg))
		pushSeqs(jb, 0, 1, 2, 3, 4)
		_, err := jb.PopAtTimestamp(0)
		require.NoError(t, err)
		_, err = jb.PopAtTimestamp(0)
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, Emitting, jb.state)
		assert.Equal(t, uint32(1), jb.stats.underflowCount)
		assert.Empty(t, jb.dynamic.underflows)
	})

	t.Run("non-dynamic buffer", func(t *testing.T) {
		jb := New(WithMinimumPacketCount(1))
		pushSeqs(jb, 0)
		popSeqs(t, jb, 0)
		_, err := jb.Pop()
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, Emitting, jb.state)
	})
}

func TestDynamicLengthLossSkip(t *testing.T) {
	newBuffer := func(minCount uint16, opts ...Option) *JitterBuffer {
		return New(append([]Option{WithMinimumPacketCount(minCount), WithDynamicLength(DynamicLengthConfig{})}, opts...)...)
	}

	t.Run("skip while emitting", func(t *testing.T) {
		jb := newBuffer(3)
		pushSeqs(jb, 0, 1, 2, 4, 5)
		popSeqs(t, jb, 0, 1, 2, 4)
		assert.Equal(t, uint32(1), jb.stats.skippedCount)
		assert.Equal(t, uint32(0), jb.stats.underflowCount)

		pushSeqs(jb, 3)
		popSeqs(t, jb, 5)
		assert.Equal(t, uint32(2), jb.stats.outOfOrderCount)
		_, err := jb.Pop()
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, Buffering, jb.state)
		assert.Equal(t, uint32(1), jb.stats.underflowCount)
	})

	t.Run("skip while buffering", func(t *testing.T) {
		jb := newBuffer(4)
		pushSeqs(jb, 0, 2, 3, 4)
		require.Equal(t, Emitting, jb.state)
		popSeqs(t, jb, 0, 2, 3, 4)
	})

	t.Run("consecutive losses", func(t *testing.T) {
		jb := newBuffer(3)
		pushSeqs(jb, 0, 1, 2, 6)
		popSeqs(t, jb, 0, 1, 2, 6)
		assert.Equal(t, uint32(3), jb.stats.skippedCount)
	})

	t.Run("duplicates", func(t *testing.T) {
		jb := newBuffer(3)
		pushSeqs(jb, 0, 1, 1, 2)
		popSeqs(t, jb, 0, 1, 2)
		assert.Equal(t, uint32(0), jb.stats.skippedCount)
	})

	t.Run("wraparound", func(t *testing.T) {
		jb := newBuffer(3)
		pushSeqs(jb, 65534, 65535, 1, 2)
		popSeqs(t, jb, 65534, 65535, 1, 2)
		assert.Equal(t, uint32(1), jb.stats.skippedCount)
	})

	t.Run("gap beyond buffer size", func(t *testing.T) {
		jb := newBuffer(1, WithSize(16))
		pushSeqs(jb, 0)
		popSeqs(t, jb, 0)
		pushSeqs(jb, 20)
		popSeqs(t, jb, 20)
		assert.Equal(t, uint32(19), jb.stats.skippedCount)
	})

	t.Run("gap at half the sequence space", func(t *testing.T) {
		jb := newBuffer(1)
		pushSeqs(jb, 0)
		popSeqs(t, jb, 0)
		pushSeqs(jb, 30000, rtpbuffer.Uint16SizeHalf)
		popSeqs(t, jb, rtpbuffer.Uint16SizeHalf)
		assert.Equal(t, uint32(rtpbuffer.Uint16SizeHalf-1), jb.stats.skippedCount)

		jb = newBuffer(1)
		pushSeqs(jb, 0)
		popSeqs(t, jb, 0)
		pushSeqs(jb, 30000, rtpbuffer.Uint16SizeHalf+1)
		_, err := jb.Pop()
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, uint32(0), jb.stats.skippedCount)
	})
}

func TestDynamicLengthConfig(t *testing.T) {
	t.Run("estimator alone uses defaults", func(t *testing.T) {
		jb := New(WithBandwidthEstimator(&fakeBWE{1_000_000}))
		require.NotNil(t, jb.dynamic)
		assert.Equal(t, DynamicLengthConfig{
			MinPackets:          defaultDynamicMinPackets,
			MaxPackets:          1024,
			StepPackets:         defaultDynamicStepPackets,
			UnderflowsPerMinute: defaultUnderflowsPerMinute,
			TargetDelay:         defaultTargetDelay,
		}, jb.dynamic.cfg)
		assert.Equal(t, uint16(50), jb.minStartCount)
		pushSeqs(jb, 0)
		assert.Equal(t, uint16(13), jb.minStartCount)
	})

	t.Run("config alone uses static count", func(t *testing.T) {
		jb := New(WithMinimumPacketCount(7), WithDynamicLength(DynamicLengthConfig{}))
		pushSeqs(jb, 0)
		assert.Equal(t, uint16(7), jb.minStartCount)
	})

	t.Run("clamping", func(t *testing.T) {
		jb := New(WithDynamicLength(DynamicLengthConfig{MinPackets: 20, MaxPackets: 10}))
		assert.Equal(t, uint16(10), jb.dynamic.cfg.MinPackets)

		jb = New(WithSize(64), WithDynamicLength(DynamicLengthConfig{MaxPackets: 500}))
		assert.Equal(t, uint16(64), jb.dynamic.cfg.MaxPackets)
	})

	t.Run("option order", func(t *testing.T) {
		cfg := DynamicLengthConfig{TargetDelay: 200 * time.Millisecond}
		bwe := &fakeBWE{1_000_000}
		a := New(WithDynamicLength(cfg), WithBandwidthEstimator(bwe))
		b := New(WithBandwidthEstimator(bwe), WithDynamicLength(cfg))
		pushSeqs(a, 0)
		pushSeqs(b, 0)
		assert.Equal(t, uint16(25), a.minStartCount)
		assert.Equal(t, a.minStartCount, b.minStartCount)
	})

	t.Run("non-positive bitrate falls back", func(t *testing.T) {
		bwe := &fakeBWE{1_000_000}
		jb := New(WithMinimumPacketCount(20), WithBandwidthEstimator(bwe))
		pushSeqs(jb, 0)
		assert.Equal(t, uint16(13), jb.minStartCount)
		bwe.bitrate = -1
		pushSeqs(jb, 1)
		assert.Equal(t, uint16(20), jb.minStartCount)
	})

	t.Run("bitrate decrease while buffering", func(t *testing.T) {
		bwe := &fakeBWE{4_000_000}
		jb := New(WithBandwidthEstimator(bwe))
		for seq := range uint16(14) {
			jb.Push(dynPacket(seq))
		}
		require.Equal(t, Buffering, jb.state)
		bwe.bitrate = 1_000_000
		pushSeqs(jb, 14)
		assert.Equal(t, uint16(13), jb.minStartCount)
		assert.Equal(t, Emitting, jb.state)
	})

	t.Run("Clear(true) resets growth and keeps estimator", func(t *testing.T) {
		jb := New(
			WithMinimumPacketCount(4),
			WithDynamicLength(DynamicLengthConfig{UnderflowsPerMinute: 1}),
			WithBandwidthEstimator(&fakeBWE{0}),
		)
		underflowCycle(t, jb, withMockClock(jb), 0, 4, 0)
		require.Equal(t, uint16(9), jb.minStartCount)

		jb.Clear(true)
		assert.Equal(t, uint16(4), jb.minStartCount)
		assert.Zero(t, jb.dynamic.avgPacketSize)

		jb.dynamic.estimator.(*fakeBWE).bitrate = 1_000_000 //nolint:forcetypeassert
		pushSeqs(jb, 0)
		assert.Equal(t, uint16(13), jb.minStartCount)
	})

	t.Run("packet size average", func(t *testing.T) {
		jb := New(WithBandwidthEstimator(&fakeBWE{1_000_000}))
		pushSeqs(jb, 0)
		small := dynPacket(1)
		small.Payload = small.Payload[:88]
		jb.Push(small)
		assert.InDelta(t, 943.75, jb.dynamic.avgPacketSize, 1e-9)
		assert.Equal(t, uint16(14), jb.minStartCount)
	})
}

type simCase struct {
	bitrate                      int
	latency, jitter, targetDelay time.Duration
	// latencyStep is added to the latency at 15s, 30s and 45s.
	latencyStep time.Duration
	loss        float64
}

func (c simCase) interval() time.Duration {
	return time.Duration(int64(time.Second) * 8 * dynPacketSize / int64(c.bitrate))
}

func (c simCase) total() int { return int(time.Minute / c.interval()) }

type simResult struct {
	delivered     []uint16
	drops         int
	underflows    uint32
	skips         uint32
	initialLength uint16
	finalLength   uint16
	extra         uint16
	// uncovered counts underflows while the length already covered the
	// worst-case lateness.
	uncovered int
}

// arrivals returns when each packet reaches the receiver through a FIFO
// network, -1 for lost packets, and the number lost.
func (c simCase) arrivals() ([]time.Duration, int) {
	interval, total := c.interval(), c.total()
	rng := rand.New(rand.NewPCG(1, 0)) //nolint:gosec
	arrivals := make([]time.Duration, total)
	drops := 0
	var prev time.Duration
	for i := range total {
		send := time.Duration(i) * interval
		steps := min(int64(send/(15*time.Second)), 3)
		delay := c.latency + time.Duration(steps)*c.latencyStep + time.Duration(rng.Float64()*float64(c.jitter))
		if rng.Float64() < c.loss && i != 0 && i != total-1 {
			arrivals[i] = -1
			drops++

			continue
		}
		prev = max(prev, send+delay)
		arrivals[i] = prev
	}

	return arrivals, drops
}

// simulate sends 60s of 1000-byte packets and plays them out one per packet
// interval, concealing the slots of lost packets.
func simulate(sim simCase) simResult { //nolint:cyclop
	interval, total := sim.interval(), sim.total()
	arrivals, drops := sim.arrivals()
	res := simResult{drops: drops}
	end := arrivals[total-1] + 2*time.Second

	jb := New(
		WithBandwidthEstimator(&fakeBWE{sim.bitrate}),
		WithDynamicLength(DynamicLengthConfig{
			MinPackets: 1, StepPackets: 5, UnderflowsPerMinute: 3, TargetDelay: sim.targetDelay,
		}),
	)
	clock := withMockClock(jb)
	last := uint16(total - 1) //nolint:gosec
	next, hold := 0, 0
	for now := time.Duration(0); now <= end; now += interval {
		clock.SetNow(time.Unix(0, 0).Add(now))
		for ; next < total && arrivals[next] <= now; next++ {
			if arrivals[next] >= 0 {
				jb.Push(dynPacket(uint16(next))) //nolint:gosec
				if res.initialLength == 0 {
					res.initialLength = jb.minStartCount
				}
			}
		}
		if jb.state != Emitting {
			continue
		}
		if hold > 0 {
			hold--

			continue
		}
		length := jb.minStartCount
		pkt, err := jb.Pop()
		if err != nil {
			if time.Duration(length-1)*interval > sim.jitter+sim.latencyStep {
				res.uncovered++
			}

			continue
		}
		if n := len(res.delivered); n > 0 {
			hold = int(pkt.SequenceNumber - res.delivered[n-1] - 1)
		}
		res.delivered = append(res.delivered, pkt.SequenceNumber)
		if pkt.SequenceNumber == last {
			break
		}
	}
	res.underflows = jb.stats.underflowCount
	res.skips = jb.stats.skippedCount
	res.finalLength = jb.minStartCount
	res.extra = jb.dynamic.extra

	return res
}

func TestDynamicLengthNetworkSimulation(t *testing.T) {
	const ms = time.Millisecond
	const unchecked = -1
	for _, tc := range []struct {
		name string
		simCase
		underflows int
		length     int
	}{
		{"latency 80ms", simCase{1_000_000, 80 * ms, 0, 100 * ms, 0, 0}, 0, 13},
		{"latency 300ms, high bandwidth", simCase{4_000_000, 300 * ms, 0, 100 * ms, 0, 0}, 0, 50},
		{"loss 5%", simCase{1_000_000, 50 * ms, 0, 100 * ms, 0, .05}, 0, 13},
		{"loss 20%", simCase{1_000_000, 50 * ms, 0, 100 * ms, 0, .20}, 0, 13},
		{"jitter 40ms", simCase{1_000_000, 50 * ms, 40 * ms, 20 * ms, 0, 0}, unchecked, unchecked},
		{"jitter 40ms, latency 200ms", simCase{1_000_000, 200 * ms, 40 * ms, 20 * ms, 0, 0}, unchecked, unchecked},
		{"jitter 40ms + loss 10%", simCase{1_000_000, 50 * ms, 40 * ms, 20 * ms, 0, .10}, unchecked, unchecked},
		{"jitter 40ms, high bandwidth", simCase{4_000_000, 50 * ms, 40 * ms, 20 * ms, 0, 0}, unchecked, unchecked},
		{"latency steps 30ms", simCase{1_000_000, 50 * ms, 0, 20 * ms, 30 * ms, 0}, 3, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := simulate(tc.simCase)
			total := tc.total()

			require.NotEmpty(t, res.delivered)
			for i := 1; i < len(res.delivered); i++ {
				require.Less(t, res.delivered[i-1], res.delivered[i])
			}
			assert.Equal(t, uint16(total-1), res.delivered[len(res.delivered)-1]) //nolint:gosec
			assert.Len(t, res.delivered, total-res.drops)
			assert.Equal(t, uint32(res.drops), res.skips) //nolint:gosec
			if tc.loss > 0 {
				assert.Positive(t, res.drops)
			}
			assert.Zero(t, res.extra%5)
			assert.Equal(t, res.initialLength+res.extra, res.finalLength)
			assert.Zero(t, res.uncovered)
			if tc.underflows != unchecked {
				assert.Equal(t, uint32(tc.underflows), res.underflows) //nolint:gosec
			}
			if tc.length != unchecked {
				assert.Equal(t, uint16(tc.length), res.finalLength) //nolint:gosec
			}
			t.Logf("drops %d, underflows %d, length %d -> %d", res.drops, res.underflows, res.initialLength, res.finalLength)
		})
	}
}
