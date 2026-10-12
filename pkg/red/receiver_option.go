// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import (
	"fmt"
	"time"
)

// ReceiverOption configures a ReceiverInterceptor.
type ReceiverOption func(*ReceiverInterceptor) error

// ReceiverPacketDuration declares a continuous stream with a fixed packet duration.
// Each increment of the RTP sequence number must advance the Opus timestamp by
// this duration, with no DTX or padding-only packets consuming sequence numbers.
// This enables recovery without prior packet history and when only some missing
// packets are carried as redundancy. Durations must be multiples of 2.5ms between
// 2.5ms and 120ms; Opus RTP timestamps use a 48kHz clock.
func ReceiverPacketDuration(duration time.Duration) ReceiverOption {
	return func(receiver *ReceiverInterceptor) error {
		const frameDuration = 2500 * time.Microsecond
		if duration <= 0 || duration > 120*time.Millisecond || duration%frameDuration != 0 {
			return fmt.Errorf("%w: %s", errInvalidReceiverPacketDuration, duration)
		}

		// Each 2.5ms interval contains 120 ticks at the Opus RTP clock rate.
		receiver.timestampStep = uint32(duration/frameDuration) * 120 //nolint:gosec // Duration is bounded to 120ms.

		return nil
	}
}
