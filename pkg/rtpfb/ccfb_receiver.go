// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"time"

	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
)

func convertCCFB(ts time.Time, feedback *rtcp.CCFeedbackReport) (time.Duration, map[uint32][]acknowledgement) {
	if feedback == nil {
		return 0, nil
	}
	result := map[uint32][]acknowledgement{}
	referenceTime := ntp.ToTime32(feedback.ReportTimestamp, ts)
	latestArrival := time.Time{}
	for _, rb := range feedback.ReportBlocks {
		var la time.Time
		la, result[rb.MediaSSRC] = convertMetricBlock(referenceTime, rb.BeginSequence, rb.MetricBlocks)
		if la.After(latestArrival) {
			latestArrival = la
		}
	}

	return referenceTime.Sub(latestArrival), result
}

func convertMetricBlock(
	reference time.Time,
	seqNrOffset uint16,
	blocks []rtcp.CCFeedbackMetricBlock,
) (time.Time, []acknowledgement) {
	reports := make([]acknowledgement, len(blocks))
	latestArrival := time.Time{}
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
				if arrival.After(latestArrival) {
					latestArrival = arrival
				}
			}
			reports[i] = acknowledgement{
				sequenceNumber: seqNrOffset + uint16(i), // nolint:gosec
				arrived:        true,
				arrival:        arrival,
				ecn:            mb.ECN,
			}
		} else {
			reports[i] = acknowledgement{
				sequenceNumber: seqNrOffset + uint16(i), // nolint:gosec
				arrived:        false,
				arrival:        time.Time{},
				ecn:            0,
			}
		}
	}

	return latestArrival, reports
}
