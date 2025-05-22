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

// Insert adds a new packet to the tree with the given priority.
// Implements canonical red-black tree insert logic.
func (t *RBTree) Insert(pkt *rtp.Packet) {
	node := &rbnode{
		val:      pkt,
		priority: pkt.SequenceNumber,
		color:    red,
	}
	t.length++
	var y *rbnode
	x := t.root
	for x != nil {
		y = x
		if node.priority < x.priority {
			x = x.left
		} else {
			x = x.right
		}
	}
	node.parent = y
	if y == nil {
		t.root = node
	} else if node.priority < y.priority {
		y.left = node
	} else {
		y.right = node
	}
	t.fixInsert(node)
}

// fixInsert restores red-black properties after insertion.
func (t *RBTree) fixInsert(node *rbnode) {
	for node != t.root && node.parent.color == red {
		if node.parent == node.parent.parent.left {
			y := node.parent.parent.right
			if y != nil && y.color == red {
				node.parent.color = black
				y.color = black
				node.parent.parent.color = red
				node = node.parent.parent
			} else {
				if node == node.parent.right {
					node = node.parent
					t.rotateLeft(node)
				}
				node.parent.color = black
				node.parent.parent.color = red
				t.rotateRight(node.parent.parent)
			}
		} else {
			y := node.parent.parent.left
			if y != nil && y.color == red {
				node.parent.color = black
				y.color = black
				node.parent.parent.color = red
				node = node.parent.parent
			} else {
				if node == node.parent.left {
					node = node.parent
					t.rotateRight(node)
				}
				node.parent.color = black
				node.parent.parent.color = red
				t.rotateLeft(node.parent.parent)
			}
		}
	}
	t.root.color = black
}

// Find returns the packet with the given priority, or an error if not found.
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
// Implements canonical red-black tree delete logic.
func (t *RBTree) Delete(priority uint16) error {
	z := t.root
	for z != nil && z.priority != priority {
		if priority < z.priority {
			z = z.left
		} else {
			z = z.right
		}
	}
	if z == nil {
		return ErrNotFound
	}
	t.length--
	y := z
	yOriginalColor := y.color
	var x *rbnode
	var xParent *rbnode
	if z.left == nil {
		x = z.right
		t.transplant(z, z.right)
		xParent = z.parent
	} else if z.right == nil {
		x = z.left
		t.transplant(z, z.left)
		xParent = z.parent
	} else {
		y = t.minimum(z.right)
		yOriginalColor = y.color
		x = y.right
		if y.parent == z {
			if x != nil {
				x.parent = y
			}
			xParent = y
		} else {
			t.transplant(y, y.right)
			y.right = z.right
			if y.right != nil {
				y.right.parent = y
			}
			xParent = y.parent
		}
		t.transplant(z, y)
		y.left = z.left
		if y.left != nil {
			y.left.parent = y
		}
		y.color = z.color
	}
	if yOriginalColor == black {
		t.fixDelete(x, xParent)
	}
	return nil
}

// transplant replaces u with v in the tree.
func (t *RBTree) transplant(u, v *rbnode) {
	if u.parent == nil {
		t.root = v
	} else if u == u.parent.left {
		u.parent.left = v
	} else {
		u.parent.right = v
	}
	if v != nil {
		v.parent = u.parent
	}
}

// minimum returns the node with minimum priority in the subtree rooted at node.
func (t *RBTree) minimum(node *rbnode) *rbnode {
	for node.left != nil {
		node = node.left
	}
	return node
}

// fixDelete restores red-black properties after deletion.
func (t *RBTree) fixDelete(x *rbnode, parent *rbnode) {
	for (x != t.root) && (x == nil || x.color == black) {
		var w *rbnode
		if x == parent.left {
			w = parent.right
			if w != nil && w.color == red {
				w.color = black
				parent.color = red
				t.rotateLeft(parent)
				w = parent.right
			}
			if (w == nil) || ((w.left == nil || w.left.color == black) && (w.right == nil || w.right.color == black)) {
				if w != nil {
					w.color = red
				}
				x = parent
				parent = x.parent
			} else {
				if w.right == nil || w.right.color == black {
					if w.left != nil {
						w.left.color = black
					}
					w.color = red
					t.rotateRight(w)
					w = parent.right
				}
				if w != nil {
					w.color = parent.color
					if w.right != nil {
						w.right.color = black
					}
				}
				parent.color = black
				t.rotateLeft(parent)
				x = t.root
			}
		} else {
			w = parent.left
			if w != nil && w.color == red {
				w.color = black
				parent.color = red
				t.rotateRight(parent)
				w = parent.left
			}
			if (w == nil) || ((w.right == nil || w.right.color == black) && (w.left == nil || w.left.color == black)) {
				if w != nil {
					w.color = red
				}
				x = parent
				parent = x.parent
			} else {
				if w.left == nil || w.left.color == black {
					if w.right != nil {
						w.right.color = black
					}
					w.color = red
					t.rotateLeft(w)
					w = parent.left
				}
				if w != nil {
					w.color = parent.color
					if w.left != nil {
						w.left.color = black
					}
				}
				parent.color = black
				t.rotateRight(parent)
				x = t.root
			}
		}
	}
	if x != nil {
		x.color = black
	}
}
