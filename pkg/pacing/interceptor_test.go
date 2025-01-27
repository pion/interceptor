package pacing

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/stretchr/testify/assert"
)

type mockPacer struct{}

// AllowN implements pacer.
func (m *mockPacer) AllowN(time.Time, int) bool {
	panic("unimplemented")
}

// Budget implements pacer.
func (m *mockPacer) Budget(time.Time) float64 {
	panic("unimplemented")
}

// SetRate implements pacer.
func (m *mockPacer) SetRate(rate int, burst int) {
	panic("unimplemented")
}

func TestInterceptor(t *testing.T) {
	mp := &mockPacer{}
	i := NewInterceptor(setPacerFactory(func(initialRate, burst int) pacer {
		return mp
	}))

	pacer, err := i.NewInterceptor("")
	assert.NoError(t, err)

	stream := test.NewMockStream(&interceptor.StreamInfo{}, pacer)
	defer func() {
		assert.NoError(t, stream.Close())
	}()
}
