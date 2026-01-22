// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rfc8888

import (
	"math/rand"
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

	t.Run("MaxreportsPerStream 3 streams", func(t *testing.T) {
		recorder := NewRecorder()
		now := time.Time{}
		maxSize := 1200

		streams := 3
		packets := 1000
		// Add 1000 packets on 3 different streams
		for i := 0; i < streams; i++ {
			ssrc := rand.Uint32() //nolint:gosec
			for j := 0; j < packets; j++ {
				recorder.AddPacket(now, ssrc, uint16(j), 0) //nolint:gosec // G115
			}
		}
		reports := recorder.BuildReport(time.Time{}, maxSize)

		blocks := 0
		for i := 0; i < streams; i++ {
			blocks += len(reports.ReportBlocks[i].MetricBlocks)
		}
		assert.Less(t, blocks*2, maxSize)
	})

	t.Run("MaxreportsPerStream 10 streams", func(t *testing.T) {
		recorder := NewRecorder()
		now := time.Time{}
		maxSize := 1300

		streams := 10
		packets := 1000
		// Add 1000 packets on 10 different streams
		for i := 0; i < streams; i++ {
			ssrc := rand.Uint32() //nolint:gosec
			for j := 0; j < packets; j++ {
				recorder.AddPacket(now, ssrc, uint16(j), 0) //nolint:gosec // G115
			}
		}
		reports := recorder.BuildReport(time.Time{}, maxSize)

		blocks := 0
		for i := 0; i < streams; i++ {
			blocks += len(reports.ReportBlocks[i].MetricBlocks)
		}
		assert.Less(t, blocks*2, maxSize)
	})
}
