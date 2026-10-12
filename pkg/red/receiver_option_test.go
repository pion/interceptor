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
	tests := []struct {
		duration time.Duration
		step     uint32
	}{
		{2500 * time.Microsecond, 120},
		{5 * time.Millisecond, 240},
		{10 * time.Millisecond, 480},
		{20 * time.Millisecond, 960},
		{40 * time.Millisecond, 1920},
		{60 * time.Millisecond, 2880},
		{120 * time.Millisecond, 5760},
	}
	for _, test := range tests {
		t.Run(test.duration.String(), func(t *testing.T) {
			factory, err := NewReceiverInterceptor(ReceiverPacketDuration(test.duration))
			require.NoError(t, err)
			instance, err := factory.NewInterceptor("")
			require.NoError(t, err)
			receiver, ok := instance.(*ReceiverInterceptor)
			require.True(t, ok)
			assert.Equal(t, test.step, receiver.timestampStep)

			reader := receiver.BindRemoteStream(opusStreamInfo(), &queuedRTPReader{})
			stream, ok := reader.(*receiverStream)
			require.True(t, ok)
			assert.Equal(t, test.step, stream.timestampStep)
		})
	}
}

func TestReceiverPacketDurationRejectsInvalidDuration(t *testing.T) {
	for _, duration := range []time.Duration{
		0,
		-time.Millisecond,
		time.Nanosecond,
		2 * time.Millisecond,
		3750 * time.Microsecond,
		122500 * time.Microsecond,
	} {
		t.Run(duration.String(), func(t *testing.T) {
			factory, err := NewReceiverInterceptor(ReceiverPacketDuration(duration))
			require.NoError(t, err)
			instance, err := factory.NewInterceptor("")
			assert.ErrorIs(t, err, errInvalidReceiverPacketDuration)
			assert.Nil(t, instance)
		})
	}
}

func TestReceiverInterceptorUsesHistoryByDefault(t *testing.T) {
	factory, err := NewReceiverInterceptor()
	require.NoError(t, err)
	instance, err := factory.NewInterceptor("")
	require.NoError(t, err)
	receiver, ok := instance.(*ReceiverInterceptor)
	require.True(t, ok)
	assert.Zero(t, receiver.timestampStep)
	reader := receiver.BindRemoteStream(opusStreamInfo(), &queuedRTPReader{})
	stream, ok := reader.(*receiverStream)
	require.True(t, ok)
	assert.Zero(t, stream.timestampStep)
}
