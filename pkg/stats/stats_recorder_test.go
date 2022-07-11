package stats

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func mustMarshalRTP(t *testing.T, pkt rtp.Packet) []byte {
	buf, err := pkt.Marshal()
	assert.NoError(t, err)
	return buf
}

func mustMarshalRTCPs(t *testing.T, pkt rtcp.Packet) []byte {
	buf, err := pkt.Marshal()
	assert.NoError(t, err)
	return buf
}

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
									SSRC:             0,
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
							&rtcp.ReceiverReport{},
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
												SSRC:   0,
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
	} {
		t.Run(fmt.Sprintf("%v:%v", i, cc.name), func(t *testing.T) {
			r := newRecorder(0, 90_000)

			go r.Start()
			defer r.Stop()

			for _, record := range cc.records {
				switch v := record.content.(type) {
				case incomingRTP:
					r.QueueIncomingRTP(record.ts, mustMarshalRTP(t, rtp.Packet{Header: v.header}), v.attr)
				case incomingRTCP:
					pkts := make(rtcp.CompoundPacket, len(v.pkts))
					copy(pkts, v.pkts)
					r.QueueIncomingRTCP(record.ts, mustMarshalRTCPs(t, &pkts), v.attr)
				case outgoingRTP:
					r.QueueOutgoingRTP(record.ts, &v.header, []byte{}, v.attr)
				case outgoingRTCP:
					r.QueueOutgoingRTCP(record.ts, v.pkts, v.attr)
				default:
					assert.FailNow(t, "invalid test case")
				}
			}

			s := r.GetStats()

			assert.Equal(t, cc.expectedInboundRTPStreamStats, s.InboundRTPStreamStats)
			assert.Equal(t, cc.expectedOutboundRTPStreamStats, s.OutboundRTPStreamStats)
			assert.Equal(t, cc.expectedRemoteInboundRTPStreamStats, s.RemoteInboundRTPStreamStats)
			assert.Equal(t, cc.expectedRemoteOutboundRTPStreamStats, s.RemoteOutboundRTPStreamStats)
		})
	}
}
