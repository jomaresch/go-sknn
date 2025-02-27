package go_sknn

import (
	"sync"

	"github.com/golang/geo/s2"
)

const (
	maxValuesPerCell = 8
)

type Node[T any] struct {
	cellID        s2.CellID
	values        []*Value[T]
	children      []*Node[T]
	parent        *Node[T]
	childMutex    sync.RWMutex
	valuesMutex   sync.RWMutex
	maxIndexDepth int
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

func (n *Node[T]) GetOrCreateChild(childCellID s2.CellID) *Node[T] {
	n.childMutex.RLock()
	for _, child := range n.children {
		if child.cellID == childCellID {
			n.childMutex.RUnlock()
			return child
		}
	}
	n.childMutex.RUnlock()

	n.childMutex.Lock()
	defer n.childMutex.Unlock()
	for _, child := range n.children {
		if child.cellID == childCellID {
			return child
		}
	}

	child := &Node[T]{
		cellID:        childCellID,
		values:        []*Value[T]{},
		children:      make([]*Node[T], 0, 1),
		parent:        n,
		childMutex:    sync.RWMutex{},
		valuesMutex:   sync.RWMutex{},
		maxIndexDepth: n.maxIndexDepth,
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

func (n *Node[T]) AddValue(key string, value T, cell s2.CellID) *Node[T] {
	valueChildCell := cell.Parent(n.cellID.Level() + 1)
	n.childMutex.RLock()
	hasChildren := len(n.children) != 0
	n.childMutex.RUnlock()
	// If the node has children, add the value to the child node.
	if hasChildren {
		return n.GetOrCreateChild(valueChildCell).AddValue(key, value, cell)
	}

	n.valuesMutex.Lock()
	defer n.valuesMutex.Unlock()

	// If the values in the node don't exceed the maximum, add the value to the node and return
	if len(n.values)+1 <= maxValuesPerCell {
		n.values = append(n.values, &Value[T]{key: key, value: value, cell: cell})
		return n
	}
	// If is already at the max depth, add the value to the node and return,
	// because we can't split a node which is already at max depth.
	if n.cellID.Level() >= n.maxIndexDepth {
		n.values = append(n.values, &Value[T]{key: key, value: value, cell: cell})
		return n
	}
	// If the node is not at the max depth, split the node.
	// Iterate over the values and add them to the children of this node they belong to.
	for _, v := range n.values {
		n.GetOrCreateChild(v.cell.Parent(n.cellID.Level()+1)).AddValue(v.key, v.value, cell)
	}
	// Remove all values, because they are all added to the children of this node.
	n.values = nil
	// Add the new value to the child node.
	return n.GetOrCreateChild(valueChildCell).AddValue(key, value, cell)
}

func (n *Node[T]) UpdateValue(key string, value T) {
	for index := range n.values {
		if n.values[index].key == key {
			n.values[index].value = value
		}
	}
}

func (n *Node[T]) IsLeaveNode() bool {
	n.childMutex.RLock()
	defer n.childMutex.RUnlock()
	return len(n.children) == 0
}

func (n *Node[T]) RemoveValue(key string) {
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
}

func (n *Node[T]) Prune() {
	n.valuesMutex.RLock()
	defer n.valuesMutex.RUnlock()
	if len(n.values) != 0 {
		return
	}
	if len(n.children) == 0 {
		return
	}
	n.parent.RemoveChild(n.cellID)
	n.parent = nil
}

func (n *Node[T]) RemoveChild(id s2.CellID) {
	n.childMutex.Lock()
	defer n.childMutex.Unlock()

	for i, child := range n.children {
		if child.cellID == id {
			n.children = append(n.children[:i], n.children[i+1:]...)
			return
		}
	}
}
