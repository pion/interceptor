package ccfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/internal/test"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

type mockHistoryAddEntry struct {
	seqNr     uint16
	size      int
	departure time.Time
}

type mockHistory struct {
	added  []mockHistoryAddEntry
	report []PacketReport
}

// add implements history.
func (m *mockHistory) add(seqNr uint16, size int, departure time.Time) error {
	m.added = append(m.added, mockHistoryAddEntry{
		seqNr:     seqNr,
		size:      size,
		departure: departure,
	})
	return nil
}

// getReportForAck implements history.
func (m *mockHistory) getReportForAck(_ []acknowledgement) []PacketReport {
	return m.report
}

func TestInterceptor(t *testing.T) {
	mockTimestamp := time.Time{}.Add(17 * time.Second)
	t.Run("writeRTP", func(t *testing.T) {
		type addPkt struct {
			pkt *rtp.Packet
			ext *rtp.TransportCCExtension
		}
		cases := []struct {
			add    []addPkt
			twcc   bool
			expect *mockHistory
		}{
			{
				add: []addPkt{},
				expect: &mockHistory{
					added: []mockHistoryAddEntry{},
				},
			},
			{
				add: []addPkt{
					{
						pkt: &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								SequenceNumber: 137,
							},
						},
					},
				},
				expect: &mockHistory{
					added: []mockHistoryAddEntry{
						{137, 12, mockTimestamp},
					},
				},
			},
			{
				add: []addPkt{
					{
						pkt: &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								SequenceNumber: 137,
							},
						},
						ext: &rtp.TransportCCExtension{
							TransportSequence: 16,
						},
					},
				},
				twcc: true,
				expect: &mockHistory{
					added: []mockHistoryAddEntry{
						{16, 20, mockTimestamp},
					},
				},
			},
		}
		for i, tc := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				mt := func() time.Time {
					return mockTimestamp
				}
				mh := &mockHistory{
					added: []mockHistoryAddEntry{},
				}
				f, err := NewInterceptor(
					historyFactory(func(_ int) history {
						return mh
					}),
					timeFactory(mt),
				)
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

				for _, pkt := range tc.add {
					if pkt.ext != nil {
						ext, err := pkt.ext.Marshal()
						assert.NoError(t, err)
						assert.NoError(t, pkt.pkt.SetExtension(2, ext))
					}
					assert.NoError(t, stream.WriteRTP(pkt.pkt))
				}

				assert.Equal(t, tc.expect, mh)
			})
		}
	})

	t.Run("missingTWCCHeaderExtension", func(t *testing.T) {
		mt := func() time.Time {
			return mockTimestamp
		}
		mh := &mockHistory{
			added: []mockHistoryAddEntry{},
		}
		f, err := NewInterceptor(
			historyFactory(func(_ int) history {
				return mh
			}),
			timeFactory(mt),
		)
		assert.NoError(t, err)

		i, err := f.NewInterceptor("")
		assert.NoError(t, err)

		info := &interceptor.StreamInfo{}
		info.RTPHeaderExtensions = append(info.RTPHeaderExtensions, interceptor.RTPHeaderExtension{
			URI: transportCCURI,
			ID:  2,
		})
		stream := test.NewMockStream(info, i)

		err = stream.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{SequenceNumber: 3},
			Payload: []byte{},
		})
		assert.NoError(t, err)
		assert.Equal(t, mh.added, []mockHistoryAddEntry{{
			seqNr:     3,
			size:      12,
			departure: mockTimestamp,
		}})
	})

	t.Run("readRTCP", func(t *testing.T) {
		cases := []struct {
			mh   *mockHistory
			rtcp rtcp.Packet
		}{
			{
				mh: &mockHistory{
					report: []PacketReport{},
				},
				rtcp: &rtcp.CCFeedbackReport{},
			},
			{
				mh: &mockHistory{
					report: []PacketReport{
						{
							SeqNr:     3,
							Size:      12,
							Departure: mockTimestamp,
							Arrived:   true,
							Arrival:   mockTimestamp,
							ECN:       0,
						},
					},
				},
				rtcp: &rtcp.CCFeedbackReport{},
			},
			{
				mh: &mockHistory{
					report: []PacketReport{},
				},
				rtcp: &rtcp.TransportLayerCC{
					Header: rtcp.Header{
						Padding: false,
						Count:   rtcp.FormatTCC,
						Type:    rtcp.TypeTransportSpecificFeedback,
						Length:  6,
					},
					SenderSSRC:         1,
					MediaSSRC:          2,
					BaseSequenceNumber: 3,
					PacketStatusCount:  0,
					ReferenceTime:      5,
					FbPktCount:         6,
					PacketChunks: []rtcp.PacketStatusChunk{
						&rtcp.RunLengthChunk{
							Type:               rtcp.RunLengthChunkType,
							PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
							RunLength:          3,
						},
					},
					RecvDeltas: []*rtcp.RecvDelta{
						{Type: 0, Delta: 0},
						{Type: 0, Delta: 0},
						{Type: 0, Delta: 0},
					},
				},
			},
			{
				mh: &mockHistory{
					report: []PacketReport{
						{
							SeqNr:     3,
							Size:      12,
							Departure: mockTimestamp,
							Arrived:   true,
							Arrival:   mockTimestamp,
							ECN:       0,
						},
					},
				},
				rtcp: &rtcp.TransportLayerCC{
					Header: rtcp.Header{
						Padding: false,
						Count:   rtcp.FormatTCC,
						Type:    rtcp.TypeTransportSpecificFeedback,
						Length:  6,
					},
					SenderSSRC:         0,
					MediaSSRC:          0,
					BaseSequenceNumber: 0,
					PacketStatusCount:  0,
					ReferenceTime:      0,
					FbPktCount:         0,
					PacketChunks: []rtcp.PacketStatusChunk{
						&rtcp.RunLengthChunk{
							Type:               rtcp.RunLengthChunkType,
							PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
							RunLength:          3,
						},
					},
					RecvDeltas: []*rtcp.RecvDelta{
						{Type: 0, Delta: 0},
						{Type: 0, Delta: 0},
						{Type: 0, Delta: 0},
					},
				},
			},
		}
		for i, tc := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				mt := func() time.Time {
					return mockTimestamp
				}
				mockCCFBConverter := func(_ time.Time, _ *rtcp.CCFeedbackReport) (time.Time, map[uint32][]acknowledgement) {
					return mockTimestamp, map[uint32][]acknowledgement{
						0: {},
					}
				}
				mockTWCCConverter := func(_ *rtcp.TransportLayerCC) (time.Time, map[uint32][]acknowledgement) {
					return mockTimestamp, map[uint32][]acknowledgement{
						0: {},
					}
				}
				f, err := NewInterceptor(
					historyFactory(func(_ int) history {
						return tc.mh
					}),
					timeFactory(mt),
					ccfbConverterFactory(mockCCFBConverter),
					twccConverterFactory(mockTWCCConverter),
				)
				assert.NoError(t, err)

				i, err := f.NewInterceptor("")
				assert.NoError(t, err)

				info := &interceptor.StreamInfo{}
				if _, ok := tc.rtcp.(*rtcp.TransportLayerCC); ok {
					info.RTPHeaderExtensions = append(info.RTPHeaderExtensions, interceptor.RTPHeaderExtension{
						URI: transportCCURI,
						ID:  2,
					})
				}
				stream := test.NewMockStream(info, i)

				stream.ReceiveRTCP([]rtcp.Packet{tc.rtcp})

				report := <-stream.ReadRTCP()

				assert.NoError(t, report.Err)

				prlsInterface, ok := report.Attr[CCFBAttributesKey]
				assert.True(t, ok)
				prls, ok := prlsInterface.([]Report)
				assert.True(t, ok)
				assert.Len(t, prls, 1)
				assert.Equal(t, tc.mh.report, prls[0].SSRCToPacketReports[0])
			})
		}
	})
}
