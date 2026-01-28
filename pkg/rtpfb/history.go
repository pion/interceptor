// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpfb

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
)

type ssrcSequenceNumber struct {
	ssrc           uint32
	sequenceNumber uint16
}

// history keeps a global sequence number for all outgoing packets, called
// counter, to avoid confusion with transport wide sequence numbers from TWCC.
// Packets will be mapped to the counter either by their TWCC sequence number or
// by their combination of RTP sequence number and SSRC. When feedback arrives,
// calls to onFeedback will update the status of each packet included in the
// report. buildReport can be used to create a new report including all packets
// from nextReport to highestAcked.
type history struct {
	lock               sync.RWMutex
	counter            uint64
	twccToCounter      map[uint16]uint64
	ssrcSeqNrToCounter map[ssrcSequenceNumber]uint64

	packets      map[uint64]*PacketReport
	highestAcked uint64
	nextReport   uint64

	cleanUntil uint64
}

func newHistory() *history {
	return &history{
		lock:               sync.RWMutex{},
		counter:            0,
		twccToCounter:      map[uint16]uint64{},
		ssrcSeqNrToCounter: map[ssrcSequenceNumber]uint64{},
		packets:            make(map[uint64]*PacketReport),
		highestAcked:       0,
		nextReport:         0,
		cleanUntil:         0,
	}
}

func (h *history) addOutgoing(
	ssrc uint32,
	rtpSequenceNumber uint16,
	isTWCC bool,
	twccSequenceNumber uint16,
	size int,
	departure time.Time,
) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if isTWCC {
		h.twccToCounter[twccSequenceNumber] = h.counter
	} else {
		h.ssrcSeqNrToCounter[ssrcSequenceNumber{
			ssrc:           ssrc,
			sequenceNumber: rtpSequenceNumber,
		}] = h.counter
	}
	h.packets[h.counter] = &PacketReport{
		SSRC:               ssrc,
		SequenceNumber:     h.counter,
		RTPSequenceNumber:  rtpSequenceNumber,
		TWCCSequenceNumber: twccSequenceNumber,
		Size:               size,
		Departure:          departure,
		Arrived:            false,
		Arrival:            time.Time{},
		ECN:                rtcp.ECNNonECT,
	}
	h.counter++
}

// onFeedback maps an incoming ack for counter to the PacketReport stored when
// the packet was sent. If the packet cannot be found, the ack is ignored.
//
// onFeedback must be called while holding the lock for reading.
// onFeedback returns the time between ts and the time the packet was sent.
func (h *history) onFeedback(ts time.Time, counter uint64, ack acknowledgement) (time.Duration, bool) {
	p, ok := h.packets[counter]
	if !ok {
		// ignore ack for unknown packet
		return 0, false
	}
	p.Arrived = ack.arrived
	if p.Arrived && h.highestAcked < p.SequenceNumber {
		h.highestAcked = p.SequenceNumber
	}
	p.Arrival = ack.arrival
	p.ECN = ack.ecn

	return ts.Sub(p.Departure), true
}

// onTWCCFeedback maps an acknowledgement to the counter by TWCC sequence number
// and then calls onFeedback.
func (h *history) onTWCCFeedback(ts time.Time, ack acknowledgement) (time.Duration, bool) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	counter, ok := h.twccToCounter[ack.sequenceNumber]
	if !ok {
		// ignore ack for unknown packet
		return 0, false
	}

	return h.onFeedback(ts, counter, ack)
}

// onCCFBFeedback maps an acknowledgement to the counter by ssrc and sequence
// number and then calls onFeedback.
func (h *history) onCCFBFeedback(ts time.Time, ssrc uint32, ack acknowledgement) (time.Duration, bool) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	counter, ok := h.ssrcSeqNrToCounter[ssrcSequenceNumber{
		ssrc:           ssrc,
		sequenceNumber: ack.sequenceNumber,
	}]
	if !ok {
		// ignore ack for unknown packet
		return 0, false
	}

	return h.onFeedback(ts, counter, ack)
}

// buildReport builds a report containing all packets up to the highest
// acknowledged packet that were not included in a previous report.
// TODO: Implement adaptive re-order window. Packets may arrive out of order. In
// that case, they will be reported as lost. Instead of reporting them lost, we
// could wait for a short time. In some cases, reordered packets will then be
// reported as arrived in the next report.
//
//nolint:godox
func (h *history) buildReport() []PacketReport {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.nextReport > h.highestAcked {
		return nil
	}
	res := make([]PacketReport, 0, h.highestAcked-h.nextReport+1)
	for i := h.nextReport; i <= h.highestAcked; i++ {
		packet, ok := h.packets[i]
		if !ok {
			// packet dropped from history?
			continue
		}
		res = append(res, *packet)
		h.delete(packet)
		if packet.SequenceNumber >= h.nextReport {
			h.nextReport = packet.SequenceNumber + 1
		}
	}
	h.cleanBefore(h.nextReport)

	return res
}

// delete removes p from the history. It must be called while holding the lock
// for writing.
func (h *history) delete(p *PacketReport) {
	if p.IsTWCC {
		delete(h.twccToCounter, p.TWCCSequenceNumber)
	}
	delete(h.ssrcSeqNrToCounter, ssrcSequenceNumber{
		ssrc:           p.SSRC,
		sequenceNumber: p.RTPSequenceNumber,
	})
}

// cleanBefore removes all entries in the interval [h.cleanBefore, counter).
// cleanBefore must be called while holding the lock for writing, because it
// calls out to delete.
func (h *history) cleanBefore(counter uint64) {
	for i := h.cleanUntil; i < counter; i++ {
		if p, ok := h.packets[i]; ok {
			h.delete(p)
		}
	}
	h.cleanUntil = counter - 1
}
