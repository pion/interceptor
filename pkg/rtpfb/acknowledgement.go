// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"time"

	"github.com/pion/rtcp"
)

type acknowledgement struct {
	sequenceNumber uint16
	arrived        bool
	arrival        time.Time
	ecn            rtcp.ECN
}
