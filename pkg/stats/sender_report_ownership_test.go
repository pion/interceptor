// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package stats

import (
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/stretchr/testify/require"
)

func TestStatsIncomingSenderReportOwnership(t *testing.T) {
	statsInterceptor := newTestInterceptor(t, nil)
	for _, ssrc := range []uint32{7, 8} {
		statsInterceptor.BindRemoteStream(&interceptor.StreamInfo{SSRC: ssrc, ClockRate: 90000}, nil)
	}
	statsInterceptor.BindLocalStream(&interceptor.StreamInfo{SSRC: 9, ClockRate: 90000}, nil)
	statsInterceptor.wg.Wait()
	remoteTime := time.Unix(1234, 0)
	for index, ssrc := range []uint32{7, 8, 7} {
		packet := &rtcp.SenderReport{
			SSRC: ssrc, NTPTime: ntp.ToNTP(remoteTime.Add(time.Duration(index) * time.Second)),
			PacketCount: uint32(index * 10), OctetCount: uint32(index * 100), //nolint:gosec // Three bounded cases.
			Reports: []rtcp.ReceptionReport{{SSRC: 15 - ssrc, TotalLost: 2}, {SSRC: 9, TotalLost: 3}},
		}
		otherBefore := statsInterceptor.Get(15 - ssrc).RemoteOutboundRTPStreamStats
		wire, err := packet.Marshal()
		require.NoError(t, err)
		reader := statsInterceptor.BindRTCPReader(interceptor.RTCPReaderFunc(
			func(buf []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
				return copy(buf, wire), attributes, nil
			},
		))
		_, _, err = reader.Read(make([]byte, len(wire)), nil)
		require.NoError(t, err)
		got := statsInterceptor.Get(ssrc).RemoteOutboundRTPStreamStats
		require.Equal(t, uint64(packet.PacketCount), got.PacketsSent)
		require.Equal(t, uint64(packet.OctetCount), got.BytesSent)
		require.Equal(t, remoteTime.Add(time.Duration(index)*time.Second), got.RemoteTimeStamp)
		require.Equal(t, uint64(index/2+1), got.ReportsSent) //nolint:gosec // Three bounded cases.
		require.Equal(t, otherBefore, statsInterceptor.Get(15-ssrc).RemoteOutboundRTPStreamStats)
		require.Equal(t, RemoteOutboundRTPStreamStats{}, statsInterceptor.Get(9).RemoteOutboundRTPStreamStats)
		require.Equal(t, int64(2), statsInterceptor.Get(15-ssrc).RemoteInboundRTPStreamStats.PacketsLost)
		require.Equal(t, int64(3), statsInterceptor.Get(9).RemoteInboundRTPStreamStats.PacketsLost)
	}
}

func TestStatsOutgoingSenderReportOwnership(t *testing.T) {
	statsInterceptor := newTestInterceptor(t, nil)
	for _, ssrc := range []uint32{7, 8} {
		statsInterceptor.BindLocalStream(&interceptor.StreamInfo{SSRC: ssrc, ClockRate: 90000}, nil)
	}
	statsInterceptor.wg.Wait()
	sentAt := time.Unix(1234, 0)
	statsInterceptor.now = func() time.Time { return sentAt.Add(2 * time.Second) }
	writer := statsInterceptor.BindRTCPWriter(interceptor.RTCPWriterFunc(
		func(packets []rtcp.Packet, _ interceptor.Attributes) (int, error) { return len(packets), nil },
	))
	for _, ssrc := range []uint32{7, 8} {
		ntpTime := ntp.ToNTP(sentAt)
		_, err := writer.Write([]rtcp.Packet{&rtcp.SenderReport{
			SSRC: ssrc, NTPTime: ntpTime,
			Reports: []rtcp.ReceptionReport{{SSRC: 15 - ssrc}},
		}}, nil)
		require.NoError(t, err)
		wire, err := (&rtcp.ReceiverReport{SSRC: 9, Reports: []rtcp.ReceptionReport{
			{SSRC: 7, LastSenderReport: uint32(ntpTime >> 16 & 0xffffffff), Delay: 65536},
			{SSRC: 8, LastSenderReport: uint32(ntpTime >> 16 & 0xffffffff), Delay: 65536},
		}}).Marshal()
		require.NoError(t, err)
		reader := statsInterceptor.BindRTCPReader(interceptor.RTCPReaderFunc(
			func(buf []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
				return copy(buf, wire), attributes, nil
			},
		))
		_, _, err = reader.Read(make([]byte, len(wire)), nil)
		require.NoError(t, err)
		own := statsInterceptor.Get(ssrc).RemoteInboundRTPStreamStats
		require.Equal(t, time.Second, own.RoundTripTime)
		require.Equal(t, uint64(1), own.RoundTripTimeMeasurements)
		other := statsInterceptor.Get(15 - ssrc).RemoteInboundRTPStreamStats
		if ssrc == 7 {
			require.Zero(t, other.RoundTripTimeMeasurements)
			require.Zero(t, other.RoundTripTime)
		} else {
			require.Equal(t, uint64(1), other.RoundTripTimeMeasurements)
			require.Equal(t, time.Second, other.TotalRoundTripTime)
		}
		sentAt = sentAt.Add(time.Second)
	}
}
