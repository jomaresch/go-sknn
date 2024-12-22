package v1

import (
	"github.com/golang/geo/s2"
)

type Node[T any] struct {
	cellID      s2.CellID
	values      map[string]T
	children    []*Node[T]
	isLeaveNode bool
	parent      *Node[T]
}

func (n *Node[T]) ValuesCount() []int {
	result := make([]int, 0)
	for _, child := range n.children {
		result = append(result, child.ValuesCount()...)
	}
	if len(n.values) > 0 {
		result = append(result, len(n.values))
	}
	return result
}

func (n *Node[T]) GetOrCreateChild(key s2.CellID, isLeaveNode bool) *Node[T] {
	for _, child := range n.children {
		if child.cellID == key {
			return child
		}
	}

	for _, child := range n.children {
		if child.cellID == key {
			return child
		}
	}

	child := &Node[T]{
		cellID:      key,
		values:      make(map[string]T),
		children:    []*Node[T]{},
		isLeaveNode: isLeaveNode,
		parent:      n,
	}
	n.children = append(n.children, child)
	return child
}

func (n *Node[T]) Children() []*Node[T] {
	return n.children
}

func (n *Node[T]) Values() map[string]T {
	return n.values
}

func (n *Node[T]) AddValue(key string, value T) {
	n.values[key] = value
}

func (n *Node[T]) IsLeaveNode() bool {
	return n.isLeaveNode
}

func (n *Node[T]) RemoveValue(key string) bool {
	delete(n.values, key)
	return len(n.values) == 0
}

func (n *Node[T]) RemoveChild(id s2.CellID) {
	for i, child := range n.children {
		if child.cellID == id {
			n.children = append(n.children[:i], n.children[i+1:]...)
			return
		}
	}
}
