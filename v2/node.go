package v2

import (
	"context"
	"sync"

	"github.com/golang/geo/s2"
)

type Node[T any] struct {
	cellID      s2.CellID
	values      map[string]T
	children    []*Node[T]
	isLeaveNode bool
	parent      *Node[T]
	childMutex  sync.RWMutex
	valuesMutex sync.RWMutex
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

// GetOrCreateChild ✅
func (n *Node[T]) GetOrCreateChild(key s2.CellID, isLeaveNode bool) *Node[T] {
	n.childMutex.RLock()
	for _, child := range n.children {
		if child.cellID == key {
			n.childMutex.RUnlock()
			return child
		}
	}
	n.childMutex.RUnlock()

	n.childMutex.Lock()
	defer n.childMutex.Unlock()
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
		childMutex:  sync.RWMutex{},
	}
	n.children = append(n.children, child)
	return child
}

// AddChildrenToQueue ✅
func (n *Node[T]) AddChildrenToQueue(point s2.Point, addFunction func(*Node[T], float64)) {
	n.childMutex.RLock()
	defer n.childMutex.RUnlock()
	for _, child := range n.children {
		addFunction(child, float64(s2.CellFromCellID(child.cellID).Distance(point)))
	}
}

// FilerValues ✅
func (n *Node[T]) FilerValues(ctx context.Context, results []T, distance float64, filter FilterFunc[T]) ([]T, bool) {
	n.valuesMutex.RLock()
	defer n.valuesMutex.RUnlock()

	for _, value := range n.values {
		switch filter(ctx, distance, value, results) {
		case FilterType_Continue:
			results = append(results, value)
		case FilterType_Stop:
			return results, true
		case FilterType_Skip:
			continue
		default:
			panic("unknown filter type")
		}
	}

	return results, false
}

// SetValue ✅
func (n *Node[T]) SetValue(key string, value T) {
	n.valuesMutex.Lock()
	defer n.valuesMutex.Unlock()
	n.values[key] = value
}

func (n *Node[T]) IsLeaveNode() bool {
	return n.isLeaveNode
}

// RemoveValue ✅
func (n *Node[T]) RemoveValue(key string) bool {
	n.valuesMutex.Lock()
	defer n.valuesMutex.Unlock()
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

func (n *Node[T]) Prune() bool {
	for _, child := range n.children {
		if child.isLeaveNode {
			return len(n.values) == 0
		}
		empty := child.Prune()
		if empty {
			n.RemoveChild(child.cellID)
		}
	}
	return len(n.children) == 0
}
