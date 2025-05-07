// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"

	"github.com/pion/rtp"
)

type (
	treeColor bool
)

const (
	red, black treeColor = false, true
)

type rbnode struct {
	parent, left, right *rbnode
	priority            uint16
	val                 *rtp.Packet
	color               treeColor
}

// RBTree structure is a red-black tree for fast access based on priority.
type RBTree struct {
	root   *rbnode
	length uint16
}

// NewTree creates a new red-black tree.
func NewTree() *RBTree {
	return &RBTree{}
}

// rotateRight will move the subtree starting at headNode under the left node of headNode.
func (t *RBTree) rotateRight(headNode *rbnode) {
	hLeft := headNode.left
	headNode.left = hLeft.right
	if hLeft.right != nil {
		hLeft.right.parent = headNode
	}

	hLeft.parent = headNode.parent
	switch {
	case headNode.parent == nil:
		t.root = hLeft
	case headNode == headNode.parent.right:
		headNode.parent.right = hLeft
	default:
		headNode.parent.left = hLeft
	}

	hLeft.right = headNode
	headNode.parent = hLeft
}

// rotateLeft will move the subtree starting at headNode under the right node of headNode.
func (t *RBTree) rotateLeft(headNode *rbnode) {
	rNode := headNode.right
	headNode.right = rNode.left
	if rNode.left != nil {
		rNode.left.parent = headNode
	}

	rNode.parent = headNode.parent
	switch {
	case headNode.parent == nil:
		t.root = rNode
	case headNode == headNode.parent.left:
		headNode.parent.left = rNode
	default:
		headNode.parent.right = rNode
	}

	rNode.left = headNode
	headNode.parent = rNode
}

func (t *RBTree) Insert(pkt *rtp.Packet) {
	node := &rbnode{
		val:      pkt,
		priority: pkt.SequenceNumber,
		color:    red,
	}
	t.length++
	if t.root == nil {
		t.root = node
		t.root.color = black

		return
	}

	current := t.root
	var parent *rbnode
	for current != nil {
		parent = current
		if node.priority < current.priority {
			current = current.left
		} else {
			current = current.right
		}
	}

	node.parent = parent
	if node.priority < parent.priority {
		parent.left = node
	} else {
		parent.right = node
	}

	t.fixInsert(node)
}

func (t *RBTree) fixInsert(n *rbnode) {
	n.color = red

	for n != t.root && n.parent != nil && n.parent.color == red {
		if n.parent.parent == nil {
			break
		}

		grandparent := n.parent.parent
		isParentLeft := n.parent == grandparent.left
		uncle := grandparent.right
		if !isParentLeft {
			uncle = grandparent.left
		}

		if uncle != nil && uncle.color == red {
			uncle.color = black
			n.parent.color = black
			if grandparent != t.root {
				grandparent.color = red
			}
			n = grandparent

			continue
		}

		if isParentLeft {
			if n == n.parent.right {
				t.rotateLeft(n.parent)
				n = n.left
			}
			n.parent.color = black
			grandparent.color = red
			t.rotateRight(grandparent)
		} else {
			if n == n.parent.left {
				t.rotateRight(n.parent)
				n = n.right
			}
			n.parent.color = black
			grandparent.color = red
			t.rotateLeft(grandparent)
		}
	}

	t.root.color = black
}

func (t *RBTree) Find(priority uint16) (*rtp.Packet, error) {
	current := t.root
	for current != nil {
		if priority == current.priority {
			return current.val, nil
		}
		if priority < current.priority {
			current = current.left
		} else {
			current = current.right
		}
	}

	return nil, ErrNotFound
}

var (
	// ErrInvalidOperation may be returned if a Pop or Find operation is performed on an empty queue.
	ErrInvalidOperation = errors.New("attempt to find or pop on an empty list")
	// ErrNotFound will be returned if the packet cannot be found in the queue.
	ErrNotFound = errors.New("priority not found")
)

// NewQueue will create a new PriorityQueue.
func NewQueue() *RBTree {
	return &RBTree{}
}

// Push will insert a packet in to the queue in order of sequence number.
func (t *RBTree) Push(val *rtp.Packet) {
	t.Insert(val)
}

// Length will get the total length of the queue.
func (t *RBTree) Length() uint16 {
	return t.length
}

// Pop will remove the root in the queue.
func (t *RBTree) Pop() (*rtp.Packet, error) {
	if t.root == nil {
		return nil, ErrInvalidOperation
	}
	pkt := t.root.val
	err := t.Delete(t.root.priority)
	if err != nil {
		return nil, err
	}

	return pkt, nil
}

// PopAt removes an element at the specified sequence number (priority).
func (t *RBTree) PopAt(sqNum uint16) (*rtp.Packet, error) {
	pkt, err := t.Find(sqNum)
	if err != nil {
		return nil, err
	}

	err = t.Delete(sqNum)
	if err != nil {
		return nil, err
	}

	return pkt, nil
}

// PopAtTimestamp removes and returns a packet at the given RTP Timestamp, regardless
// sequence number order.
func (t *RBTree) PopAtTimestamp(timestamp uint32) (*rtp.Packet, error) {
	if t.root == nil {
		return nil, ErrNotFound
	}

	queue := []*rbnode{t.root}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if node.val.Timestamp == timestamp {
			pkt := node.val
			err := t.Delete(node.priority)
			if err != nil {
				return nil, err
			}

			return pkt, nil
		}

		if node.left != nil {
			queue = append(queue, node.left)
		}
		if node.right != nil {
			queue = append(queue, node.right)
		}
	}

	return nil, ErrNotFound
}

// Clear will empty a PriorityQueue.
func (t *RBTree) Clear() {
	t.clear(t.root)
	t.root = nil
	t.length = 0
}

func (t *RBTree) clear(n *rbnode) {
	if n == nil {
		return
	}
	t.clear(n.left)
	t.clear(n.right)
}

// Peek will find a node by priority.
func (t *RBTree) Peek(priority uint16) (*rtp.Packet, error) {
	return t.Find(priority)
}

// Delete removes a node with the given priority from the tree.
func (t *RBTree) Delete(priority uint16) error {
	node := t.root
	for node != nil && node.priority != priority {
		if priority < node.priority {
			node = node.left
		} else {
			node = node.right
		}
	}
	if node == nil {
		return ErrNotFound
	}
	t.length--
	// rSuccessor is the node to be removed from the tree
	// If node has less than 2 children, y = node
	// If node has 2 children, y = successor
	var rSuccessor *rbnode
	var child *rbnode // child is y's only child (or nil)

	if node.left == nil || node.right == nil {
		rSuccessor = node
	} else {
		// Find successor (smallest value in right subtree)
		rSuccessor = node.right
		for rSuccessor.left != nil {
			rSuccessor = rSuccessor.left
		}
	}

	if rSuccessor.left != nil {
		child = rSuccessor.left
	} else {
		child = rSuccessor.right
	}

	if child != nil {
		child.parent = rSuccessor.parent
	}
	switch {
	case rSuccessor.parent == nil:
		t.root = child
	case rSuccessor == rSuccessor.parent.left:
		rSuccessor.parent.left = child
	default:
		rSuccessor.parent.right = child
	}

	// If we removed the successor, copy its data to the original node
	if rSuccessor != node {
		node.priority = rSuccessor.priority
		node.val = rSuccessor.val
	}

	// If we removed a black node, we need to fix the tree
	if rSuccessor.color == black {
		t.fixDelete(child, rSuccessor.parent)
	}

	return nil
}

func (t *RBTree) fixDelete(x *rbnode, parent *rbnode) {
	for x != t.root && (x == nil || x.color == black) {
		if x == nil && parent == nil {
			break
		}

		var wnode *rbnode
		isLeft := x == parent.left

		if isLeft {
			wnode = parent.right
		} else {
			wnode = parent.left
		}

		if wnode == nil {
			x = parent
			parent = parent.parent

			continue
		}

		if wnode.color == red {
			wnode.color = black
			parent.color = red
			if isLeft {
				t.rotateLeft(parent)
				wnode = parent.right
			} else {
				t.rotateRight(parent)
				wnode = parent.left
			}
		}

		wnodeLeftBlack := wnode.left == nil || wnode.left.color == black
		wnodeRightBlack := wnode.right == nil || wnode.right.color == black

		if wnodeLeftBlack && wnodeRightBlack {
			wnode.color = red
			x = parent
			parent = parent.parent
		} else {
			if isLeft {
				if wnodeRightBlack {
					if wnode.left != nil {
						wnode.left.color = black
					}
					wnode.color = red
					t.rotateRight(wnode)
					wnode = parent.right
				}
				wnode.color = parent.color
				parent.color = black
				if wnode.right != nil {
					wnode.right.color = black
				}
				t.rotateLeft(parent)
			} else {
				if wnodeLeftBlack {
					if wnode.right != nil {
						wnode.right.color = black
					}
					wnode.color = red
					t.rotateLeft(wnode)
					wnode = parent.left
				}
				wnode.color = parent.color
				parent.color = black
				if wnode.left != nil {
					wnode.left.color = black
				}
				t.rotateRight(parent)
			}
			x = t.root
		}
	}
	if x != nil {
		x.color = black
	}
}
