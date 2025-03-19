package ccfb

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor/internal/sequencenumber"
	"github.com/pion/rtcp"
)

var errSequenceNumberWentBackwards = errors.New("sequence number went backwards")

// PacketReport contains departure and arrival information about an acknowledged
// packet.
type PacketReport struct {
	SeqNr     int64
	Size      int
	Departure time.Time
	Arrived   bool
	Arrival   time.Time
	ECN       rtcp.ECN
}

type sentPacket struct {
	seqNr     int64
	size      int
	departure time.Time
}

type historyList struct {
	lock          sync.Mutex
	size          int
	evictList     *list.List
	seqNrToPacket map[int64]*list.Element
	sentSeqNr     *sequencenumber.Unwrapper
	ackedSeqNr    *sequencenumber.Unwrapper
}

func newHistoryList(size int) *historyList {
	return &historyList{
		lock:          sync.Mutex{},
		size:          size,
		evictList:     list.New(),
		seqNrToPacket: make(map[int64]*list.Element),
		sentSeqNr:     &sequencenumber.Unwrapper{},
		ackedSeqNr:    &sequencenumber.Unwrapper{},
	}
}

func (h *historyList) add(seqNr uint16, size int, departure time.Time) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	sn := h.sentSeqNr.Unwrap(seqNr)
	last := h.evictList.Back()
	if last != nil {
		if p, ok := last.Value.(sentPacket); ok && sn < p.seqNr {
			return errSequenceNumberWentBackwards
		}
	}
	ent := h.evictList.PushBack(sentPacket{
		seqNr:     sn,
		size:      size,
		departure: departure,
	})
	h.seqNrToPacket[sn] = ent

	if h.evictList.Len() > h.size {
		h.removeOldest()
	}

	return nil
}

// Must be called while holding the lock.
func (h *historyList) removeOldest() {
	if ent := h.evictList.Front(); ent != nil {
		v := h.evictList.Remove(ent)
		if sp, ok := v.(sentPacket); ok {
			delete(h.seqNrToPacket, sp.seqNr)
		}
	}
}

func (h *historyList) getReportForAck(acks []acknowledgement) []PacketReport {
	h.lock.Lock()
	defer h.lock.Unlock()

	reports := []PacketReport{}
	for _, pr := range acks {
		sn := h.ackedSeqNr.Unwrap(pr.seqNr)
		ent, ok := h.seqNrToPacket[sn]
		// Ignore report for unknown packets (migth have been dropped from
		// history)
		if ok {
			if ack, ok := ent.Value.(sentPacket); ok {
				reports = append(reports, PacketReport{
					SeqNr:     sn,
					Size:      ack.size,
					Departure: ack.departure,
					Arrived:   pr.arrived,
					Arrival:   pr.arrival,
					ECN:       pr.ecn,
				})
			}
		}
	}

	return reports
}
