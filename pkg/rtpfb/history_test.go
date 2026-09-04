// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHistoryCCFB(t *testing.T) {
	cases := []struct {
		packets        []uint16 // RTP sequence number
		feedbackFirst  []acknowledgement
		feedbackSecond []acknowledgement
		expectedFirst  []PacketReport
		expectedSecond []PacketReport
	}{
		{
			packets: []uint16{0, 1, 2, 3},
			feedbackFirst: []acknowledgement{
				{sequenceNumber: 0, arrived: true},
				{sequenceNumber: 1, arrived: true},
				{sequenceNumber: 2, arrived: true},
			},
			expectedFirst: []PacketReport{
				{SSRC: 1, SequenceNumber: 0, RTPSequenceNumber: 0, Arrived: true},
				{SSRC: 1, SequenceNumber: 1, RTPSequenceNumber: 1, Arrived: true},
				{SSRC: 1, SequenceNumber: 2, RTPSequenceNumber: 2, Arrived: true},
			},
		},
		{
			packets: []uint16{5, 6, 7, 8, 9},
			feedbackFirst: []acknowledgement{
				{sequenceNumber: 5, arrived: true},
				{sequenceNumber: 6, arrived: false},
				{sequenceNumber: 7, arrived: true},
			},
			expectedFirst: []PacketReport{
				{SSRC: 1, SequenceNumber: 0, RTPSequenceNumber: 5, Arrived: true},
				{SSRC: 1, SequenceNumber: 1, RTPSequenceNumber: 6, Arrived: false},
				{SSRC: 1, SequenceNumber: 2, RTPSequenceNumber: 7, Arrived: true},
			},
		},
		{
			packets: []uint16{1, 2, 3, 4, 5},
			feedbackFirst: []acknowledgement{
				{sequenceNumber: 1, arrived: true},
				{sequenceNumber: 2, arrived: true},
			},
			feedbackSecond: []acknowledgement{
				{sequenceNumber: 3, arrived: true},
				{sequenceNumber: 4, arrived: true},
				{sequenceNumber: 5, arrived: true},
			},
			expectedFirst: []PacketReport{
				{SSRC: 1, SequenceNumber: 0, RTPSequenceNumber: 1, Arrived: true},
				{SSRC: 1, SequenceNumber: 1, RTPSequenceNumber: 2, Arrived: true},
			},
			expectedSecond: []PacketReport{
				{SSRC: 1, SequenceNumber: 2, RTPSequenceNumber: 3, Arrived: true},
				{SSRC: 1, SequenceNumber: 3, RTPSequenceNumber: 4, Arrived: true},
				{SSRC: 1, SequenceNumber: 4, RTPSequenceNumber: 5, Arrived: true},
			},
		},
		{
			packets: []uint16{1, 2, 3, 4, 5},
			feedbackFirst: []acknowledgement{
				{sequenceNumber: 1, arrived: true},
				{sequenceNumber: 2, arrived: true},
				{sequenceNumber: 3, arrived: true},
			},
			feedbackSecond: []acknowledgement{
				{sequenceNumber: 1, arrived: true},
				{sequenceNumber: 2, arrived: true},
				{sequenceNumber: 3, arrived: true},
				{sequenceNumber: 4, arrived: true},
			},
			expectedFirst: []PacketReport{
				{SSRC: 1, SequenceNumber: 0, RTPSequenceNumber: 1, Arrived: true},
				{SSRC: 1, SequenceNumber: 1, RTPSequenceNumber: 2, Arrived: true},
				{SSRC: 1, SequenceNumber: 2, RTPSequenceNumber: 3, Arrived: true},
			},
			expectedSecond: []PacketReport{
				{SSRC: 1, SequenceNumber: 3, RTPSequenceNumber: 4, Arrived: true},
			},
		},
		{
			packets: []uint16{65534, 65535, 0, 1},
			feedbackFirst: []acknowledgement{
				{sequenceNumber: 65534, arrived: true},
				{sequenceNumber: 65535, arrived: true},
				{sequenceNumber: 0, arrived: true},
				{sequenceNumber: 1, arrived: true},
			},
			feedbackSecond: []acknowledgement{},
			expectedFirst: []PacketReport{
				{SSRC: 1, SequenceNumber: 0, RTPSequenceNumber: 65534, Arrived: true},
				{SSRC: 1, SequenceNumber: 1, RTPSequenceNumber: 65535, Arrived: true},
				{SSRC: 1, SequenceNumber: 2, RTPSequenceNumber: 0, Arrived: true},
				{SSRC: 1, SequenceNumber: 3, RTPSequenceNumber: 1, Arrived: true},
			},
			expectedSecond: nil,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			history := newHistory()
			for _, p := range tc.packets {
				history.addOutgoing(1, p, false, 0, 0, time.Time{})
			}
			for _, f := range tc.feedbackFirst {
				history.onCCFBFeedback(time.Time{}, 1, f)
			}
			reports := history.buildReport()
			assert.Equal(t, tc.expectedFirst, reports)

			for _, f := range tc.feedbackSecond {
				history.onCCFBFeedback(time.Time{}, 1, f)
			}
			reports = history.buildReport()
			assert.Equal(t, tc.expectedSecond, reports)
		})
	}
}

func TestHistoryPrunesPackets(t *testing.T) {
	history := newHistory()
	for i := range uint16(10) {
		history.addOutgoing(1, i, false, 0, 0, time.Time{})
	}
	for i := range uint16(10) {
		history.onCCFBFeedback(time.Time{}, 1, acknowledgement{sequenceNumber: i, arrived: true})
	}
	reports := history.buildReport()
	assert.Len(t, reports, 10)
	assert.Empty(t, history.packets)
}

func TestHistoryCleanUntilDoesNotUnderflow(t *testing.T) {
	history := newHistory()
	reports := history.buildReport()
	assert.Empty(t, reports)
	assert.Equal(t, uint64(0), history.cleanUntil)

	for i := range uint16(5) {
		history.addOutgoing(1, i, false, 0, 0, time.Time{})
	}
	for i := range uint16(5) {
		history.onCCFBFeedback(time.Time{}, 1, acknowledgement{sequenceNumber: i, arrived: true})
	}
	reports = history.buildReport()
	assert.Len(t, reports, 5)
	assert.Equal(t, uint64(5), history.cleanUntil)
}

func TestHistorySetsIsTWCC(t *testing.T) {
	history := newHistory()
	for i := range uint16(10) {
		history.addOutgoing(1, i, true, i, 0, time.Time{})
	}
	for i := range uint16(10) {
		history.onTWCCFeedback(time.Time{}, acknowledgement{sequenceNumber: i, arrived: true})
	}
	reports := history.buildReport()
	for _, r := range reports {
		assert.True(t, r.IsTWCC)
	}
	assert.Empty(t, history.twccToCounter)
}

// TestHistoryConcurrentFeedback verifies that concurrent calls to onTWCCFeedback
// do not race on shared state, since onFeedback writes h.highestAcked and fields
// on the shared *PacketReport.
func TestHistoryConcurrentFeedback(t *testing.T) {
	history := newHistory()
	const packetCount = 100
	for i := range uint16(packetCount) {
		history.addOutgoing(1, i, true, i, 0, time.Time{})
	}

	var wg sync.WaitGroup
	for i := range uint16(packetCount) {
		wg.Add(1)
		go func(seqNr uint16) {
			defer wg.Done()
			history.onTWCCFeedback(time.Time{}, acknowledgement{sequenceNumber: seqNr, arrived: true})
		}(i)
	}
	wg.Wait()

	assert.Equal(t, uint64(packetCount-1), history.highestAcked)
}
