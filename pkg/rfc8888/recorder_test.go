package rfc8888

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
)

func TestGetArrivalTimeOffset(t *testing.T) {
	for _, test := range []struct {
		base    time.Time
		arrival time.Time
		want    uint16
	}{
		{
			base:    time.Time{}.Add(time.Second),
			arrival: time.Time{},
			want:    1024,
		},
		{
			base:    time.Time{}.Add(500 * time.Millisecond),
			arrival: time.Time{},
			want:    512,
		},
		{
			base:    time.Time{}.Add(8 * time.Second),
			arrival: time.Time{},
			want:    0x1FFE,
		},
		{
			base:    time.Time{},
			arrival: time.Time{}.Add(time.Second),
			want:    0x1FFF,
		},
	} {
		assert.Equal(t, test.want, getArrivalTimeOffset(test.base, test.arrival))
	}
}

func TestRecorder(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		recorder := NewRecorder()
		now := time.Time{}
		recorder.AddPacket(now, 123456, 0, 0)
		recorder.AddPacket(now.Add(125*time.Millisecond), 123456, 1, 0)
		recorder.AddPacket(now.Add(250*time.Millisecond), 123456, 2, 0)
		recorder.AddPacket(now.Add(500*time.Millisecond), 123456, 3, 0)
		recorder.AddPacket(now.Add(625*time.Millisecond), 123456, 4, 0)
		recorder.AddPacket(now.Add(750*time.Millisecond), 123456, 5, 0)

		report := recorder.BuildReport(now.Add(time.Second), 1500)
		assert.Equal(t, 1, len(report.ReportBlocks))
		assert.Equal(t, rtcp.CCFeedbackReportBlock{
			MediaSSRC:     123456,
			BeginSequence: 0,
			MetricBlocks: []rtcp.CCFeedbackMetricBlock{
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 128,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 256,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 512,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 640,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 768,
				},
			},
		}, report.ReportBlocks[0])
	})

	t.Run("packet loss", func(t *testing.T) {
		recorder := NewRecorder()
		now := time.Time{}
		recorder.AddPacket(now, 123456, 0, 0)
		recorder.AddPacket(now.Add(250*time.Millisecond), 123456, 2, 0)
		recorder.AddPacket(now.Add(625*time.Millisecond), 123456, 4, 0)
		recorder.AddPacket(now.Add(750*time.Millisecond), 123456, 5, 0)

		report := recorder.BuildReport(now.Add(time.Second), 1500)
		assert.Equal(t, 1, len(report.ReportBlocks))
		assert.Equal(t, 6, len(report.ReportBlocks[0].MetricBlocks))
		assert.Equal(t, rtcp.CCFeedbackReportBlock{
			MediaSSRC:     123456,
			BeginSequence: 0,
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
					ArrivalTimeOffset: 1024 - 256,
				},
				{
					Received:          false,
					ECN:               0,
					ArrivalTimeOffset: 0,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 640,
				},
				{
					Received:          true,
					ECN:               0,
					ArrivalTimeOffset: 1024 - 768,
				},
			},
		}, report.ReportBlocks[0])
	})

	t.Run("MaxreportsPerStream", func(t *testing.T) {
		recorder := NewRecorder()
		now := time.Time{}

		// Add 1000 packets on 10 different streams
		for i := 0; i < 10; i++ {
			for j := 0; j < 100; j++ {
				recorder.AddPacket(now, uint32(i), uint16(j), 0)
			}
		}
		reports := recorder.BuildReport(time.Time{}, 1380)

		for i := 0; i < 10; i++ {
			assert.Greater(t, 72, len(reports.ReportBlocks[i].MetricBlocks))
			assert.Less(t, 3, len(reports.ReportBlocks[i].MetricBlocks))
		}
	})
}

func TestNTPTime32(t *testing.T) {
	zero := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
	notSoLongAgo := time.Date(2022, time.May, 5, 14, 48, 20, 0, time.UTC)
	for i, cc := range []struct {
		input    time.Time
		expected uint32
	}{
		{
			input:    zero,
			expected: 0,
		},
		{
			input:    zero.Add(time.Second),
			expected: 1 << 16,
		},
		{
			input:    notSoLongAgo,
			expected: uint32(int(notSoLongAgo.Sub(zero).Seconds())&0xffff) << 16,
		},
		{
			input:    zero.Add(400 * time.Millisecond),
			expected: 26214,
		},
		{
			input:    zero.Add(1400 * time.Millisecond),
			expected: 1<<16 + 26214,
		},
	} {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			res := ntpTime32(cc.input)
			assert.Equalf(t, cc.expected, res, "%b != %b", cc.expected, res)
		})
	}
}
