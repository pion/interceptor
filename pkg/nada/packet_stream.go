package nada

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type packet struct {
	ts            time.Time
	seq           uint16
	ecn           bool
	size          Bits
	queueingDelay bool
}

// String returns a string representation of the packet.
func (p *packet) String() string {
	return fmt.Sprintf("%v@%v", p.seq, p.ts.Nanosecond()%1000)
}

type packetStream struct {
	sync.Mutex

	window             time.Duration
	packets            []*packet
	markCount          uint16
	totalSize          Bits
	queueingDelayCount uint16
}

func newPacketStream(window time.Duration) *packetStream {
	return &packetStream{
		window: window,
	}
}

var errTimeOrder = errors.New("invalid packet timestamp ordering")

// add writes a packet to the underlying stream.
func (ps *packetStream) add(ts time.Time, seq uint16, ecn bool, size Bits, queueingDelay bool) error {
	ps.Lock()
	defer ps.Unlock()

	if len(ps.packets) > 0 && ps.packets[len(ps.packets)-1].ts.After(ts) {
		return errTimeOrder
	}
	// check if the packet seq already exists.
	for _, p := range ps.packets {
		if p.seq == seq {
			return errTimeOrder
		}
	}
	ps.packets = append(ps.packets, &packet{
		ts:            ts,
		seq:           seq,
		ecn:           ecn,
		size:          size,
		queueingDelay: queueingDelay,
	})
	if ecn {
		ps.markCount++
	}
	ps.totalSize += size
	if queueingDelay {
		ps.queueingDelayCount++
	}
	return nil
}

// prune removes packets that are older than the window and returns the loss and marking rate.
func (ps *packetStream) prune(now time.Time) (loss float64, marking float64, receivingRate BitsPerSecond, hasQueueingDelay bool) {
	ps.Lock()
	defer ps.Unlock()

	startTS := now.Add(-ps.window)
	start := 0
	for ; start < len(ps.packets) && ps.packets[start].ts.Before(startTS); start++ {
		// decrement mark count if ecn.
		if ps.packets[start].ecn {
			ps.markCount--
		}
		ps.totalSize -= ps.packets[start].size
		if ps.packets[start].queueingDelay {
			ps.queueingDelayCount--
		}
	}
	if start > 0 {
		ps.packets = ps.packets[start:]
	}
	seqs := make([]uint16, len(ps.packets))
	for i, p := range ps.packets {
		seqs[i] = p.seq
	}
	begin, end := getSeqRange(seqs)
	loss = 1 - float64(len(ps.packets))/float64(end-begin+1)
	marking = float64(ps.markCount) / float64(end-begin+1)
	return loss, marking, BitsPerSecond(float64(ps.totalSize) / ps.window.Seconds()), ps.queueingDelayCount > 0
}

func getSeqRange(seqs []uint16) (uint16, uint16) {
	minDelta := 0
	maxDelta := 0
	seq0 := seqs[0]
	for _, seq := range seqs {
		delta := int(seq - seq0)
		if seq-seq0 >= 16384 {
			delta -= (1 << 16)
			if delta < minDelta {
				minDelta = delta
			}
		} else if delta > maxDelta {
			maxDelta = delta
		}
	}
	return seq0 + uint16(minDelta), seq0 + uint16(maxDelta)
}
