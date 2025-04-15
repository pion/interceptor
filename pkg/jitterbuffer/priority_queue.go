// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"
	"fmt"

	"github.com/pion/rtp"
)

type (
	treeColor bool
	direction int8
)

const (
	red, black treeColor = false, true
)

const (
	right direction = 1
)

type rbnode struct {
	parent, left, right *rbnode
	next                *rbnode
	priority            uint16
	val                 *rtp.Packet
	color               treeColor
}

// Red-Black tree
type RBTree struct {
	root   *rbnode
	head   *rbnode
	tail   *rbnode
	length uint16
}

func NewTree() *RBTree {
	return &RBTree{}
}

func (t *RBTree) RotateRight(x *rbnode) {
	y := x.left
	x.left = y.right
	if y.right != nil {
		y.right.parent = x
	}

	y.parent = x.parent
	if x.parent == nil {
		t.root = y
	} else if x == x.parent.right {
		x.parent.right = y
	} else {
		x.parent.left = y
	}

	y.right = x
	x.parent = y
}

func (t *RBTree) RotateLeft(x *rbnode) {
	y := x.right
	x.right = y.left
	if y.left != nil {
		y.left.parent = x
	}

	y.parent = x.parent
	if x.parent == nil {
		t.root = y
	} else if x == x.parent.left {
		x.parent.left = y
	} else {
		x.parent.right = y
	}

	y.left = x
	x.parent = y
}

func (t *RBTree) Insert(pkt *rtp.Packet) {
	n := &rbnode{
		val:      pkt,
		priority: pkt.SequenceNumber,
		color:    red,
	}
	t.length++
	if t.root == nil {
		t.root = n
		t.root.color = black
		return
	}

	current := t.root
	var parent *rbnode
	for current != nil {
		parent = current
		if n.priority < current.priority {
			current = current.left
		} else {
			current = current.right
		}
	}

	n.parent = parent
	if n.priority < parent.priority {
		parent.left = n
	} else {
		parent.right = n
	}

	t.fixInsert(n)
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
				t.RotateLeft(n.parent)
				n = n.left
			}
			n.parent.color = black
			grandparent.color = red
			t.RotateRight(grandparent)
		} else {
			if n == n.parent.left {
				t.RotateRight(n.parent)
				n = n.right
			}
			n.parent.color = black
			grandparent.color = red
			t.RotateLeft(grandparent)
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

func (t *RBTree) PrettyPrint() {
	if t.root == nil {
		fmt.Println("Empty tree")
		return
	}

	nodeInfo := func(n *rbnode) string {
		if n == nil {
			return "NIL(B)"
		}
		color := "R"
		if n.color == black {
			color = "B"
		}
		return fmt.Sprintf("%d(%s)", n.priority, color)
	}

	var printNode func(node *rbnode, prefix string, isLeft bool)
	printNode = func(node *rbnode, prefix string, isLeft bool) {
		if node == nil {
			return
		}

		nodePrefix := "└──"
		childPrefix := "    "
		if isLeft {
			nodePrefix = "├──"
			childPrefix = "│   "
		}

		fmt.Printf("%s%s%s\n", prefix, nodePrefix, nodeInfo(node))
		printNode(node.left, prefix+childPrefix, true)
		printNode(node.right, prefix+childPrefix, false)
	}

	fmt.Printf("%s\n", nodeInfo(t.root))
	printNode(t.root.left, "", true)
	printNode(t.root.right, "", false)
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
	n = nil
}

// Find a node by priority.
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
	// y is the node to be removed from the tree
	// If node has less than 2 children, y = node
	// If node has 2 children, y = successor
	var y *rbnode
	var x *rbnode // x is y's only child (or nil)

	if node.left == nil || node.right == nil {
		y = node
	} else {
		// Find successor (smallest value in right subtree)
		y = node.right
		for y.left != nil {
			y = y.left
		}
	}

	if y.left != nil {
		x = y.left
	} else {
		x = y.right
	}

	if x != nil {
		x.parent = y.parent
	}
	if y.parent == nil {
		t.root = x
	} else if y == y.parent.left {
		y.parent.left = x
	} else {
		y.parent.right = x
	}

	// If we removed the successor, copy its data to the original node
	if y != node {
		node.priority = y.priority
		node.val = y.val
	}

	// If we removed a black node, we need to fix the tree
	if y.color == black {
		t.fixDelete(x, y.parent)
	}

	return nil
}

func (t *RBTree) fixDelete(x *rbnode, parent *rbnode) {
	for x != t.root && (x == nil || x.color == black) {
		if x == nil && parent == nil {
			break
		}

		var w *rbnode
		isLeft := x == parent.left
		if isLeft {
			w = parent.right
		} else {
			w = parent.left
		}

		if w == nil {
			x = parent
			parent = parent.parent
			continue
		}

		if w.color == red {
			w.color = black
			parent.color = red
			if isLeft {
				t.RotateLeft(parent)
				w = parent.right
			} else {
				t.RotateRight(parent)
				w = parent.left
			}
		}

		wLeftBlack := w.left == nil || w.left.color == black
		wRightBlack := w.right == nil || w.right.color == black

		if wLeftBlack && wRightBlack {
			w.color = red
			x = parent
			parent = parent.parent
		} else {
			if isLeft {
				if wRightBlack {
					if w.left != nil {
						w.left.color = black
					}
					w.color = red
					t.RotateRight(w)
					w = parent.right
				}
				w.color = parent.color
				parent.color = black
				if w.right != nil {
					w.right.color = black
				}
				t.RotateLeft(parent)
			} else {
				if wLeftBlack {
					if w.right != nil {
						w.right.color = black
					}
					w.color = red
					t.RotateLeft(w)
					w = parent.left
				}
				w.color = parent.color
				parent.color = black
				if w.left != nil {
					w.left.color = black
				}
				t.RotateRight(parent)
			}
			x = t.root
		}
	}
	if x != nil {
		x.color = black
	}
}
