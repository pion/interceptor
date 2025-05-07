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

func TestRotations(t *testing.T) {
	t.Run("rotateLeft", func(t *testing.T) {
		tree := NewTree()

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

		// After rotating left around 5:
		//       7
		//      / \
		//     5   9
		tree.rotateLeft(root)

		// Verify structure
		assert.Equal(t, uint16(7), tree.root.priority)
		assert.Equal(t, uint16(5), tree.root.left.priority)
		assert.Equal(t, uint16(9), tree.root.right.priority)

		// Verify parent pointers
		assert.Nil(t, tree.root.parent)
		assert.Equal(t, tree.root, tree.root.left.parent)
		assert.Equal(t, tree.root, tree.root.right.parent)
	})

	t.Run("rotateRight", func(t *testing.T) {
		tree := NewTree()

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

		// After rotating right around 7:
		//     5
		//    / \
		//   3   7
		tree.rotateRight(root)

		// Verify structure
		assert.Equal(t, uint16(5), tree.root.priority)
		assert.Equal(t, uint16(3), tree.root.left.priority)
		assert.Equal(t, uint16(7), tree.root.right.priority)

		// Verify parent pointers
		assert.Nil(t, tree.root.parent)
		assert.Equal(t, tree.root, tree.root.left.parent)
		assert.Equal(t, tree.root, tree.root.right.parent)
	})
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

func TestRBTreeProperties(t *testing.T) {
	t.Run("Tree Properties", func(t *testing.T) {
		tree := NewTree()
		values := []uint16{5004, 5000, 5002, 5001, 5003, 5005, 5006, 5007, 5008, 5009, 5010}

		for _, v := range values {
			tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
		}

		// Property 1: Root must be black
		assert.Equal(t, black, tree.root.color, "Root must be black")

		// Property 2: Red nodes must have black children
		var checkRedNodes func(*rbnode) bool
		checkRedNodes = func(node *rbnode) bool {
			if node == nil {
				return true
			}
			if node.color == red {
				if node.left != nil && node.left.color == red {
					t.Errorf("Red node %d has red left child %d", node.priority, node.left.priority)

					return false
				}
				if node.right != nil && node.right.color == red {
					t.Errorf("Red node %d has red right child %d", node.priority, node.right.priority)

					return false
				}
			}

			return checkRedNodes(node.left) && checkRedNodes(node.right)
		}

		checkRedNodes(tree.root)
	})

	t.Run("Black Path Property", func(t *testing.T) {
		tree := NewTree()
		values := []uint16{5004, 5000, 5002, 5001, 5003, 5005, 5006, 5007, 5008, 5009, 5010}
		for _, v := range values {
			tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
		}

		// Count black nodes in path to each leaf
		var blackPathLength func(*rbnode, int) []int
		blackPathLength = func(n *rbnode, blackCount int) []int {
			if n == nil {
				return []int{blackCount}
			}
			if n.color == black {
				blackCount++
			}
			leftPaths := blackPathLength(n.left, blackCount)
			rightPaths := blackPathLength(n.right, blackCount)

			return append(leftPaths, rightPaths...)
		}

		paths := blackPathLength(tree.root, 0)
		firstPath := paths[0]
		for i, pathLen := range paths {
			if pathLen != firstPath {
				t.Errorf("Unequal black path lengths: path %d has %d black nodes, expected %d",
					i, pathLen, firstPath)
			}
		}
	})

	t.Run("Black Node Children", func(t *testing.T) {
		tree := NewTree()
		values := []uint16{5004, 5000, 5002, 5001, 5003, 5005, 5006, 5007, 5008, 5009, 5010}
		for _, v := range values {
			tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
		}

		var checkRedNodes func(*rbnode) bool
		checkRedNodes = func(node *rbnode) bool {
			if node == nil {
				return true
			}
			if node.color == red {
				// Check that red nodes have black children
				if node.left != nil && node.left.color == red {
					t.Errorf("Red node %d has red left child %d", node.priority, node.left.priority)

					return false
				}
				if node.right != nil && node.right.color == red {
					t.Errorf("Red node %d has red right child %d", node.priority, node.right.priority)

					return false
				}
			}

			return checkRedNodes(node.left) && checkRedNodes(node.right)
		}

		checkRedNodes(tree.root)
	})
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

func TestBalancedStructure(t *testing.T) {
	t.Run("Sequential Right-Side Insertions", func(t *testing.T) {
		tree := NewTree()
		values := []uint16{5004, 5000, 5002, 5001, 5003, 5005, 5006, 5007, 5008, 5009, 5010}

		for _, v := range values {
			tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: v}})
		}

		// Check Red-Black tree properties
		var verifyProperties func(*rbnode) (int, bool)
		verifyProperties = func(node *rbnode) (blackHeight int, valid bool) {
			if node == nil {
				return 0, true
			}

			// Property 1: Node is either red or black (implicit in our implementation)

			// Property 2: Red nodes cannot have red children
			if node.color == red {
				if (node.left != nil && node.left.color == red) ||
					(node.right != nil && node.right.color == red) {
					t.Errorf("Red node %d has a red child", node.priority)

					return 0, false
				}
			}

			leftHeight, leftValid := verifyProperties(node.left)
			rightHeight, rightValid := verifyProperties(node.right)

			// Property 3: All paths must have same number of black nodes
			if leftHeight != rightHeight {
				t.Errorf("Black height mismatch at node %d: left=%d, right=%d",
					node.priority, leftHeight, rightHeight)

				return 0, false
			}

			// Calculate black height of current path
			currentBlackHeight := leftHeight
			if node.color == black {
				currentBlackHeight++
			}

			return currentBlackHeight, leftValid && rightValid
		}

		// Property 4: Root must be black
		if tree.root.color != black {
			t.Error("Root node is not black")
		}

		verifyProperties(tree.root)
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
