// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package pacing

import (
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
		assert.Equal(t, 40_000, mp.burst)
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
}
