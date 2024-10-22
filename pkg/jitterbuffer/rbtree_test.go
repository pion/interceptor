// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package jitterbuffer

import (
	"errors"
	"runtime"
	"slices"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

var (
	errRootNotBlack        = errors.New("root node is not black")
	errRedNodeRedParent    = errors.New("red node has red parent")
	errBlackHeightMismatch = errors.New("black height mismatch")
)

func TestTreeOperations(t *testing.T) {
	tests := []struct {
		name     string
		ops      func(*RBTree)
		validate func(*testing.T, *RBTree)
	}{
		{
			name: "TreeRotation",
			ops: func(tree *RBTree) {
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

				tree.rotateLeft(root)
			},
			validate: func(t *testing.T, tree *RBTree) {
				t.Helper()
				assert := assert.New(t)
				assert.Equal(uint16(7), tree.root.priority)
				assert.Equal(uint16(5), tree.root.left.priority)
				assert.Equal(uint16(9), tree.root.right.priority)
				assert.Nil(tree.root.parent)
				assert.Equal(tree.root, tree.root.left.parent)
				assert.Equal(tree.root, tree.root.right.parent)
			},
		},
		{
			name: "PriorityQueueReordering",
			ops: func(tree *RBTree) {
				packets := []uint16{5004, 5000, 5002, 5001, 5003, 5005, 5006, 5007, 5008, 5009, 5010}
				for _, seq := range packets {
					tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: 500}, Payload: []byte{0x02}})
				}
			},
			validate: func(t *testing.T, tree *RBTree) {
				t.Helper()
				assert := assert.New(t)
				expected := []uint16{5000, 5001, 5002, 5003, 5004, 5005, 5006, 5007, 5008, 5009, 5010}
				popped := []uint16{}
				for range expected {
					item, err := tree.Pop()
					assert.NoError(err)
					popped = append(popped, item.SequenceNumber)
				}
				slices.Sort(popped)
				assert.Equal(expected, popped)
			},
		},
		{
			name: "RedBlackProperties",
			ops: func(tree *RBTree) {
				for i := 0; i < 101; i++ {
					tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: safeUint16(i)}})
				}
			},
			validate: func(t *testing.T, tree *RBTree) {
				t.Helper()
				assert := assert.New(t)
				assert.True(checkRedBlackProperties(tree.root, assert), "Red-black properties violated")
				_, valid := checkBlackHeight(tree.root, assert)
				assert.True(valid, "Black height property violated")
			},
		},
		{
			name: "TreeEdgeCases",
			ops: func(tree *RBTree) {
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1}})
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 2}})
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 3}})
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 4}})
				tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5}})
			},
			validate: func(t *testing.T, tree *RBTree) {
				t.Helper()
				assert := assert.New(t)
				assert.NoError(validateRBProperties(tree))
				assert.Equal(uint16(5), tree.Length())

				// Test Pop on empty tree
				tree.Clear()
				_, err := tree.Pop()
				assert.Error(err)
				assert.Contains(err.Error(), "priority not found")

				// Test Peek on non-existent sequence
				_, err = tree.Peek(999)
				assert.Error(err)
				assert.Contains(err.Error(), "priority not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := NewTree()
			tt.ops(tree)
			tt.validate(t, tree)
		})
	}
}

func TestMemoryLeaks(t *testing.T) {
	assert := assert.New(t)
	tree := NewTree()

	// Insert and remove many packets
	for i := 0; i < 1000; i++ {
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: safeUint16(i)}})
	}

	// Force GC
	runtime.GC()

	// Get initial memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	initialAlloc := memStats.TotalAlloc

	// Perform operations that should not leak
	for i := 0; i < 1000; i++ {
		tree.Insert(&rtp.Packet{Header: rtp.Header{SequenceNumber: safeUint16(i + 1000)}})
		_, _ = tree.Pop()
	}

	// Force GC again
	runtime.GC()
	runtime.ReadMemStats(&memStats)

	// Memory usage should be stable
	assert.Less(memStats.TotalAlloc-initialAlloc, uint64(1024*1024), "Memory leak detected")
}

// Helper functions for tree validation.
func checkRedBlackProperties(node *rbnode, assert *assert.Assertions) bool {
	if node == nil {
		return true
	}

	if node.parent == nil && node.color != black {
		assert.Fail("Root node is not black")

		return false
	}

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

func validateRBProperties(tree *RBTree) error {
	if tree.root == nil {
		return nil
	}

	if tree.root.color != black {
		return errRootNotBlack
	}

	_, err := validateNode(tree.root, black)

	return err
}

func validateNode(node *rbnode, parentColor treeColor) (int, error) {
	if node == nil {
		return 1, nil
	}

	if node.color == red && parentColor == red {
		return 0, errRedNodeRedParent
	}

	leftHeight, err := validateNode(node.left, node.color)
	if err != nil {
		return 0, err
	}

	rightHeight, err := validateNode(node.right, node.color)
	if err != nil {
		return 0, err
	}

	if leftHeight != rightHeight {
		return 0, errBlackHeightMismatch
	}

	if node.color == black {
		return leftHeight + 1, nil
	}

	return leftHeight, nil
}
