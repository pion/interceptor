// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"time"

	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
)

type acknowledgement struct {
	seqNr   uint16
	arrived bool
	arrival time.Time
	ecn     rtcp.ECN
}

func convertCCFB(ts time.Time, feedback *rtcp.CCFeedbackReport) (time.Time, map[uint32][]acknowledgement) {
	if feedback == nil {
		return time.Time{}, nil
	}
	result := map[uint32][]acknowledgement{}
	referenceTime := ntp.ToTime32(feedback.ReportTimestamp, ts)
	for _, rb := range feedback.ReportBlocks {
		result[rb.MediaSSRC] = convertMetricBlock(referenceTime, rb.BeginSequence, rb.MetricBlocks)
	}

	return referenceTime, result
}

func convertMetricBlock(
	reference time.Time,
	seqNrOffset uint16,
	blocks []rtcp.CCFeedbackMetricBlock,
) []acknowledgement {
	reports := make([]acknowledgement, len(blocks))
	for i, mb := range blocks {
		if mb.Received {
			arrival := time.Time{}

			// RFC 8888 states: If the measurement is unavailable or if the
			// arrival time of the RTP packet is after the time represented by
			// the RTS field, then an ATO value of 0x1FFF MUST be reported for
			// the packet. In that case, we set a zero time.Time value.
			if mb.ArrivalTimeOffset != 0x1FFF {
				delta := time.Duration((float64(mb.ArrivalTimeOffset) / 1024.0) * float64(time.Second))
				arrival = reference.Add(-delta)
			}
			reports[i] = acknowledgement{
				seqNr:   seqNrOffset + uint16(i), // nolint:gosec
				arrived: true,
				arrival: arrival,
				ecn:     mb.ECN,
			}
		} else {
			reports[i] = acknowledgement{
				seqNr:   seqNrOffset + uint16(i), // nolint:gosec
				arrived: false,
				arrival: time.Time{},
				ecn:     0,
			}
		}
	}

	return reports
}
