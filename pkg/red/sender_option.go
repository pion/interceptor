// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package red

import "fmt"

// SenderOption configures a SenderInterceptor.
type SenderOption func(*SenderInterceptor) error

// SenderMaxPacketSize sets the maximum size of an RTP packet after RED is added.
// Packets that already exceed this size are forwarded unchanged.
func SenderMaxPacketSize(maxPacketSize int) SenderOption {
	return func(sender *SenderInterceptor) error {
		if maxPacketSize <= 0 {
			return fmt.Errorf("%w: %d", errInvalidMaxPacketSize, maxPacketSize)
		}

		sender.maxPacketSize = maxPacketSize

		return nil
	}
}
