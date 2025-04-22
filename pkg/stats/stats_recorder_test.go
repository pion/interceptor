// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package stats

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func mustMarshalRTP(t *testing.T, pkt rtp.Packet) []byte {
	t.Helper()
	buf, err := pkt.Marshal()
	assert.NoError(t, err)

	return buf
}

func mustMarshalRTCPs(t *testing.T, pkt rtcp.Packet) []byte {
	t.Helper()
	buf, err := pkt.Marshal()
	assert.NoError(t, err)

	return buf
}

//nolint:maintidx
func TestStatsRecorder(t *testing.T) {
	cname := &rtcp.SourceDescription{
		Chunks: []rtcp.SourceDescriptionChunk{{
			Source: 1234,
			Items: []rtcp.SourceDescriptionItem{{
				Type: rtcp.SDESCNAME,
				Text: "cname",
			}},
		}},
	}
	type record struct {
		ts      time.Time
		content interface{}
	}
	type input struct {
		name string

		records []record

		expectedInboundRTPStreamStats        InboundRTPStreamStats
		expectedOutboundRTPStreamStats       OutboundRTPStreamStats
		expectedRemoteInboundRTPStreamStats  RemoteInboundRTPStreamStats
		expectedRemoteOutboundRTPStreamStats RemoteOutboundRTPStreamStats
	}
	now := time.Date(2022, time.July, 18, 0, 0, 0, 0, time.Local)
	for i, cc := range []input{
		{
			name: "basicIncomingRTP",
			records: []record{
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 7,
							Timestamp:      0,
						},
					},
				},
				{
					ts: now.Add(1 * time.Second),
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 10,
							Timestamp:      90000,
						},
					},
				},
				{
					ts: now.Add(2 * time.Second),
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 11,
							Timestamp:      2 * 90000,
						},
					},
				},
			},
			expectedInboundRTPStreamStats: InboundRTPStreamStats{
				ReceivedRTPStreamStats: ReceivedRTPStreamStats{
					PacketsReceived: 3,
					PacketsLost:     2,
					Jitter:          90000 / 16,
				},
				LastPacketReceivedTimestamp: now.Add(2 * time.Second),
				HeaderBytesReceived:         36,
				BytesReceived:               36,
			},
		},
		{
			name: "basicOutgoingRTP",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now,
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{
								SSRC: 0,
								Reports: []rtcp.ReceptionReport{
									{
										SSRC:               0,
										FractionLost:       85,
										TotalLost:          1,
										LastSequenceNumber: 3,
										Jitter:             45000,
									},
								},
							},
							cname,
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 2,
					BytesSent:   24,
				},
				HeaderBytesSent: 24,
			},
			expectedRemoteInboundRTPStreamStats: RemoteInboundRTPStreamStats{
				ReceivedRTPStreamStats: ReceivedRTPStreamStats{
					PacketsReceived: 2,
					PacketsLost:     1,
					Jitter:          0.5,
				},
				FractionLost: 0.33203125,
			},
		},
		{
			name: "issue#193",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 65535,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 0,
						},
					},
				},
				{
					ts: now,
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{
								SSRC: 0,
								Reports: []rtcp.ReceptionReport{
									{
										SSRC:               0,
										FractionLost:       0,
										TotalLost:          0,
										LastSequenceNumber: 1 << 16,
										Jitter:             45000,
									},
								},
							},
							cname,
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 2,
					BytesSent:   24,
				},
				HeaderBytesSent: 24,
			},
			expectedRemoteInboundRTPStreamStats: RemoteInboundRTPStreamStats{
				ReceivedRTPStreamStats: ReceivedRTPStreamStats{
					PacketsReceived: 2,
					PacketsLost:     0,
					Jitter:          0.5,
				},
				FractionLost: 0.0,
			},
		},
		{
			name: "basicOutgoingRTCP",
			records: []record{
				{
					ts: now,
					content: outgoingRTCP{
						ts: now,
						pkts: []rtcp.Packet{&rtcp.SenderReport{
							NTPTime: ntp.ToNTP(now),
						}},
					},
				},
				{
					ts: now.Add(2 * time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{
								SSRC: 0,
								Reports: []rtcp.ReceptionReport{{
									SSRC: 0,
									//nolint:gosec // G115
									LastSenderReport: uint32((ntp.ToNTP(now) & 0x0000FFFFFFFF0000) >> 16),
									Delay:            1 * 65536.0,
								}},
							},
							cname,
						},
					},
				},
			},
			expectedRemoteInboundRTPStreamStats: RemoteInboundRTPStreamStats{
				RoundTripTime:             time.Second,
				TotalRoundTripTime:        time.Second,
				RoundTripTimeMeasurements: 1,
			},
		},
		{
			name: "basicIncomingRTCP",
			records: []record{
				{
					ts: now,
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.SenderReport{
								NTPTime: ntp.ToNTP(now),
							},
							cname,
						},
					},
				},
			},

			expectedRemoteOutboundRTPStreamStats: RemoteOutboundRTPStreamStats{
				ReportsSent:     1,
				RemoteTimeStamp: ntp.ToTime(ntp.ToNTP(now)),
			},
		},
		{
			name: "remoteOutboundRTT",
			records: []record{
				{
					ts: now,
					content: outgoingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{
								SSRC: 9999,
								Reports: []rtcp.ReceptionReport{
									{SSRC: 0},
								},
							},
							&rtcp.ExtendedReport{
								SenderSSRC: 0,
								Reports: []rtcp.ReportBlock{
									&rtcp.ReceiverReferenceTimeReportBlock{
										NTPTimestamp: ntp.ToNTP(now),
									},
								},
							},
						},
					},
				},
				{
					ts: now.Add(2 * time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.SenderReport{
								NTPTime: ntp.ToNTP(now.Add(time.Second)),
							},
							cname,
							&rtcp.ExtendedReport{
								SenderSSRC: 0,
								Reports: []rtcp.ReportBlock{
									&rtcp.DLRRReportBlock{
										Reports: []rtcp.DLRRReport{
											{
												SSRC: 0,
												//nolint:gosec // G115
												LastRR: uint32((ntp.ToNTP(now) >> 16) & 0xFFFFFFFF),
												DLRR:   1 * 65536.0,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedRemoteOutboundRTPStreamStats: RemoteOutboundRTPStreamStats{
				RemoteTimeStamp:           now.Add(time.Second),
				ReportsSent:               1,
				RoundTripTime:             time.Second,
				TotalRoundTripTime:        time.Second,
				RoundTripTimeMeasurements: 1,
			},
		},
		{
			name: "RecordIncomingNACKAfterRR",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{},
							cname,
							&rtcp.TransportLayerNack{
								SenderSSRC: 9999,
								MediaSSRC:  0,
								Nacks:      rtcp.NackPairsFromSequenceNumbers([]uint16{2}),
							},
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 3,
					BytesSent:   36,
				},
				HeaderBytesSent: 36,
				NACKCount:       1,
			},
		},
		{
			name: "IgnoreUnknownOutgoingSSRCs",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
							SSRC:           0,
						},
					},
				},
				{
					ts: now.Add(33 * time.Millisecond),
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
							SSRC:           0,
						},
					},
				},
				{
					ts: now.Add(66 * time.Millisecond),
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
							SSRC:           0,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
							SSRC:           1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
							SSRC:           1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
							SSRC:           1,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{
								SSRC:    9999,
								Reports: []rtcp.ReceptionReport{},
							},
							cname,
							&rtcp.TransportLayerNack{
								SenderSSRC: 9999,
								MediaSSRC:  0,
								Nacks:      rtcp.NackPairsFromSequenceNumbers([]uint16{2}),
							},
							&rtcp.TransportLayerNack{
								SenderSSRC: 9999,
								MediaSSRC:  1,
								Nacks:      rtcp.NackPairsFromSequenceNumbers([]uint16{2}),
							},
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 3,
					BytesSent:   36,
				},
				HeaderBytesSent: 36,
				NACKCount:       1,
			},
		},
		{
			name: "IgnoreIncomingNACKForUnknownSSRC",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{},
							cname,
							&rtcp.TransportLayerNack{
								SenderSSRC: 9999,
								MediaSSRC:  1,
								Nacks:      rtcp.NackPairsFromSequenceNumbers([]uint16{2}),
							},
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 3,
					BytesSent:   36,
				},
				HeaderBytesSent: 36,
				NACKCount:       0,
			},
		},
		{
			name: "IgnoreIncomingFIRForUnknownSSRC",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{},
							cname,
							&rtcp.FullIntraRequest{
								SenderSSRC: 9999,
								MediaSSRC:  1,
							},
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 3,
					BytesSent:   36,
				},
				HeaderBytesSent: 36,
				FIRCount:        0,
			},
		},
		{
			name: "IgnoreIncomingPLIForUnknownSSRC",
			records: []record{
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
						},
					},
				},
				{
					ts: now,
					content: outgoingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: incomingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{},
							cname,
							&rtcp.PictureLossIndication{
								SenderSSRC: 9999,
								MediaSSRC:  1,
							},
						},
					},
				},
			},
			expectedOutboundRTPStreamStats: OutboundRTPStreamStats{
				SentRTPStreamStats: SentRTPStreamStats{
					PacketsSent: 3,
					BytesSent:   36,
				},
				HeaderBytesSent: 36,
				PLICount:        0,
			},
		},
		{
			name: "IgnoreUnknownIncomingSSRCs",
			records: []record{
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
							SSRC:           0,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
							SSRC:           0,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
							SSRC:           0,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
							SSRC:           1,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
							SSRC:           1,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
							SSRC:           1,
						},
					},
				},
			},
			expectedInboundRTPStreamStats: InboundRTPStreamStats{
				ReceivedRTPStreamStats: ReceivedRTPStreamStats{
					PacketsReceived: 3,
				},
				LastPacketReceivedTimestamp: now,
				HeaderBytesReceived:         36,
				BytesReceived:               36,
			},
		},
		{
			name: "IgnoreOutgoingNACKForUnknownSSRC",
			records: []record{
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: outgoingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{},
							cname,
							&rtcp.PictureLossIndication{
								SenderSSRC: 9999,
								MediaSSRC:  1,
							},
						},
					},
				},
			},
			expectedInboundRTPStreamStats: InboundRTPStreamStats{
				ReceivedRTPStreamStats: ReceivedRTPStreamStats{
					PacketsReceived: 3,
				},
				LastPacketReceivedTimestamp: now,
				HeaderBytesReceived:         36,
				BytesReceived:               36,
				PLICount:                    0,
			},
		},
		{
			name: "IgnoreOutgoingFIRForUnknownSSRC",
			records: []record{
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 1,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 2,
						},
					},
				},
				{
					ts: now,
					content: incomingRTP{
						header: rtp.Header{
							SequenceNumber: 3,
						},
					},
				},
				{
					ts: now.Add(time.Second),
					content: outgoingRTCP{
						pkts: []rtcp.Packet{
							&rtcp.ReceiverReport{},
							cname,
							&rtcp.FullIntraRequest{
								SenderSSRC: 9999,
								MediaSSRC:  1,
							},
						},
					},
				},
			},
			expectedInboundRTPStreamStats: InboundRTPStreamStats{
				ReceivedRTPStreamStats: ReceivedRTPStreamStats{
					PacketsReceived: 3,
				},
				LastPacketReceivedTimestamp: now,
				HeaderBytesReceived:         36,
				BytesReceived:               36,
				FIRCount:                    0,
			},
		},
	} {
		t.Run(fmt.Sprintf("%v:%v", i, cc.name), func(t *testing.T) {
			recorder := newRecorder(0, 90_000)

			recorder.Start()

			for _, record := range cc.records {
				switch v := record.content.(type) {
				case incomingRTP:
					recorder.QueueIncomingRTP(record.ts, mustMarshalRTP(t, rtp.Packet{Header: v.header}), v.attr)
				case incomingRTCP:
					pkts := make(rtcp.CompoundPacket, len(v.pkts))
					copy(pkts, v.pkts)
					recorder.QueueIncomingRTCP(record.ts, mustMarshalRTCPs(t, &pkts), v.attr)
				case outgoingRTP:
					recorder.QueueOutgoingRTP(record.ts, &v.header, []byte{}, v.attr)
				case outgoingRTCP:
					recorder.QueueOutgoingRTCP(record.ts, v.pkts, v.attr)
				default:
					assert.FailNow(t, "invalid test case")
				}
			}

			s := recorder.GetStats()

			recorder.Stop()

			assert.Equal(t, cc.expectedInboundRTPStreamStats, s.InboundRTPStreamStats)
			assert.Equal(t, cc.expectedOutboundRTPStreamStats, s.OutboundRTPStreamStats)
			assert.Equal(t, cc.expectedRemoteInboundRTPStreamStats, s.RemoteInboundRTPStreamStats)
			assert.Equal(t, cc.expectedRemoteOutboundRTPStreamStats, s.RemoteOutboundRTPStreamStats)
		})
	}
}

func TestStatsRecorder_DLRR_Precision(t *testing.T) {
	recorder := newRecorder(0, 90_000)

	report := &rtcp.ExtendedReport{
		Reports: []rtcp.ReportBlock{
			&rtcp.DLRRReportBlock{
				Reports: []rtcp.DLRRReport{
					{
						SSRC:   5000,
						LastRR: 762,
						DLRR:   30000,
					},
				},
			},
		},
	}

	s := recorder.recordIncomingXR(internalStats{
		lastReceiverReferenceTimes: []uint64{50000000},
	}, report, time.Time{})

	assert.Equal(t, int64(s.RemoteOutboundRTPStreamStats.RoundTripTime), int64(-9223372036854775808))
}

func TestGetStatsNotBlocking(t *testing.T) {
	r := newRecorder(0, 90_000)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go func() {
		defer cancel()
		r.Start()
		r.GetStats()
	}()
	go r.Stop()

	<-ctx.Done()

	assert.False(t, errors.Is(ctx.Err(), context.DeadlineExceeded), "it shouldn't block")
}

func TestQueueNotBlocking(t *testing.T) {
	for _, testCase := range []struct {
		f    func(r *recorder)
		name string
	}{
		{
			f: func(r *recorder) {
				r.QueueIncomingRTP(time.Now(), mustMarshalRTP(t, rtp.Packet{}), interceptor.Attributes{})
			},
			name: "QueueIncomingRTP",
		},
		{
			f: func(r *recorder) {
				r.QueueOutgoingRTP(time.Now(), &rtp.Header{}, mustMarshalRTP(t, rtp.Packet{}), interceptor.Attributes{})
			},
			name: "QueueOutgoingRTP",
		},
		{
			f: func(r *recorder) {
				r.QueueIncomingRTCP(time.Now(), mustMarshalRTCPs(t, &rtcp.CCFeedbackReport{}), interceptor.Attributes{})
			},
			name: "QueueIncomingRTCP",
		},
		{
			f: func(r *recorder) {
				r.QueueOutgoingRTCP(time.Now(), []rtcp.Packet{}, interceptor.Attributes{})
			},
			name: "QueueOutgoingRTCP",
		},
	} {
		t.Run(testCase.name+"NotBlocking", func(t *testing.T) {
			r := newRecorder(0, 90_000)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			go func() {
				defer cancel()
				r.Start()
				testCase.f(r)
			}()
			go r.Stop()

			<-ctx.Done()

			assert.False(t, errors.Is(ctx.Err(), context.DeadlineExceeded), "it shouldn't block")
		})
	}
}

func TestContains(t *testing.T) {
	for i, tc := range []struct {
		list     []uint32
		element  uint32
		expected bool
	}{
		{list: []uint32{}, element: 0, expected: false},
		{list: []uint32{0}, element: 0, expected: true},
		{list: []uint32{0, 1, 2}, element: 1, expected: true},
		{list: []uint32{0, 1, 2}, element: 3, expected: false},
	} {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, tc.expected, contains(tc.list, tc.element))
		})
	}
}
