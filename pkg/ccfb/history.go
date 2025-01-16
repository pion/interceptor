package ccfb

import (
	"errors"
	"log"
	"time"

	"github.com/pion/interceptor/internal/sequencenumber"
	"github.com/pion/rtcp"
)

type PacketReportList struct {
	Timestamp time.Time
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
	inflight   []sentPacket
	sentSeqNr  *sequencenumber.Unwrapper
	ackedSeqNr *sequencenumber.Unwrapper
}

func newHistory() *history {
	return &history{
		inflight:   []sentPacket{},
		sentSeqNr:  &sequencenumber.Unwrapper{},
		ackedSeqNr: &sequencenumber.Unwrapper{},
	}
}

func (h *history) add(seqNr uint16, size uint16, departure time.Time) error {
	sn := h.sentSeqNr.Unwrap(seqNr)
	if len(h.inflight) > 0 && sn < h.inflight[len(h.inflight)-1].seqNr {
		return errors.New("sequence number went backwards")
	}
	h.inflight = append(h.inflight, sentPacket{
		seqNr:     sn,
		size:      size,
		departure: departure,
	})
	return nil
}

func (h *history) getReportForAck(al acknowledgementList) PacketReportList {
	reports := []PacketReport{}
	log.Printf("highest sent: %v", h.inflight[len(h.inflight)-1].seqNr)
	for _, pr := range al.acks {
		sn := h.ackedSeqNr.Unwrap(pr.seqNr)
		i := h.index(sn)
		if i > -1 {
			reports = append(reports, PacketReport{
				SeqNr:     sn,
				Size:      h.inflight[i].size,
				Departure: h.inflight[i].departure,
				Arrived:   pr.arrived,
				Arrival:   pr.arrival,
				ECN:       pr.ecn,
			})
		} else {
			panic("got feedback for unknown packet")
		}
		log.Printf("processed ack for seq nr %v, arrived: %v", sn, pr.arrived)
	}
	return PacketReportList{
		Timestamp: al.ts,
		Reports:   reports,
	}
}

func (h *history) index(seqNr int64) int {
	for i := range h.inflight {
		if h.inflight[i].seqNr == seqNr {
			return i
		}
	}
	return -1
}
