// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"

	"github.com/pion/rtp"
)

// PriorityQueue provides a linked list sorting of RTP packets by SequenceNumber.
type PriorityQueue struct {
	next   *node
	length uint16
}

type node struct {
	val      *rtp.Packet
	next     *node
	prev     *node
	priority uint16
}

var (
	// ErrInvalidOperation may be returned if a Pop or Find operation is performed on an empty queue.
	ErrInvalidOperation = errors.New("attempt to find or pop on an empty list")
	// ErrNotFound will be returned if the packet cannot be found in the queue.
	ErrNotFound = errors.New("priority not found")
)

// NewQueue will create a new PriorityQueue whose order relies on monotonically
// increasing Sequence Number, wrapping at MaxUint16, so
// a packet with sequence number MaxUint16 - 1 will be after 0.
func NewQueue() *PriorityQueue {
	return &PriorityQueue{
		next:   nil,
		length: 0,
	}
}

func newNode(val *rtp.Packet, priority uint16) *node {
	return &node{
		val:      val,
		prev:     nil,
		next:     nil,
		priority: priority,
	}
}

// Find a packet in the queue with the provided sequence number,
// regardless of position (the packet is retained in the queue).
func (q *PriorityQueue) Find(sqNum uint16) (*rtp.Packet, error) {
	next := q.next
	for next != nil {
		if next.priority == sqNum {
			return next.val, nil
		}
		next = next.next
	}

	return nil, ErrNotFound
}

// Push will insert a packet in to the queue in order of sequence number.
func (q *PriorityQueue) Push(val *rtp.Packet, priority uint16) {
	newPq := newNode(val, priority)

	// Insert at head: empty list or new node has smallest-or-equal priority.
	// Using <= ensures equal-priority elements are consistently placed before
	// all existing equals, matching the traversal loop's break-on-equal behavior.
	cur := q.next
	if cur == nil || priority <= cur.priority {
		newPq.next = cur
		if cur != nil {
			cur.prev = newPq
		}
		q.next = newPq
		q.length++
		return
	}

	// Traverse to find insertion point; cur is guaranteed non-nil here.
	prev := cur
	for cur = cur.next; cur != nil && priority > cur.priority; cur = cur.next {
		prev = cur
	}

	prev.next = newPq
	newPq.prev = prev
	newPq.next = cur
	if cur != nil {
		cur.prev = newPq
	}
	q.length++
}

// Length will get the total length of the queue.
func (q *PriorityQueue) Length() uint16 {
	return q.length
}

// Pop removes the first element from the queue, regardless
// sequence number.
func (q *PriorityQueue) Pop() (*rtp.Packet, error) {
	head := q.next
	if head == nil {
		return nil, ErrInvalidOperation
	}
	q.next = head.next
	if head.next != nil {
		head.next.prev = nil
	}
	head.next = nil
	val := head.val
	head.val = nil
	q.length--

	return val, nil
}

// PopAt removes an element at the specified sequence number (priority).
func (q *PriorityQueue) PopAt(sqNum uint16) (*rtp.Packet, error) {
	head := q.next
	if head == nil {
		return nil, ErrInvalidOperation
	}
	if head.priority == sqNum {
		q.next = head.next
		if head.next != nil {
			head.next.prev = nil
		}
		head.next = nil
		val := head.val
		head.val = nil
		q.length--

		return val, nil
	}
	// prev is guaranteed non-nil since head didn't match.
	prev := head
	for pos := head.next; pos != nil; pos = pos.next {
		if pos.priority == sqNum {
			prev.next = pos.next
			if pos.next != nil {
				pos.next.prev = prev
			}
			pos.next = nil
			pos.prev = nil
			val := pos.val
			pos.val = nil
			q.length--

			return val, nil
		}
		prev = pos
	}

	return nil, ErrNotFound
}

// PopAtTimestamp removes and returns a packet at the given RTP Timestamp, regardless
// sequence number order.
func (q *PriorityQueue) PopAtTimestamp(timestamp uint32) (*rtp.Packet, error) {
	head := q.next
	if head == nil {
		return nil, ErrInvalidOperation
	}
	if head.val.Timestamp == timestamp {
		q.next = head.next
		if head.next != nil {
			head.next.prev = nil
		}
		head.next = nil
		val := head.val
		head.val = nil
		q.length--

		return val, nil
	}
	// prev is guaranteed non-nil since head didn't match.
	prev := head
	for pos := head.next; pos != nil; pos = pos.next {
		if pos.val.Timestamp == timestamp {
			prev.next = pos.next
			if pos.next != nil {
				pos.next.prev = prev
			}
			pos.next = nil
			pos.prev = nil
			val := pos.val
			pos.val = nil
			q.length--

			return val, nil
		}
		prev = pos
	}

	return nil, ErrNotFound
}

// Clear will empty a PriorityQueue.
func (q *PriorityQueue) Clear() {
	q.next = nil
	q.length = 0
}
