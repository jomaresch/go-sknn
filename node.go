package go_sknn

import (
	"sync"

	"github.com/golang/geo/s2"
)

type Node[T any] struct {
	cellID      s2.CellID
	values      []*Value[T]
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
		values:      []*Value[T]{},
		children:    make([]*Node[T], 0, 1),
		isLeaveNode: isLeaveNode,
		parent:      n,
		childMutex:  sync.RWMutex{},
		valuesMutex: sync.RWMutex{},
	}
	n.children = append(n.children, child)
	return child
}

func (n *Node[T]) AddChildrenToQueue(point s2.Point, addFunction func(*Node[T], float64)) {
	n.childMutex.RLock()
	defer n.childMutex.RUnlock()
	for _, child := range n.children {
		addFunction(child, float64(s2.CellFromCellID(child.cellID).Distance(point)))
	}
}

func (n *Node[T]) AddChildrenToQueueInterface(point s2.Point, addFunction func(interface{}, float64)) {
	n.childMutex.RLock()
	defer n.childMutex.RUnlock()
	for _, child := range n.children {
		addFunction(child, float64(s2.CellFromCellID(child.cellID).Distance(point)))
	}
}

func (n *Node[T]) AddValuesToQueue(point s2.Point, addFunction func(interface{}, float64)) {
	n.valuesMutex.RLock()
	defer n.valuesMutex.RUnlock()
	for _, value := range n.values {
		addFunction(value, float64(s2.CellFromCellID(value.cell).Distance(point)))
	}
}

func (n *Node[T]) FilerValues(callback func(*Value[T]) bool) bool {
	n.valuesMutex.RLock()
	defer n.valuesMutex.RUnlock()

	for _, value := range n.values {
		if callback(value) {
			return true
		}
	}

	return false
}

func (n *Node[T]) SetValue(key string, value T, cell s2.CellID) {
	n.valuesMutex.Lock()
	defer n.valuesMutex.Unlock()
	n.values = append(n.values, &Value[T]{key: key, value: value, cell: cell})
}

func (n *Node[T]) IsLeaveNode() bool {
	return n.isLeaveNode
}

func (n *Node[T]) RemoveValue(key string) bool {
	n.valuesMutex.Lock()
	defer n.valuesMutex.Unlock()
	foundIndex := -1
	for i := range n.values {
		if n.values[i].key == key {
			foundIndex = i
			break
		}
	}
	if foundIndex != -1 {
		n.values[foundIndex] = n.values[len(n.values)-1]
		n.values = n.values[:len(n.values)-1]
	}
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
