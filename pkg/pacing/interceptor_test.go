// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package pacing

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/logging"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

type mockPacer struct {
	lock sync.Mutex

	rate  int
	burst int

	allow        bool
	allowCalled  bool
	budget       float64
	budgetCalled bool
}

// AllowN implements pacer.
func (m *mockPacer) AllowN(time.Time, int) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.allowCalled = true

	return m.allow
}

// Budget implements pacer.
func (m *mockPacer) Budget(time.Time) float64 {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.budgetCalled = true

	return m.budget
}

// SetRate implements pacer.
func (m *mockPacer) SetRate(rate int, burst int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.rate = rate
	m.burst = burst
}

func TestBurst(t *testing.T) {
	const minBurst = 8 * 1500

	for _, cc := range []struct {
		name     string
		rate     int
		interval time.Duration
		minBurst int
		expected int
	}{
		{"sub_millisecond_interval", 1_000_000, 500 * time.Microsecond, minBurst, minBurst},
		{"sub_millisecond_interval_high_rate", 100_000_000, 500 * time.Microsecond, minBurst, 50_000},
		{"zero_interval_defaults_to_1ms", 100_000_000, 0, minBurst, 100_000},
		{"negative_interval_defaults_to_1ms", 100_000_000, -time.Second, minBurst, 100_000},
		{"rate_below_min_burst", 300_000, 5 * time.Millisecond, minBurst, minBurst},
		{"divides_evenly", 3_000_000, 5 * time.Millisecond, minBurst, 15_000},
		{"does_not_divide_evenly", 3_000_000, 7 * time.Millisecond, minBurst, 21_000},
		{"long_interval", 3_000_000, 33 * time.Millisecond, minBurst, 99_000},
		{"zero_rate", 0, 5 * time.Millisecond, minBurst, minBurst},
		{"grown_min_burst_wins", 300_000, 5 * time.Millisecond, 8 * 4000, 8 * 4000},
		{"rate_above_grown_min_burst", 100_000_000, 5 * time.Millisecond, 8 * 4000, 500_000},
	} {
		t.Run(cc.name, func(t *testing.T) {
			assert.Equal(t, cc.expected, burst(cc.rate, cc.interval, cc.minBurst))
		})
	}
}

func TestInterceptor(t *testing.T) {
	t.Run("calls_set_rate", func(t *testing.T) {
		mp := &mockPacer{}
		i := NewInterceptor(
			setPacerFactory(func(initialRate, burst int) pacer {
				return mp
			}),
			WithLoggerFactory(logging.NewDefaultLoggerFactory()),
		)

		_, err := i.NewInterceptor("")
		assert.NoError(t, err)

		i.SetRate("", 1_000_000)
		assert.Equal(t, 1_000_000, mp.rate)
		assert.Equal(t, 12000, mp.burst)
	})

	t.Run("grows_burst", func(t *testing.T) {
		mp := &mockPacer{}
		factory := NewInterceptor(
			setPacerFactory(func(int, int) pacer {
				return mp
			}),
			InitialRate(300_000),
			Interval(5*time.Millisecond),
		)

		created, err := factory.NewInterceptor("id")
		assert.NoError(t, err)
		pacer, ok := created.(*Interceptor)
		assert.True(t, ok)
		defer func() {
			assert.NoError(t, pacer.Close())
		}()

		pacer.growMTU(1500)
		mp.lock.Lock()
		assert.Equal(t, 0, mp.burst)
		mp.lock.Unlock()

		pacer.growMTU(4000)
		mp.lock.Lock()
		assert.Equal(t, 300_000, mp.rate)
		assert.Equal(t, 8*4000, mp.burst)
		mp.lock.Unlock()

		pacer.growMTU(1500)
		factory.SetRate("id", 600_000)
		mp.lock.Lock()
		assert.Equal(t, 600_000, mp.rate)
		assert.Equal(t, 8*4000, mp.burst)
		mp.lock.Unlock()
	})

	t.Run("paces_packets", func(t *testing.T) {
		mp := &mockPacer{
			rate:         0,
			burst:        0,
			allow:        false,
			allowCalled:  false,
			budget:       0,
			budgetCalled: false,
		}
		i := NewInterceptor(
			setPacerFactory(func(initialRate, burst int) pacer {
				return mp
			}),
			Interval(time.Millisecond),
		)

		pacer, err := i.NewInterceptor("")
		assert.NoError(t, err)

		stream := test.NewMockStream(&interceptor.StreamInfo{}, pacer)
		defer func() {
			assert.NoError(t, stream.Close())
		}()

		mp.lock.Lock()
		mp.allow = true
		mp.budget = 8 * 1500
		mp.lock.Unlock()

		hdr := rtp.Header{}
		err = stream.WriteRTP(&rtp.Packet{
			Header:  hdr,
			Payload: make([]byte, 1200-hdr.MarshalSize()),
		})
		assert.NoError(t, err)

		select {
		case <-stream.WrittenRTP():
		case <-time.After(time.Second):
			assert.Fail(t, "no RTP packet written")
		}
		mp.lock.Lock()
		assert.True(t, mp.allowCalled)
		assert.True(t, mp.budgetCalled)
		mp.lock.Unlock()

		mp.lock.Lock()
		mp.allow = false
		mp.budget = 0
		mp.lock.Unlock()

		hdr = rtp.Header{}
		err = stream.WriteRTP(&rtp.Packet{
			Header:  hdr,
			Payload: make([]byte, 1200-hdr.MarshalSize()),
		})
		assert.NoError(t, err)

		mp.lock.Lock()
		assert.True(t, mp.allowCalled)
		assert.True(t, mp.budgetCalled)
		mp.lock.Unlock()

		select {
		case <-stream.WrittenRTP():
			assert.Fail(t, "RTP packet written without pacing budget")
		case <-time.After(10 * time.Millisecond):
		}
	})

	// A packet is only sent once the budget covers its full size, but the
	// budget never exceeds the burst size. Packets at or above the burst size
	// must still be sent, instead of blocking the queue forever.
	t.Run("sends_packets_at_and_above_burst_size", func(t *testing.T) {
		// At 300 kbps and a 5ms pacing interval the rate alone allows a burst
		// of only 1500 bits.
		for _, size := range []int{20, 1499, 1500, 1501, 4000} {
			t.Run(fmt.Sprintf("%d_bytes", size), func(t *testing.T) {
				i := NewInterceptor(
					InitialRate(300_000),
					Interval(5*time.Millisecond),
				)

				pacer, err := i.NewInterceptor("")
				assert.NoError(t, err)

				stream := test.NewMockStream(&interceptor.StreamInfo{}, pacer)
				defer func() {
					assert.NoError(t, stream.Close())
				}()

				hdr := rtp.Header{}
				assert.NoError(t, stream.WriteRTP(&rtp.Packet{
					Header:  hdr,
					Payload: make([]byte, size-hdr.MarshalSize()),
				}))

				select {
				case <-stream.WrittenRTP():
				case <-time.After(2 * time.Second):
					assert.Fail(t, "no RTP packet written")
				}
			})
		}
	})
}
