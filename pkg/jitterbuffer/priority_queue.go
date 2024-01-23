// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"

	"github.com/pion/rtp"
)

// PriorityQueue provides a linked list sorting of RTP packets by SequenceNumber
type PriorityQueue struct {
	next   *node
	length uint16
}

type node struct {
	val  *rtp.Packet
	next *node
	prev *node
	prio uint16
}

var (
	// ErrInvalidOperation may be returned if a Pop or Find operation is performed on an empty queue
	ErrInvalidOperation = errors.New("attempt to find or pop on an empty list")
	// ErrNotFound will be returned if the packet cannot be found in the queue
	ErrNotFound = errors.New("priority not found")
)

// NewQueue will create a new PriorityQueue whose order relies on monotonically
// increasing Sequence Number, wrapping at MaxUint16, so
// a packet with sequence number MaxUint16 - 1 will be after 0
func NewQueue() *PriorityQueue {
	return &PriorityQueue{
		next:   nil,
		length: 0,
	}
}

func newNode(val *rtp.Packet, prio uint16) *node {
	return &node{
		val:  val,
		prev: nil,
		next: nil,
		prio: prio,
	}
}

// Find a packet in the queue with the provided sequence number,
// regardless of position (the packet is retained in the queue)
func (q *PriorityQueue) Find(sqNum uint16) (*rtp.Packet, error) {
	if q.next.prio == sqNum {
		return q.next.val, nil
	}

	if sqNum < q.next.prio {
		return nil, ErrInvalidOperation
	}
	next := q.next
	for next != nil {
		if next.prio == sqNum {
			return next.val, nil
		}
		next = next.next
	}
	return nil, ErrNotFound
}

// Push will insert a packet in to the queue in order of sequence number
func (q *PriorityQueue) Push(val *rtp.Packet, prio uint16) {
	newPq := newNode(val, prio)
	if q.next == nil {
		q.next = newPq
		q.length++
		return
	}
	if prio < q.next.prio {
		newPq.next = q.next
		q.next.prev = newPq
		q.next = newPq
		q.length++
		return
	}
	head := q.next
	prev := q.next
	for head != nil {
		if prio <= head.prio {
			break
		}
		prev = head
		head = head.next
	}
	if head == nil {
		if prev != nil {
			prev.next = newPq
		}
		newPq.prev = prev
	} else {
		newPq.next = head
		newPq.prev = prev
		if prev != nil {
			prev.next = newPq
		}
		head.prev = newPq
	}
	q.length++
}

// Length will get the total length of the queue
func (q *PriorityQueue) Length() uint16 {
	return q.length
}

// Pop removes the first element from the queue, regardless
// sequence number
func (q *PriorityQueue) Pop() (*rtp.Packet, error) {
	if q.next == nil {
		return nil, ErrInvalidOperation
	}
	val := q.next.val
	q.length--
	q.next = q.next.next
	return val, nil
}

// PopAt removes an element at the specified sequence number (priority)
func (q *PriorityQueue) PopAt(sqNum uint16) (*rtp.Packet, error) {
	if q.next == nil {
		return nil, ErrInvalidOperation
	}
	if q.next.prio == sqNum {
		val := q.next.val
		q.next = q.next.next
		return val, nil
	}
	pos := q.next
	prev := q.next.prev
	for pos != nil {
		if pos.prio == sqNum {
			val := pos.val
			prev.next = pos.next
			if prev.next != nil {
				prev.next.prev = prev
			}
			return val, nil
		}
		prev = pos
		pos = pos.next
	}
	return nil, ErrNotFound
}

// PopAtTimestamp removes and returns a packet at the given RTP Timestamp, regardless
// sequence number order
func (q *PriorityQueue) PopAtTimestamp(timestamp uint32) (*rtp.Packet, error) {
	if q.next == nil {
		return nil, ErrInvalidOperation
	}
	if q.next.val.Timestamp == timestamp {
		val := q.next.val
		q.next = q.next.next
		return val, nil
	}
	pos := q.next
	prev := q.next.prev
	for pos != nil {
		if pos.val.Timestamp == timestamp {
			val := pos.val
			prev.next = pos.next
			if prev.next != nil {
				prev.next.prev = prev
			}
			return val, nil
		}
		prev = pos
		pos = pos.next
	}
	return nil, ErrNotFound
}
