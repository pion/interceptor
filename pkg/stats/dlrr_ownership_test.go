// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package stats

import (
	"testing"
	"time"

	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsRecorder_DLRROwnership(t *testing.T) {
	sentAt := time.Unix(1700000000, 0)
	reference := ntp.ToNTP(sentAt)
	lastRR := uint32(reference >> 16 & 0xffffffff)
	for _, tc := range []struct {
		name          string
		local, remote uint32
	}{
		{"distinct SSRCs", 17, 42},
		{"zero local SSRC", 0, 42},
		{"zero remote SSRC", 17, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ssrc := range []uint32{tc.local, tc.remote, 99} {
				recorder := newRecorder(ssrc, 90000, logging.NewDefaultLoggerFactory())
				recorder.Start()
				t.Cleanup(recorder.Stop)
				// Repeated RRTRs and equal timestamps from another SSRC must not multiply measurements.
				for _, origin := range []uint32{tc.local, tc.local, 99} {
					recorder.QueueOutgoingRTCP(sentAt, []rtcp.Packet{&rtcp.ExtendedReport{
						SenderSSRC: origin,
						Reports:    []rtcp.ReportBlock{&rtcp.ReceiverReferenceTimeReportBlock{NTPTimestamp: reference}},
					}}, nil)
				}
				report := &rtcp.ExtendedReport{
					SenderSSRC: tc.remote,
					Reports: []rtcp.ReportBlock{&rtcp.DLRRReportBlock{Reports: []rtcp.DLRRReport{
						{SSRC: tc.local, LastRR: lastRR, DLRR: 65536},
					}}},
				}
				// Separate received reports still count even when their bytes are identical.
				for count := uint64(1); count <= 2; count++ {
					recorder.QueueIncomingRTCP(sentAt.Add(2*time.Second), mustMarshalRTCPs(t, report), nil)
					expected := RemoteOutboundRTPStreamStats{}
					if ssrc == tc.remote {
						expected.RoundTripTime = time.Second
						expected.TotalRoundTripTime = time.Duration(count) * time.Second //nolint:gosec // At most two reports.
						expected.RoundTripTimeMeasurements = count
					}
					assert.Equal(t, expected, recorder.GetStats().RemoteOutboundRTPStreamStats, "recorder SSRC %d", ssrc)
				}
			}
		})
	}
}

func TestStatsRecorder_DLRRReferenceMatch(t *testing.T) {
	sentAt := time.Unix(1700000000, 0)
	reference := ntp.ToNTP(sentAt)
	lastRR := uint32(reference >> 16 & 0xffffffff)
	for _, tc := range []struct {
		name         string
		ssrc         uint32
		lastRR       uint32
		delay        uint32
		references   int
		measurements uint64
	}{
		{"matching reference", 17, lastRR, 65536, 1, 1},
		{"wrong local SSRC", 42, lastRR, 65536, 1, 0},
		{"wrong timestamp", 17, lastRR + 1, 65536, 1, 0},
		{"zero timestamp", 17, 0, 65536, 1, 0},
		{"zero delay", 17, lastRR, 0, 1, 0},
		{"missing reference", 17, lastRR, 65536, 0, 0},
		{"evicted reference", 17, lastRR, 65536, 6, 0},
		{"latest reference", 17, lastRR + 5*65536, 65536, 6, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := newRecorder(42, 90000, logging.NewDefaultLoggerFactory())
			recorder.Start()
			defer recorder.Stop()
			for index := range tc.references {
				timestamp := ntp.ToNTP(sentAt.Add(time.Duration(index) * time.Second))
				recorder.QueueOutgoingRTCP(sentAt, []rtcp.Packet{&rtcp.ExtendedReport{
					SenderSSRC: 17,
					Reports:    []rtcp.ReportBlock{&rtcp.ReceiverReferenceTimeReportBlock{NTPTimestamp: timestamp}},
				}}, nil)
			}
			report := &rtcp.ExtendedReport{
				SenderSSRC: 42,
				Reports: []rtcp.ReportBlock{&rtcp.DLRRReportBlock{Reports: []rtcp.DLRRReport{
					{SSRC: tc.ssrc, LastRR: tc.lastRR, DLRR: tc.delay},
				}}},
			}
			recorder.QueueIncomingRTCP(sentAt.Add(7*time.Second), mustMarshalRTCPs(t, report), nil)
			assert.Equal(t, tc.measurements, recorder.GetStats().RemoteOutboundRTPStreamStats.RoundTripTimeMeasurements)
			assert.LessOrEqual(t, len(recorder.latestStats.lastReceiverReferenceTimes), 5)
		})
	}
}

func TestStatsRecorder_RTCPAfterXR(t *testing.T) {
	recorder := newRecorder(42, 90000, logging.NewDefaultLoggerFactory())
	recorder.Start()
	defer recorder.Stop()
	buf, err := rtcp.Marshal([]rtcp.Packet{
		&rtcp.ExtendedReport{SenderSSRC: 42},
		&rtcp.PictureLossIndication{SenderSSRC: 17, MediaSSRC: 42},
	})
	require.NoError(t, err)
	recorder.QueueIncomingRTCP(time.Now(), buf, nil)
	assert.Equal(t, uint32(1), recorder.GetStats().OutboundRTPStreamStats.PLICount)
}
