// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package report

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReceiverStream(t *testing.T) {
	t.Run("can use entire history size", func(t *testing.T) {
		stream := newReceiverStream(12345, 90000)
		maxPackets := stream.size * packetsPerHistoryEntry

		// We shouldn't wrap around so long as we only try maxPackets worth.
		for seq := uint16(0); seq < maxPackets; seq++ {
			require.False(t, stream.getReceived(seq), "packet with SN %v shouldn't be received yet", seq)
			stream.setReceived(seq)
			require.True(t, stream.getReceived(seq), "packet with SN %v should now be received", seq)
		}

		// Delete should also work.
		for seq := uint16(0); seq < maxPackets; seq++ {
			require.True(t, stream.getReceived(seq), "packet with SN %v should still be marked as received", seq)
			stream.delReceived(seq)
			require.False(t, stream.getReceived(seq), "packet with SN %v should no longer be received", seq)
		}
	})
}
