package gcc

import (
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

type packetWithAttributes struct {
	packet     *rtp.Packet
	attributes interceptor.Attributes
}

func (p *packetWithAttributes) size() int {
	return p.packet.MarshalSize()
}

type packetWithAttributeQueue struct {
	data  []*packetWithAttributes
	mutex sync.RWMutex
}

func (q *packetWithAttributeQueue) Push(p *packetWithAttributes) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.data = append(q.data, p)
}

func (q *packetWithAttributeQueue) Pop() *packetWithAttributes {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if len(q.data) == 0 {
		return nil
	}
	p := q.data[0]
	q.data = q.data[1:]

	return p
}

func (q *packetWithAttributeQueue) Peek() *packetWithAttributes {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if len(q.data) == 0 {
		return nil
	}

	return q.data[0]
}

func (q *packetWithAttributeQueue) Size() int {
	return len(q.data)
}
