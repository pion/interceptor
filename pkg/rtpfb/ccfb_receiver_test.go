// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

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
		ts             time.Time
		feedback       *rtcp.CCFeedbackReport
		expect         map[uint32][]acknowledgement
		expectAckDelay time.Duration
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
			expect: map[uint32][]acknowledgement{
				2: {
					{
						sequenceNumber: 17,
						arrived:        true,
						arrival:        timeZero.Add(500 * time.Millisecond),
						ecn:            0,
					},
				},
			},
			expectAckDelay: 500 * time.Millisecond,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			ackDelay, res := convertCCFB(tc.ts, tc.feedback)

			assert.Equal(t, tc.expectAckDelay, ackDelay)

			// Can't directly check equality since arrival timestamp conversions
			// may be slightly off due to ntp conversions.
			assert.Equal(t, len(tc.expect), len(res))
			for i, acks := range tc.expect {
				for j, ack := range acks {
					assert.Equal(t, ack.sequenceNumber, res[i][j].sequenceNumber)
					assert.Equal(t, ack.arrived, res[i][j].arrived)
					assert.Equal(t, ack.ecn, res[i][j].ecn)
					assert.InDelta(t, ack.arrival.UnixNano(), res[i][j].arrival.UnixNano(), float64(time.Millisecond.Nanoseconds()))
				}
			}
		})
	}
}

func TestConvertMetricBlock(t *testing.T) {
	cases := []struct {
		ts                    time.Time
		reference             time.Time
		seqNrOffset           uint16
		blocks                []rtcp.CCFeedbackMetricBlock
		expected              []acknowledgement
		expectedLatestArrival time.Time
	}{
		{
			ts:          time.Time{},
			reference:   time.Time{},
			seqNrOffset: 0,
			blocks:      []rtcp.CCFeedbackMetricBlock{},
			expected:    []acknowledgement{},
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
			expected: []acknowledgement{
				{
					sequenceNumber: 3,
					arrived:        true,
					arrival:        time.Time{}.Add(500 * time.Millisecond),
					ecn:            0,
				},
				{
					sequenceNumber: 4,
					arrived:        false,
					arrival:        time.Time{},
					ecn:            0,
				},
				{
					sequenceNumber: 5,
					arrived:        true,
					arrival:        time.Time{}.Add(time.Second),
					ecn:            0,
				},
			},
			expectedLatestArrival: time.Time{}.Add(time.Second),
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
			expected: []acknowledgement{
				{
					sequenceNumber: 3,
					arrived:        true,
					arrival:        time.Time{}.Add(500 * time.Millisecond),
					ecn:            0,
				},
				{
					sequenceNumber: 4,
					arrived:        false,
					arrival:        time.Time{},
					ecn:            0,
				},
				{
					sequenceNumber: 5,
					arrived:        true,
					arrival:        time.Time{}.Add(time.Second),
					ecn:            0,
				},
				{
					sequenceNumber: 6,
					arrived:        true,
					arrival:        time.Time{},
					ecn:            0,
				},
			},
			expectedLatestArrival: time.Time{}.Add(time.Second),
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			ela, res := convertMetricBlock(tc.reference, tc.seqNrOffset, tc.blocks)
			assert.Equal(t, tc.expected, res)
			assert.Equal(t, tc.expectedLatestArrival, ela)
		})
	}
}
