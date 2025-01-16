package ccfb

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

type acknowledgementList struct {
	ts   time.Time
	acks []acknowledgement
}

func convertCCFB(ts time.Time, feedback *rtcp.CCFeedbackReport) map[uint32]acknowledgementList {
	result := map[uint32]acknowledgementList{}
	referenceTime := ntp.ToTime(uint64(feedback.ReportTimestamp) << 16)
	for _, rb := range feedback.ReportBlocks {
		result[rb.MediaSSRC] = convertMetricBlock(ts, referenceTime, rb.BeginSequence, rb.MetricBlocks)
	}
	return result
}

func convertMetricBlock(ts time.Time, referenceTime time.Time, seqNrOffset uint16, blocks []rtcp.CCFeedbackMetricBlock) acknowledgementList {
	reports := make([]acknowledgement, len(blocks))
	for i, mb := range blocks {
		if mb.Received {
			delta := time.Duration((float64(mb.ArrivalTimeOffset) / 1024.0) * float64(time.Second))
			reports[i] = acknowledgement{
				seqNr:   seqNrOffset + uint16(i),
				arrived: true,
				arrival: referenceTime.Add(-delta),
				ecn:     mb.ECN,
			}
		} else {
			reports[i] = acknowledgement{
				seqNr:   seqNrOffset + uint16(i),
				arrived: false,
				arrival: time.Time{},
				ecn:     0,
			}
		}
	}
	return acknowledgementList{
		ts:   ts,
		acks: reports,
	}
}
