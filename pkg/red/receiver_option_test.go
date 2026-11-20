// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiverPacketDuration(t *testing.T) {
	for duration, step := range map[time.Duration]uint32{
		2500 * time.Microsecond: 120,
		5 * time.Millisecond:    240,
		10 * time.Millisecond:   480,
		20 * time.Millisecond:   960,
		40 * time.Millisecond:   1920,
		60 * time.Millisecond:   2880,
		120 * time.Millisecond:  5760,
	} {
		receiver := &ReceiverInterceptor{}
		require.NoError(t, ReceiverPacketDuration(duration)(receiver))
		assert.Equal(t, step, receiver.timestampStep, duration)
	}

	for _, duration := range []time.Duration{
		0, -time.Millisecond, time.Nanosecond, 2 * time.Millisecond, 3750 * time.Microsecond, 122500 * time.Microsecond,
	} {
		factory, err := NewReceiverInterceptor(ReceiverPacketDuration(duration))
		require.NoError(t, err)
		_, err = factory.NewInterceptor("")
		assert.ErrorIs(t, err, errInvalidReceiverPacketDuration, duration)
	}
}
