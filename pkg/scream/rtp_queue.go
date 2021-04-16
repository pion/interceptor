//+build scream

package scream

import (
	"container/list"
	"sync"

	"github.com/pion/rtp"
)

type rtpQueueItem struct {
	packet *rtp.Packet
	ts     uint64
}

type queue struct {
	m sync.RWMutex

	bytesInQueue int
	queue        *list.List
}

func newQueue() RTPQueue {
	return &queue{queue: list.New()}
}

func (q *queue) SizeOfNextRTP() int {
	q.m.RLock()
	defer q.m.RUnlock()

	if q.queue.Len() <= 0 {
		return 0
	}

	return q.queue.Front().Value.(rtpQueueItem).packet.MarshalSize()
}

func (q *queue) SeqNrOfNextRTP() uint16 {
	q.m.RLock()
	defer q.m.RUnlock()

	if q.queue.Len() <= 0 {
		return 0
	}

	return q.queue.Front().Value.(rtpQueueItem).packet.SequenceNumber
}

func (q *queue) BytesInQueue() int {
	q.m.Lock()
	defer q.m.Unlock()

	return q.bytesInQueue
}

func (q *queue) SizeOfQueue() int {
	q.m.RLock()
	defer q.m.RUnlock()

	return q.queue.Len()
}

func (q *queue) GetDelay(ts float32) float32 {
	q.m.Lock()
	defer q.m.Unlock()

	if q.queue.Len() <= 0 {
		return 0
	}
	pkt := q.queue.Front().Value.(rtpQueueItem)
	ntpTime := pkt.ts
	// First convert 64 bit NTP timestamp to 32 bit NTP timestamp as used by scream
	q16Time := uint32((ntpTime >> 16) & 0xFFFFFFFF)
	// Then scale the timestamp to seconds as done by scream and calculate the difference to given ts
	delay := ts - float32(q16Time)/65536.0

	return delay
}

func (q *queue) GetSizeOfLastFrame() int {
	q.m.RLock()
	defer q.m.RUnlock()

	if q.queue.Len() <= 0 {
		return 0
	}

	return q.queue.Back().Value.(rtpQueueItem).packet.MarshalSize()
}

func (q *queue) Clear() {
	q.m.Lock()
	defer q.m.Unlock()

	q.bytesInQueue = 0
	q.queue.Init()
}

func (q *queue) Enqueue(packet *rtp.Packet, ts uint64) {
	q.m.Lock()
	defer q.m.Unlock()

	q.bytesInQueue += packet.MarshalSize()
	q.queue.PushBack(rtpQueueItem{
		packet: packet,
		ts:     ts,
	})
}

func (q *queue) Dequeue() *rtp.Packet {
	q.m.Lock()
	defer q.m.Unlock()

	if q.queue.Len() <= 0 {
		return nil
	}

	front := q.queue.Front()
	q.queue.Remove(front)
	packet := front.Value.(rtpQueueItem).packet
	q.bytesInQueue -= packet.MarshalSize()
	return packet
}
