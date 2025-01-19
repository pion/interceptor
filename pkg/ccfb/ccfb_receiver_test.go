package ccfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

func TestConvertCCFB(t *testing.T) {
	timeZero := time.Now()
	cases := []struct {
		ts       time.Time
		feedback *rtcp.CCFeedbackReport
		expect   map[uint32]acknowledgementList
		expectTS time.Time
	}{
		{},
		{
			ts: timeZero.Add(2 * time.Second),
			feedback: &rtcp.CCFeedbackReport{
				SenderSSRC: 1,
				ReportBlocks: []rtcp.CCFeedbackReportBlock{
					{
						MediaSSRC:     2,
						BeginSequence: 17,
						MetricBlocks: []rtcp.CCFeedbackMetricBlock{
							{
								Received:          true,
								ECN:               0,
								ArrivalTimeOffset: 512,
							},
						},
					},
				},
				ReportTimestamp: ntp.ToNTP32(timeZero.Add(time.Second)),
			},
			expect: map[uint32]acknowledgementList{
				2: {
					ts: timeZero.Add(2 * time.Second),
					acks: []acknowledgement{
						{
							seqNr:   17,
							arrived: true,
							arrival: timeZero.Add(500 * time.Millisecond),
							ecn:     0,
						},
					},
				},
			},
			expectTS: timeZero.Add(time.Second),
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			resTS, res := convertCCFB(tc.ts, tc.feedback)

			assert.InDelta(t, tc.expectTS.UnixNano(), resTS.UnixNano(), float64(time.Millisecond.Nanoseconds()))

			// Can't directly check equality since arrival timestamp conversions
			// may be slightly off due to ntp conversions.
			assert.Equal(t, len(tc.expect), len(res))
			for i, ee := range tc.expect {
				assert.Equal(t, ee.ts, res[i].ts)
				for j, ack := range ee.acks {
					assert.Equal(t, ack.seqNr, res[i].acks[j].seqNr)
					assert.Equal(t, ack.arrived, res[i].acks[j].arrived)
					assert.Equal(t, ack.ecn, res[i].acks[j].ecn)
					assert.InDelta(t, ack.arrival.UnixNano(), res[i].acks[j].arrival.UnixNano(), float64(time.Millisecond.Nanoseconds()))
				}
			}
		})
	}
}

func TestConvertMetricBlock(t *testing.T) {
	cases := []struct {
		ts          time.Time
		reference   time.Time
		seqNrOffset uint16
		blocks      []rtcp.CCFeedbackMetricBlock
		expected    acknowledgementList
	}{
		{
			ts:          time.Time{},
			reference:   time.Time{},
			seqNrOffset: 0,
			blocks:      []rtcp.CCFeedbackMetricBlock{},
			expected: acknowledgementList{
				ts:   time.Time{},
				acks: []acknowledgement{},
			},
		},
		{
			ts:          time.Time{}.Add(2 * time.Second),
			reference:   time.Time{}.Add(time.Second),
			seqNrOffset: 3,
			blocks: []rtcp.CCFeedbackMetricBlock{
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 512,
				},
				{
					Received:          false,
					ECN:               0,
					ArrivalTimeOffset: 0,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 0,
				},
			},
			expected: acknowledgementList{
				ts: time.Time{}.Add(2 * time.Second),
				acks: []acknowledgement{
					{
						seqNr:   3,
						arrived: true,
						arrival: time.Time{}.Add(500 * time.Millisecond),
						ecn:     0,
					},
					{
						seqNr:   4,
						arrived: false,
						arrival: time.Time{},
						ecn:     0,
					},
					{
						seqNr:   5,
						arrived: true,
						arrival: time.Time{}.Add(time.Second),
						ecn:     0,
					},
				},
			},
		},
		{
			ts:          time.Time{}.Add(2 * time.Second),
			reference:   time.Time{}.Add(time.Second),
			seqNrOffset: 3,
			blocks: []rtcp.CCFeedbackMetricBlock{
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 512,
				},
				{
					Received:          false,
					ECN:               0,
					ArrivalTimeOffset: 0,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 0,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 0x1FFF,
				},
			},
			expected: acknowledgementList{
				ts: time.Time{}.Add(2 * time.Second),
				acks: []acknowledgement{
					{
						seqNr:   3,
						arrived: true,
						arrival: time.Time{}.Add(500 * time.Millisecond),
						ecn:     0,
					},
					{
						seqNr:   4,
						arrived: false,
						arrival: time.Time{},
						ecn:     0,
					},
					{
						seqNr:   5,
						arrived: true,
						arrival: time.Time{}.Add(time.Second),
						ecn:     0,
					},
					{
						seqNr:   6,
						arrived: true,
						arrival: time.Time{},
						ecn:     0,
					},
				},
			},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			res := convertMetricBlock(tc.ts, tc.reference, tc.seqNrOffset, tc.blocks)
			assert.Equal(t, tc.expected, res)
		})
	}
}
