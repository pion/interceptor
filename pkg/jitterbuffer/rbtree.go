// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
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

// compareSequenceNumbers compares two sequence numbers, handling wrapping.
// Returns:
//
//	-1 if a < b
//	0 if a == b
//	1 if a > b
func (t *RBTree) compareSequenceNumbers(a, b uint16) int {
	// Handle wrapping by checking if the difference is more than half the range
	diff := int(a) - int(b)
	if diff > 32768 {
		return -1
	}
	if diff < -32768 {
		return 1
	}
	if diff < 0 {
		return -1
	}
	if diff > 0 {
		return 1
	}

	return 0
}

// Insert adds a new packet to the tree with the given priority.
func (t *RBTree) Insert(pkt *rtp.Packet) {
	node := &rbnode{
		val:      pkt,
		priority: pkt.SequenceNumber,
		color:    red,
	}
	t.length++

	// Find insertion point
	var parent *rbnode
	current := t.root
	for current != nil {
		parent = current
		if t.compareSequenceNumbers(node.priority, current.priority) < 0 {
			current = current.left
		} else {
			current = current.right
		}
	}

	// Insert node
	node.parent = parent
	switch {
	case parent == nil:
		t.root = node
	case t.compareSequenceNumbers(node.priority, parent.priority) < 0:
		parent.left = node
	default:
		parent.right = node
	}

	t.fixInsert(node)
}

// fixInsert restores red-black properties after insertion.
func (t *RBTree) fixInsert(node *rbnode) {
	for node != t.root && node.parent.color == red {
		if !t.fixInsertCase(&node) {
			break
		}
	}
	t.root.color = black
}

// fixInsertCase handles a single case of insertion fix-up.
// Returns false if no more fix-up is needed.
func (t *RBTree) fixInsertCase(node **rbnode) bool {
	isLeftChild := (*node).parent == (*node).parent.parent.left
	var uncle *rbnode
	if isLeftChild {
		uncle = (*node).parent.parent.right
	} else {
		uncle = (*node).parent.parent.left
	}

	if uncle != nil && uncle.color == red {
		// Case 1: Uncle is red
		(*node).parent.color = black
		uncle.color = black
		(*node).parent.parent.color = red
		*node = (*node).parent.parent

		return true
	}

	// Case 2: Uncle is black
	if isLeftChild {
		if *node == (*node).parent.right {
			*node = (*node).parent
			t.rotateLeft(*node)
		}
		(*node).parent.color = black
		(*node).parent.parent.color = red
		t.rotateRight((*node).parent.parent)
	} else {
		if *node == (*node).parent.left {
			*node = (*node).parent
			t.rotateRight(*node)
		}
		(*node).parent.color = black
		(*node).parent.parent.color = red
		t.rotateLeft((*node).parent.parent)
	}

	return false
}

// rotateLeft performs a left rotation around the given node.
func (t *RBTree) rotateLeft(node *rbnode) {
	if node == nil || node.right == nil {
		return
	}

	right := node.right
	node.right = right.left
	if right.left != nil {
		right.left.parent = node
	}

	right.parent = node.parent
	switch {
	case node.parent == nil:
		t.root = right
	case node == node.parent.left:
		node.parent.left = right
	default:
		node.parent.right = right
	}

	right.left = node
	node.parent = right
}

// rotateRight performs a right rotation around the given node.
func (t *RBTree) rotateRight(node *rbnode) {
	if node == nil || node.left == nil {
		return
	}

	left := node.left
	node.left = left.right
	if left.right != nil {
		left.right.parent = node
	}

	left.parent = node.parent
	switch {
	case node.parent == nil:
		t.root = left
	case node == node.parent.right:
		node.parent.right = left
	default:
		node.parent.left = left
	}

	left.right = node
	node.parent = left
}

// Find returns the packet with the given priority, or an error if not found.
func (t *RBTree) Find(priority uint16) (*rtp.Packet, error) {
	node := t.root
	for node != nil {
		cmp := t.compareSequenceNumbers(priority, node.priority)
		if cmp == 0 {
			return node.val, nil
		}
		if cmp < 0 {
			node = node.left
		} else {
			node = node.right
		}
	}

	return nil, ErrNotFound
}

// Delete removes a node with the given priority from the tree.
func (t *RBTree) Delete(priority uint16) error {
	node := t.root
	for node != nil {
		cmp := t.compareSequenceNumbers(priority, node.priority)
		if cmp == 0 {
			t.deleteNode(node)
			t.length--

			return nil
		}
		if cmp < 0 {
			node = node.left
		} else {
			node = node.right
		}
	}

	return ErrNotFound
}

// deleteNode removes the given node from the tree.
func (t *RBTree) deleteNode(node *rbnode) {
	var child *rbnode
	originalColor := node.color

	switch {
	case node.left == nil:
		child = node.right
		t.transplant(node, node.right)
	case node.right == nil:
		child = node.left
		t.transplant(node, node.left)
	default:
		successor := t.minimum(node.right)
		originalColor = successor.color
		child = successor.right

		if successor.parent == node {
			if child != nil {
				child.parent = successor
			}
		} else {
			t.transplant(successor, successor.right)
			successor.right = node.right
			successor.right.parent = successor
		}

		t.transplant(node, successor)
		successor.left = node.left
		successor.left.parent = successor
		successor.color = node.color
	}

	if originalColor == black {
		t.fixDelete(child, node.parent)
	}
}

// fixDelete restores red-black properties after deletion.
func (t *RBTree) fixDelete(node *rbnode, parent *rbnode) {
	for node != t.root && (node == nil || node.color == black) {
		switch {
		case parent == nil:
			return
		default:
			if !t.fixDeleteCase(&node, &parent) {
				return
			}
		}
	}

	if node != nil {
		node.color = black
	}
}

// fixDeleteCase handles a single case of deletion fix-up.
// Returns false if no more fix-up is needed.
func (t *RBTree) fixDeleteCase(node **rbnode, parent **rbnode) bool {
	isLeftChild := *node == (*parent).left
	sibling := t.getSibling(*parent, isLeftChild)
	if sibling == nil {
		return false
	}

	// Case 1: Sibling is red
	if sibling.color == red {
		t.handleRedSibling(*parent, sibling, isLeftChild)
		sibling = t.getSibling(*parent, isLeftChild)
		if sibling == nil {
			return false
		}
	}

	// Get sibling's children after potential rotation
	leftChild, rightChild := t.getSiblingChildren(sibling, isLeftChild)

	// Case 2: Both sibling's children are black
	if t.areBothChildrenBlack(leftChild, rightChild) {
		sibling.color = red
		if (*parent).color == red {
			(*parent).color = black

			return false
		}
		*node = *parent
		*parent = (*node).parent

		return true
	}

	// Case 3: Inner child is black, outer child is red
	if t.isInnerChildBlack(leftChild, rightChild, isLeftChild) {
		t.handleInnerBlackChild(sibling, isLeftChild)
		sibling = t.getSibling(*parent, isLeftChild)
		if sibling == nil {
			return false
		}
		leftChild, rightChild = t.getSiblingChildren(sibling, isLeftChild)
	}

	// Case 4: Inner child is red
	t.handleInnerRedChild(*parent, sibling, leftChild, rightChild, isLeftChild)

	return false
}

// handleRedSibling handles case 1: sibling is red.
func (t *RBTree) handleRedSibling(parent *rbnode, sibling *rbnode, isLeftChild bool) {
	sibling.color = black
	parent.color = red
	if isLeftChild {
		t.rotateLeft(parent)
	} else {
		t.rotateRight(parent)
	}
}

// handleInnerBlackChild handles case 3: inner child is black.
func (t *RBTree) handleInnerBlackChild(sibling *rbnode, isLeftChild bool) {
	if isLeftChild {
		if sibling.left != nil {
			sibling.left.color = black
		}
		t.rotateRight(sibling)
	} else {
		if sibling.right != nil {
			sibling.right.color = black
		}
		t.rotateLeft(sibling)
	}
}

// handleInnerRedChild handles case 4: inner child is red.
func (t *RBTree) handleInnerRedChild(
	parent *rbnode,
	sibling *rbnode,
	leftChild *rbnode,
	rightChild *rbnode,
	isLeftChild bool,
) {
	sibling.color = parent.color
	parent.color = black

	if isLeftChild {
		if rightChild != nil {
			rightChild.color = black
		}
		t.rotateLeft(parent)
	} else {
		if leftChild != nil {
			leftChild.color = black
		}
		t.rotateRight(parent)
	}
}

// minimum returns the node with minimum priority in the subtree rooted at node.
func (t *RBTree) minimum(node *rbnode) *rbnode {
	for node.left != nil {
		node = node.left
	}

	return node
}

// transplant replaces u with v in the tree.
func (t *RBTree) transplant(u, v *rbnode) {
	switch {
	case u.parent == nil:
		t.root = v
	case u == u.parent.left:
		u.parent.left = v
	default:
		u.parent.right = v
	}

	if v != nil {
		v.parent = u.parent
	}
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
		return nil, ErrNotFound
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
	t.root = nil
	t.length = 0
}

// Peek will find a node by priority.
func (t *RBTree) Peek(priority uint16) (*rtp.Packet, error) {
	return t.Find(priority)
}

// getSibling returns the sibling of the given node.
func (t *RBTree) getSibling(node *rbnode, isLeftChild bool) *rbnode {
	if node == nil {
		return nil
	}

	if isLeftChild {
		return node.right
	}

	return node.left
}

// getSiblingChildren returns the children of the given sibling.
func (t *RBTree) getSiblingChildren(sibling *rbnode, isLeftChild bool) (*rbnode, *rbnode) {
	if sibling == nil {
		return nil, nil
	}

	if isLeftChild {
		return sibling.left, sibling.right
	}

	return sibling.right, sibling.left
}

// areBothChildrenBlack returns true if both children of the given nodes are black.
func (t *RBTree) areBothChildrenBlack(leftChild, rightChild *rbnode) bool {
	return (leftChild == nil || leftChild.color == black) &&
		(rightChild == nil || rightChild.color == black)
}

// isInnerChildBlack returns true if the inner child of the given nodes is black.
func (t *RBTree) isInnerChildBlack(leftChild, rightChild *rbnode, isLeftChild bool) bool {
	if isLeftChild {
		return rightChild == nil || rightChild.color == black
	}

	return leftChild == nil || leftChild.color == black
}
