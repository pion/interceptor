// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestTreeRotation(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*RBTree) *rbnode
		rotate   func(*RBTree, *rbnode)
		expected struct {
			root     uint16
			left     uint16
			right    uint16
			rootLeft uint16
		}
	}{
		{
			name: "rotateLeft",
			setup: func(tree *RBTree) *rbnode {
				// Create a simple tree:
				//     5
				//      \
				//       7
				//        \
				//         9
				root := &rbnode{priority: 5, color: black}
				right := &rbnode{priority: 7, color: red}
				rightRight := &rbnode{priority: 9, color: red}

				tree.root = root
				root.right = right
				right.parent = root
				right.right = rightRight
				rightRight.parent = right

				return root
			},
			rotate: func(tree *RBTree, node *rbnode) {
				tree.rotateLeft(node)
			},
			expected: struct {
				root     uint16
				left     uint16
				right    uint16
				rootLeft uint16
			}{
				root:  7,
				left:  5,
				right: 9,
			},
		},
		{
			name: "rotateRight",
			setup: func(tree *RBTree) *rbnode {
				// Create a simple tree:
				//       7
				//      /
				//     5
				//    /
				//   3
				root := &rbnode{priority: 7, color: black}
				left := &rbnode{priority: 5, color: red}
				leftLeft := &rbnode{priority: 3, color: red}

				tree.root = root
				root.left = left
				left.parent = root
				left.left = leftLeft
				leftLeft.parent = left

				return root
			},
			rotate: func(tree *RBTree, node *rbnode) {
				tree.rotateRight(node)
			},
			expected: struct {
				root     uint16
				left     uint16
				right    uint16
				rootLeft uint16
			}{
				root:  5,
				left:  3,
				right: 7,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := NewTree()
			root := tt.setup(tree)
			tt.rotate(tree, root)

			// Verify structure
			assert.Equal(t, tt.expected.root, tree.root.priority)
			assert.Equal(t, tt.expected.left, tree.root.left.priority)
			assert.Equal(t, tt.expected.right, tree.root.right.priority)

			// Verify parent pointers
			assert.Nil(t, tree.root.parent)
			assert.Equal(t, tree.root, tree.root.left.parent)
			assert.Equal(t, tree.root, tree.root.right.parent)
		})
	}
}

func TestPriorityQueueReordersOnPop(t *testing.T) {
	assert := assert.New(t)
	tree := NewTree()

	t.Run("rotateRight", func(_ *testing.T) {
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5003, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5005, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5006, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5007, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5009, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5010, Timestamp: 500}, Payload: []byte{0x02}})

		// verify elements are Pop'd, the order of which depends on insertion order since Pop without a
		// priority will pop the root of the tree.
		order := []uint16{5000, 5001, 5002, 5003, 5004, 5005, 5006, 5007, 5008, 5009, 5010}
		popped := []uint16{}
		for range order {
			item, err := tree.Pop()
			assert.NoError(err)
			popped = append(popped, item.SequenceNumber)
		}
		slices.Sort(popped)
		assert.Equal(order, popped)
	})
}

func TestPriorityQueue(t *testing.T) {
	assert := assert.New(t)
	tree := NewTree()

	t.Run("rotateRight", func(_ *testing.T) {
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5003, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5005, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5006, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5007, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5009, Timestamp: 500}, Payload: []byte{0x02}})
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5010, Timestamp: 500}, Payload: []byte{0x02}})

		assert.Equal(tree.root.priority, uint16(5004))
		// Verify tree maintains Red-Black properties
		assert.NoError(validateRBProperties(tree))
		// Verify all the elements inserted are in the tree
		for _, v := range []uint16{5004, 5000, 5002, 5001, 5003, 5005, 5006, 5007, 5008, 5009, 5010} {
			packet, err := tree.Peek(v)
			assert.NoError(err)
			assert.Equal(v, packet.SequenceNumber)
		}
	})
}

// checkRedBlackProperties checks the red-black properties of the tree.
func checkRedBlackProperties(node *rbnode, assert *assert.Assertions) bool {
	if node == nil {
		return true
	}

	// Property 1: Root is black
	if node.parent == nil && node.color != black {
		assert.Fail("Root node is not black")

		return false
	}

	// Property 2: Red nodes have black children
	if node.color == red {
		if node.left != nil && node.left.color == red {
			assert.Failf("Red node has red left child", "Node priority: %v", node.priority)

			return false
		}
		if node.right != nil && node.right.color == red {
			assert.Failf("Red node has red right child", "Node priority: %v", node.priority)

			return false
		}
	}

	return checkRedBlackProperties(node.left, assert) && checkRedBlackProperties(node.right, assert)
}

// checkBlackHeight checks if all paths from root to leaves have the same black height.
func checkBlackHeight(node *rbnode, assert *assert.Assertions) (int, bool) {
	if node == nil {
		return 1, true
	}

	leftHeight, leftValid := checkBlackHeight(node.left, assert)
	rightHeight, rightValid := checkBlackHeight(node.right, assert)

	if !leftValid || !rightValid {
		return 0, false
	}

	if leftHeight != rightHeight {
		assert.Failf("Black height mismatch", "Node priority: %v", node.priority)

		return 0, false
	}

	if node.color == black {
		return leftHeight + 1, true
	}

	return leftHeight, true
}

func TestRBTreeProperties(t *testing.T) {
	assert := assert.New(t)
	tree := NewTree()

	for i := 0; i < 101; i++ {
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: uint16(i)}})
	}

	assert.True(checkRedBlackProperties(tree.root, assert), "Red-black properties violated")

	_, valid := checkBlackHeight(tree.root, assert)
	assert.True(valid, "Black height property violated")
}

// printTree prints the tree structure for debugging.
func printTree(node *rbnode, level int) {
	if node == nil {
		return
	}
	printTree(node.right, level+1)
	for i := 0; i < level; i++ {
		print("  ")
	}
	println(node.priority, node.color)
	printTree(node.left, level+1)
}

// checkBalancedStructure checks if the tree is balanced.
func checkBalancedStructure(node *rbnode, assert *assert.Assertions) (int, bool) {
	if node == nil {
		return 0, true
	}

	leftHeight, leftValid := checkBalancedStructure(node.left, assert)
	rightHeight, rightValid := checkBalancedStructure(node.right, assert)

	if !leftValid || !rightValid {
		return 0, false
	}

	if abs(leftHeight-rightHeight) > 1 {
		assert.Failf("Tree is not balanced", "Node priority: %v, left depth: %d, right depth: %d, left subtree: %v, right subtree: %v",
			node.priority, leftHeight, rightHeight, node.left, node.right)
		return 0, false
	}

	return max(leftHeight, rightHeight) + 1, true
}

func TestTreeStructure(t *testing.T) {
	t.Run("Sequential Insertion Structure", func(t *testing.T) {
		tree := NewTree()

		// Insert first node (becomes root)
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5004}})

		// Verify initial state
		assert.Equal(t, uint16(5004), tree.root.priority)
		assert.Equal(t, black, tree.root.color)

		// Insert second node
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002}})

		// Verify after second insertion
		assert.Equal(t, uint16(5004), tree.root.priority)
		assert.Equal(t, black, tree.root.color)
		assert.Equal(t, uint16(5002), tree.root.left.priority)
		assert.Equal(t, red, tree.root.left.color)

		// Insert third node
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000}})

		// Verify final state after rebalancing
		assert.Equal(t, uint16(5002), tree.root.priority)
		assert.Equal(t, black, tree.root.color)
		assert.Equal(t, uint16(5000), tree.root.left.priority)
		assert.Equal(t, red, tree.root.left.color)
		assert.Equal(t, uint16(5004), tree.root.right.priority)
		assert.Equal(t, red, tree.root.right.color)
	})
}

func TestPeekAndDelete(t *testing.T) {
	t.Run("Peek Operations", func(t *testing.T) {
		tree := NewTree()

		// Test empty tree
		_, err := tree.Peek(5000)
		assert.Error(t, err, "Peek on empty tree should return error")

		values := []uint16{5004, 5000, 5002, 5001, 5003}
		for _, v := range values {
			tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
		}

		// Test finding existing values
		for _, v := range values {
			packet, peekErr := tree.Peek(v)
			assert.NoError(t, peekErr)
			assert.Equal(t, v, packet.SequenceNumber)
		}

		// Test finding non-existent value
		_, err = tree.Peek(5999)
		assert.Error(t, err, "Peek for non-existent value should return error")
	})

	t.Run("Delete Operations", func(t *testing.T) {
		t.Run("Delete Leaf Node", func(t *testing.T) {
			tree := NewTree()
			values := []uint16{5004, 5000, 5002}
			for _, v := range values {
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
			}
			err := tree.Delete(5000)
			assert.NoError(t, err)

			_, err = tree.Peek(5000)
			assert.Error(t, err)

			assert.NoError(t, validateRBProperties(tree))
		})

		t.Run("Delete Node with One Child", func(t *testing.T) {
			tree := NewTree()
			values := []uint16{5004, 5002, 5000}
			for _, v := range values {
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
			}

			err := tree.Delete(5002)
			assert.NoError(t, err)

			assert.Equal(t, uint16(5004), tree.root.priority)
			assert.Equal(t, uint16(5000), tree.root.left.priority)

			_, err = tree.Peek(5002)
			assert.Error(t, err)
		})

		t.Run("Delete Node with Two Children", func(t *testing.T) {
			tree := NewTree()
			values := []uint16{5004, 5000, 5002, 5001, 5003}
			for _, v := range values {
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
			}

			err := tree.Delete(5002)
			assert.NoError(t, err)

			assert.NoError(t, validateRBProperties(tree))

			_, err = tree.Peek(5002)
			assert.Error(t, err)
		})

		t.Run("Delete Root", func(t *testing.T) {
			tree := NewTree()
			values := []uint16{5004, 5000, 5002}
			for _, v := range values {
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
			}

			err := tree.Delete(5004)
			assert.NoError(t, err)

			assert.Equal(t, uint16(5002), tree.root.priority)
			assert.Equal(t, black, tree.root.color)

			_, err = tree.Peek(5004)
			assert.Error(t, err)
		})

		t.Run("Delete Non-existent Node", func(t *testing.T) {
			tree := NewTree()
			tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5004}})

			err := tree.Delete(5999)
			assert.Error(t, err, "Deleting non-existent node should return error")
		})

		t.Run("Delete from Empty Tree", func(t *testing.T) {
			tree := NewTree()
			err := tree.Delete(5000)
			assert.Error(t, err, "Deleting from empty tree should return error")
		})
	})
}

func TestMemoryLeaks(t *testing.T) {
	var refs int64
	finalizer := func(*rtp.Packet) {
		atomic.AddInt64(&refs, -1)
	}

	t.Run("Insert and Delete Memory Management", func(t *testing.T) {
		tree := NewTree()
		const numOperations = 1000

		for i := uint16(0); i < numOperations; i++ {
			pkt := &rtp.Packet{Header: rtp.Header{SequenceNumber: i}}
			runtime.SetFinalizer(pkt, finalizer)
			atomic.AddInt64(&refs, 1)
			tree.Insert(pkt)
		}

		for i := uint16(0); i < numOperations; i++ {
			err := tree.Delete(i)
			assert.NoError(t, err)
		}

		runtime.GC()

		time.Sleep(time.Millisecond * 100)

		// Verify all packets were freed
		assert.Equal(t, int64(0), atomic.LoadInt64(&refs),
			"Memory leak detected: %d packets not freed", atomic.LoadInt64(&refs))

		assert.Nil(t, tree.root)
	})
}

var (
	ErrInvalidRootColor      = errors.New("root is not black")
	ErrInvalidHeight         = errors.New("invalid black height")
	ErrInvalidRedParentIsRed = errors.New("red node has red parent")
)

func validateRBProperties(tree *RBTree) error {
	if tree.root == nil {
		return nil
	}

	// Property 1: Root must be black
	if tree.root.color != black {
		return ErrInvalidRootColor
	}

	// Property 2: Red nodes must have black children
	// Property 3: All paths must have same number of black nodes
	blackHeight, err := validateNode(tree.root, black)
	if err != nil {
		return err
	}
	if blackHeight < 0 {
		return ErrInvalidHeight
	}

	return nil
}

func validateNode(node *rbnode, parentColor treeColor) (int, error) {
	if node == nil {
		return 0, nil
	}

	// Check red property violation
	if node.color == red && parentColor == red {
		return -1, ErrInvalidRedParentIsRed
	}

	// Check left subtree
	leftHeight, err := validateNode(node.left, node.color)
	if err != nil {
		return -1, err
	}

	// Check right subtree
	rightHeight, err := validateNode(node.right, node.color)
	if err != nil {
		return -1, err
	}

	// Verify black height property
	if leftHeight != rightHeight {
		return -1, ErrInvalidHeight
	}

	// Add 1 to black height if current node is black
	blackHeight := leftHeight
	if node.color == black {
		blackHeight++
	}

	return blackHeight, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
