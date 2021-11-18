package nack

import (
	"sync"

	"github.com/pion/rtp"
)

const (
	uint16SizeHalf = 1 << 15
)

type sendBuffer struct {
	packets   []*rtp.Packet
	size      uint16
	lastAdded uint16
	started   bool

	m sync.RWMutex
}

func newSendBuffer(size uint16) *sendBuffer {
	return &sendBuffer{
		packets: make([]*rtp.Packet, size),
		size:    size,
	}
}

func (s *sendBuffer) add(packet *rtp.Packet) {
	s.m.Lock()
	defer s.m.Unlock()

	seq := packet.SequenceNumber
	if !s.started {
		s.packets[seq%s.size] = packet
		s.lastAdded = seq
		s.started = true
		return
	}

	diff := seq - s.lastAdded
	if diff == 0 {
		return
	} else if diff < uint16SizeHalf {
		for i := s.lastAdded + 1; i != seq; i++ {
			s.packets[i%s.size] = nil
		}
	}

	s.packets[seq%s.size] = packet
	s.lastAdded = seq
}

func (s *sendBuffer) get(seq uint16) *rtp.Packet {
	s.m.RLock()
	defer s.m.RUnlock()

	diff := s.lastAdded - seq
	if diff >= uint16SizeHalf {
		return nil
	}

	if diff >= s.size {
		return nil
	}

	return s.packets[seq%s.size]
}
