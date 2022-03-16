package rfc8888

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

type input struct {
	ts  time.Time
	nr  uint16
	ecn uint8
}

func TestStreamLogAdd(t *testing.T) {
	tests := []struct {
		name         string
		inputs       []input
		expectedNext int64
		expectedLast int64
		expectedLog  map[int64]*packetReport
	}{
		{
			name:         "emptyLog",
			inputs:       []input{},
			expectedNext: 0,
			expectedLast: 0,
			expectedLog:  map[int64]*packetReport{},
		},
		//nolint
		{
			name: "addInOrderSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(10 * time.Millisecond),
					nr:  1,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNext: 0,
			expectedLast: 3,
			expectedLog: map[int64]*packetReport{
				0: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				1: {
					arrivalTime: time.Time{}.Add(10 * time.Millisecond),
					ecn:         0,
				},
				2: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				3: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
			},
		},
		//nolint
		{
			name: "reorderedSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(10 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  1,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNext: 0,
			expectedLast: 3,
			expectedLog: map[int64]*packetReport{
				0: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				1: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				2: {
					arrivalTime: time.Time{}.Add(10 * time.Millisecond),
					ecn:         0,
				},
				3: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
			},
		},
		{
			name: "reorderedWrappingSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  65534,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(10 * time.Millisecond),
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  65535,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(40 * time.Millisecond),
					nr:  1,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(50 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNext: 65534,
			expectedLast: 65539,
			expectedLog: map[int64]*packetReport{
				65534: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				65535: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				65536: {
					arrivalTime: time.Time{}.Add(10 * time.Millisecond),
					ecn:         0,
				},
				65537: {
					arrivalTime: time.Time{}.Add(40 * time.Millisecond),
					ecn:         0,
				},
				65538: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
				65539: {
					arrivalTime: time.Time{}.Add(50 * time.Millisecond),
					ecn:         0,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sl := newStreamLog(0)
			for _, input := range test.inputs {
				sl.add(input.ts, input.nr, input.ecn)
			}
			assert.Equal(t, test.expectedNext, sl.nextSequenceNumberToReport)
			assert.Equal(t, test.expectedLast, sl.lastSequenceNumberReceived)
			assert.Equal(t, test.expectedLog, sl.log)
		})
	}
}

func TestStreamLogMetricsAfter(t *testing.T) {
	tests := []struct {
		name   string
		inputs []input

		expectedLast int64

		expectedNextBefore int64
		expectedLogBefore  map[int64]*packetReport

		expectedNextAfter int64
		expectedLogAfter  map[int64]*packetReport

		expectedMetrics rtcp.CCFeedbackReportBlock
	}{
		{
			name:               "emptyLog",
			inputs:             []input{},
			expectedNextBefore: 0,
			expectedLast:       0,
			expectedLogBefore:  map[int64]*packetReport{},
			expectedNextAfter:  0,
			expectedLogAfter:   map[int64]*packetReport{},
			expectedMetrics: rtcp.CCFeedbackReportBlock{
				MediaSSRC:     0,
				BeginSequence: 0,
				MetricBlocks:  []rtcp.CCFeedbackMetricBlock{},
			},
		},
		//nolint
		{
			name: "addInOrderSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(10 * time.Millisecond),
					nr:  1,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNextBefore: 0,
			expectedLast:       3,
			expectedLogBefore: map[int64]*packetReport{
				0: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				1: {
					arrivalTime: time.Time{}.Add(10 * time.Millisecond),
					ecn:         0,
				},
				2: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				3: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
			},
			expectedNextAfter: 4,
			expectedLogAfter:  map[int64]*packetReport{},
			expectedMetrics: rtcp.CCFeedbackReportBlock{
				MetricBlocks: []rtcp.CCFeedbackMetricBlock{
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1024,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1013,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1003,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 993,
					},
				},
			},
		},
		//nolint
		{
			name: "reorderedSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(10 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  1,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNextBefore: 0,
			expectedLast:       3,
			expectedLogBefore: map[int64]*packetReport{
				0: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				1: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				2: {
					arrivalTime: time.Time{}.Add(10 * time.Millisecond),
					ecn:         0,
				},
				3: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
			},
			expectedNextAfter: 4,
			expectedLogAfter:  map[int64]*packetReport{},
			expectedMetrics: rtcp.CCFeedbackReportBlock{
				MetricBlocks: []rtcp.CCFeedbackMetricBlock{
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1024,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1003,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1013,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 993,
					},
				},
			},
		},
		{
			name: "reorderedWrappingSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  65534,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(10 * time.Millisecond),
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  65535,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(40 * time.Millisecond),
					nr:  1,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(50 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNextBefore: 65534,
			expectedLast:       65539,
			expectedLogBefore: map[int64]*packetReport{
				65534: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				65535: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				65536: {
					arrivalTime: time.Time{}.Add(10 * time.Millisecond),
					ecn:         0,
				},
				65537: {
					arrivalTime: time.Time{}.Add(40 * time.Millisecond),
					ecn:         0,
				},
				65538: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
				65539: {
					arrivalTime: time.Time{}.Add(50 * time.Millisecond),
					ecn:         0,
				},
			},
			expectedNextAfter: 65540,
			expectedLogAfter:  map[int64]*packetReport{},
			expectedMetrics: rtcp.CCFeedbackReportBlock{
				BeginSequence: 65534,
				MetricBlocks: []rtcp.CCFeedbackMetricBlock{
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1024,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1003,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1013,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 983,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 993,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 972,
					},
				},
			},
		},
		{
			name: "addMissingPacketSequence",
			inputs: []input{
				{
					ts:  time.Time{},
					nr:  0,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(20 * time.Millisecond),
					nr:  2,
					ecn: 0,
				},
				{
					ts:  time.Time{}.Add(30 * time.Millisecond),
					nr:  3,
					ecn: 0,
				},
			},
			expectedNextBefore: 0,
			expectedLast:       3,
			expectedLogBefore: map[int64]*packetReport{
				0: {
					arrivalTime: time.Time{},
					ecn:         0,
				},
				2: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				3: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
			},
			expectedNextAfter: 1,
			expectedLogAfter: map[int64]*packetReport{
				2: {
					arrivalTime: time.Time{}.Add(20 * time.Millisecond),
					ecn:         0,
				},
				3: {
					arrivalTime: time.Time{}.Add(30 * time.Millisecond),
					ecn:         0,
				},
			},
			expectedMetrics: rtcp.CCFeedbackReportBlock{
				MetricBlocks: []rtcp.CCFeedbackMetricBlock{
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1024,
					},
					{
						Received:          false,
						ECN:               0,
						ArrivalTimeOffset: 0,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 1003,
					},
					{
						Received:          true,
						ECN:               0,
						ArrivalTimeOffset: 993,
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sl := newStreamLog(0)
			for _, input := range test.inputs {
				sl.add(input.ts, input.nr, input.ecn)
			}

			assert.Equal(t, test.expectedNextBefore, sl.nextSequenceNumberToReport)
			assert.Equal(t, test.expectedLast, sl.lastSequenceNumberReceived)
			assert.Equal(t, test.expectedLogBefore, sl.log)

			metrics := sl.metricsAfter(time.Time{}.Add(time.Second), 500)

			assert.Equal(t, test.expectedNextAfter, sl.nextSequenceNumberToReport)
			assert.Equal(t, test.expectedLast, sl.lastSequenceNumberReceived)
			assert.Equal(t, test.expectedLogAfter, sl.log)

			assert.Equal(t, test.expectedMetrics, metrics)
		})
	}
}

func TestRemoveOldestPackets(t *testing.T) {
	sl := newStreamLog(0)
	sl.add(time.Time{}.Add(time.Second), 1, 0)
	now := time.Now().Add(10 * time.Second)
	for i := 2; i < 16386; i++ {
		now = now.Add(10 * time.Millisecond)
		sl.add(now, uint16(i), 0)
	}
	metrics := sl.metricsAfter(now, maxReportsPerReportBlock)
	assert.Equal(t, uint16(2), metrics.BeginSequence)
	assert.Lenf(t, metrics.MetricBlocks, 16384, "%v != %v", len(metrics.MetricBlocks), 16384)
}
