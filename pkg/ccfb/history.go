package ccfb

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/pion/interceptor/internal/sequencenumber"
	"github.com/pion/rtcp"
)

type PacketReportList struct {
	Arrival   time.Time
	Departure time.Time
	Reports   []PacketReport
}

type PacketReport struct {
	SeqNr     int64
	Size      uint16
	Departure time.Time
	Arrived   bool
	Arrival   time.Time
	ECN       rtcp.ECN
}

type sentPacket struct {
	seqNr     int64
	size      uint16
	departure time.Time
}

type history struct {
	lock          sync.Mutex
	size          int
	evictList     *list.List
	seqNrToPacket map[int64]*list.Element
	sentSeqNr     *sequencenumber.Unwrapper
	ackedSeqNr    *sequencenumber.Unwrapper
}

func newHistory(size int) *history {
	return &history{
		lock:          sync.Mutex{},
		size:          size,
		evictList:     list.New(),
		seqNrToPacket: make(map[int64]*list.Element),
		sentSeqNr:     &sequencenumber.Unwrapper{},
		ackedSeqNr:    &sequencenumber.Unwrapper{},
	}
}

func (h *history) add(seqNr uint16, size uint16, departure time.Time) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	sn := h.sentSeqNr.Unwrap(seqNr)
	last := h.evictList.Back()
	if last != nil {
		if p, ok := last.Value.(sentPacket); ok && sn < p.seqNr {
			return errors.New("sequence number went backwards")
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

// Must be called while holding the lock
func (h *history) removeOldest() {
	if ent := h.evictList.Front(); ent != nil {
		v := h.evictList.Remove(ent)
		if sp, ok := v.(sentPacket); ok {
			delete(h.seqNrToPacket, sp.seqNr)
		}
	}
}

func (h *history) getReportForAck(al acknowledgementList) PacketReportList {
	h.lock.Lock()
	defer h.lock.Unlock()

	var reports []PacketReport
	for _, pr := range al.acks {
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

	return PacketReportList{
		Arrival: al.ts,
		Reports: reports,
	}
}
