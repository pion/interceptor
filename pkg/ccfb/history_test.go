package ccfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHistory(t *testing.T) {
	t.Run("errorOnDecreasingSeqNr", func(t *testing.T) {
		h := newHistoryList(200)
		assert.NoError(t, h.add(10, 1200, time.Now()))
		assert.NoError(t, h.add(11, 1200, time.Now()))
		assert.Error(t, h.add(9, 1200, time.Now()))
	})

	t.Run("getReportForAck", func(t *testing.T) {
		cases := []struct {
			outgoing []struct {
				seqNr uint16
				size  int
				ts    time.Time
			}
			acks                []acknowledgement
			expectedReport      []PacketReport
			expectedHistorySize int
		}{
			{
				outgoing: []struct {
					seqNr uint16
					size  int
					ts    time.Time
				}{},
				acks:                []acknowledgement{},
				expectedReport:      []PacketReport{},
				expectedHistorySize: 0,
			},
			{
				outgoing: []struct {
					seqNr uint16
					size  int
					ts    time.Time
				}{
					{0, 1200, time.Time{}.Add(1 * time.Millisecond)},
					{1, 1200, time.Time{}.Add(2 * time.Millisecond)},
					{2, 1200, time.Time{}.Add(3 * time.Millisecond)},
					{3, 1200, time.Time{}.Add(4 * time.Millisecond)},
				},
				acks:                []acknowledgement{},
				expectedReport:      []PacketReport{},
				expectedHistorySize: 4,
			},
			{
				outgoing: []struct {
					seqNr uint16
					size  int
					ts    time.Time
				}{
					{0, 1200, time.Time{}.Add(1 * time.Millisecond)},
					{1, 1200, time.Time{}.Add(2 * time.Millisecond)},
					{2, 1200, time.Time{}.Add(3 * time.Millisecond)},
					{3, 1200, time.Time{}.Add(4 * time.Millisecond)},
				},
				acks: []acknowledgement{
					{1, true, time.Time{}.Add(3 * time.Millisecond), 0},
					{2, false, time.Time{}, 0},
					{3, true, time.Time{}.Add(5 * time.Millisecond), 0},
				},
				expectedReport: []PacketReport{
					{1, 1200, time.Time{}.Add(2 * time.Millisecond), true, time.Time{}.Add(3 * time.Millisecond), 0},
					{2, 1200, time.Time{}.Add(3 * time.Millisecond), false, time.Time{}, 0},
					{3, 1200, time.Time{}.Add(4 * time.Millisecond), true, time.Time{}.Add(5 * time.Millisecond), 0},
				},
				expectedHistorySize: 4,
			},
		}
		for i, tc := range cases {
			t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
				h := newHistoryList(200)
				for _, op := range tc.outgoing {
					assert.NoError(t, h.add(op.seqNr, op.size, op.ts))
				}
				prl := h.getReportForAck(tc.acks)
				assert.Equal(t, tc.expectedReport, prl)
				assert.Equal(t, tc.expectedHistorySize, len(h.seqNrToPacket))
				assert.Equal(t, tc.expectedHistorySize, h.evictList.Len())
			})
		}
	})

	t.Run("garbageCollection", func(t *testing.T) {
		hist := newHistoryList(200)

		for i := uint16(0); i < 300; i++ {
			assert.NoError(t, hist.add(i, 1200, time.Time{}.Add(time.Duration(i)*time.Millisecond)))
		}

		acks := []acknowledgement{}
		for i := uint16(200); i < 290; i++ {
			acks = append(acks, acknowledgement{
				seqNr:   i,
				arrived: true,
				arrival: time.Time{}.Add(time.Duration(500+i) * time.Millisecond),
				ecn:     0,
			})
		}
		prl := hist.getReportForAck(acks)
		assert.Len(t, prl, 90)
		assert.Equal(t, 200, len(hist.seqNrToPacket))
		assert.Equal(t, 200, hist.evictList.Len())
	})
}
