// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/ntp"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

type ackListEntry struct {
	ts   time.Time
	ssrc uint32
	ack  acknowledgement
}

type mockHistory struct {
	log  []PacketReport
	acks []ackListEntry
}

// addOutgoing implements packetLog.
func (m *mockHistory) addOutgoing(
	ssrc uint32,
	rtpSequenceNumber uint16,
	isTWCC bool,
	twccSequenceNumber uint16,
	size int,
	departure time.Time,
) {
	m.log = append(m.log, PacketReport{
		SSRC:               ssrc,
		RTPSequenceNumber:  rtpSequenceNumber,
		IsTWCC:             isTWCC,
		TWCCSequenceNumber: twccSequenceNumber,
		Size:               size,
		Departure:          departure,
	})
}

// buildReport implements packetLog.
func (m *mockHistory) buildReport() []PacketReport {
	return nil
}

// onCCFBFeedback implements packetLog.
func (m *mockHistory) onCCFBFeedback(ts time.Time, ssrc uint32, ack acknowledgement) (time.Duration, bool) {
	m.acks = append(m.acks, ackListEntry{
		ts:   ts,
		ssrc: ssrc,
		ack:  ack,
	})

	return 0, true
}

// onTWCCFeedback implements packetLog.
func (m *mockHistory) onTWCCFeedback(ts time.Time, ack acknowledgement) (time.Duration, bool) {
	m.acks = append(m.acks, ackListEntry{
		ts:   ts,
		ssrc: 0,
		ack:  ack,
	})

	return 0, true
}

func TestInterceptor(t *testing.T) {
	mockTimeStamp := time.Time{}.Add(120 * time.Second)
	t.Run("calls_add_outgoing", func(t *testing.T) {
		cases := []struct {
			twcc     bool
			packets  []uint16
			expected []PacketReport
		}{
			{
				twcc:     false,
				packets:  []uint16{},
				expected: []PacketReport{},
			},
			{
				twcc:    false,
				packets: []uint16{7, 8, 9},
				expected: []PacketReport{
					{SequenceNumber: 0, RTPSequenceNumber: 7, Size: 12, Departure: mockTimeStamp},
					{SequenceNumber: 0, RTPSequenceNumber: 8, Size: 12, Departure: mockTimeStamp},
					{SequenceNumber: 0, RTPSequenceNumber: 9, Size: 12, Departure: mockTimeStamp},
				},
			},
			{
				twcc:     true,
				packets:  []uint16{},
				expected: []PacketReport{},
			},
			{
				twcc:    true,
				packets: []uint16{7, 8, 9},
				expected: []PacketReport{
					{SequenceNumber: 0, RTPSequenceNumber: 7, IsTWCC: true, TWCCSequenceNumber: 7, Size: 20, Departure: mockTimeStamp},
					{SequenceNumber: 0, RTPSequenceNumber: 8, IsTWCC: true, TWCCSequenceNumber: 8, Size: 20, Departure: mockTimeStamp},
					{SequenceNumber: 0, RTPSequenceNumber: 9, IsTWCC: true, TWCCSequenceNumber: 9, Size: 20, Departure: mockTimeStamp},
				},
			},
		}

		for i, tc := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				mh := &mockHistory{
					log: []PacketReport{},
				}
				mt := func() time.Time {
					return mockTimeStamp
				}
				f, err := NewInterceptor(timeFactory(mt), setHistory(mh))
				assert.NoError(t, err)
				i, err := f.NewInterceptor("")
				assert.NoError(t, err)

				info := &interceptor.StreamInfo{}
				if tc.twcc {
					info.RTPHeaderExtensions = append(info.RTPHeaderExtensions, interceptor.RTPHeaderExtension{
						URI: transportCCURI,
						ID:  2,
					})
				}
				stream := test.NewMockStream(info, i)

				for _, pkt := range tc.packets {
					packet := &rtp.Packet{Header: rtp.Header{SequenceNumber: pkt}}
					if tc.twcc {
						ext := rtp.TransportCCExtension{
							TransportSequence: pkt,
						}
						var buf []byte
						buf, err = ext.Marshal()
						assert.NoError(t, err)
						err = packet.SetExtension(2, buf)
						assert.NoError(t, err)
					}
					err = stream.WriteRTP(packet)
					assert.NoError(t, err)
				}

				assert.Equal(t, tc.expected, mh.log)
			})
		}
	})

	t.Run("calls_on_feedback", func(t *testing.T) {
		cases := []struct {
			feedback rtcp.Packet
			expected []ackListEntry
		}{
			{
				feedback: &rtcp.CCFeedbackReport{
					SenderSSRC: 0,
					ReportBlocks: []rtcp.CCFeedbackReportBlock{
						{
							MediaSSRC:     0,
							BeginSequence: 17,
							MetricBlocks: []rtcp.CCFeedbackMetricBlock{
								{
									Received:          true,
									ECN:               0,
									ArrivalTimeOffset: 0,
								},
							},
						},
					},
					ReportTimestamp: 0,
				},
				expected: []ackListEntry{
					{
						ts:   mockTimeStamp,
						ssrc: 0,
						ack: acknowledgement{
							sequenceNumber: 17,
							arrived:        true,
							arrival:        ntp.ToTime32(0, mockTimeStamp),
							ecn:            0,
						},
					},
				},
			},
		}
		for i, tc := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				mh := &mockHistory{
					log:  []PacketReport{},
					acks: []ackListEntry{},
				}
				mt := func() time.Time {
					return mockTimeStamp
				}
				f, err := NewInterceptor(timeFactory(mt), setHistory(mh))
				assert.NoError(t, err)
				i, err := f.NewInterceptor("")
				assert.NoError(t, err)

				info := &interceptor.StreamInfo{}
				stream := test.NewMockStream(info, i)

				stream.ReceiveRTCP([]rtcp.Packet{tc.feedback})
				<-stream.ReadRTCP()

				assert.Equal(t, tc.expected, mh.acks)
			})
		}
	})
}
