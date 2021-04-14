package scream

import (
	"container/list"
	"sync"

	"github.com/pion/rtp"
)

type rtpQueueItem struct {
	packet *rtp.Packet
	ts     float64
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

func (q *queue) GetDelay(ts float64) float64 {
	q.m.Lock()
	defer q.m.Unlock()

	if q.queue.Len() <= 0 {
		return 0
	}
	pkt := q.queue.Front().Value.(rtpQueueItem)
	d := ts - pkt.ts
	//fmt.Printf("ts=%v, pkt.ts=%v delay=ts-pkt.ts=%v\n", ts, pkt.ts, d)
	return d
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

func (q *queue) Enqueue(packet *rtp.Packet, ts float64) {
	q.m.Lock()
	defer q.m.Unlock()

	q.bytesInQueue += packet.MarshalSize()
	q.queue.PushBack(rtpQueueItem{
		packet: packet,
		ts:     float64(ts),
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
