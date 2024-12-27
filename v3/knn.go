package v3

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/geo/s2"
	"github.com/oleiade/lane/v2"
)

const (
	MinPrecision = 0
	MaxPrecision = 30
)

type KNN[T any] struct {
	indexRoot   *Node[T]
	lookup      map[string]*Node[T]
	precision   int
	lookupMutex sync.RWMutex
	pruneMutex  sync.RWMutex
}

func NewKNN[T any](precision int) (*KNN[T], error) {
	if precision < MinPrecision || precision > MaxPrecision {
		return nil, fmt.Errorf("invalid precision %d: precision must be between %d and %d", precision, MinPrecision, MaxPrecision)
	}
	return &KNN[T]{
		indexRoot: &Node[T]{},
		lookup:    make(map[string]*Node[T]),
		precision: precision,
	}, nil
}

// AddValue adds a new value to the search tree.
// The function will panic if the latitude or longitude are out of bounds.
func (a *KNN[T]) AddValue(id string, value T, lat float64, long float64) {
	// The pruneMutex is used to prevent pruning while adding a new value.
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()

	if long < -180 || long > 180 || lat < -90 || lat > 90 {
		panic(fmt.Sprintf("invalid latitude %f (Min:-90, Max 90) or longitude %f (Min: -180, Max 180)", lat, long))
	}
	// Calculate the Cell which the value belongs to.
	cellID := s2.CellIDFromLatLng(s2.LatLngFromDegrees(lat, long))
	node := a.indexRoot
	// Building the tree-path, by going from precision 0 to a.precision.
	for i := range a.precision {
		node = node.GetOrCreateChild(cellID.Parent(i), a.precision-1 == i)
	}
	// Add the value to the leave node.
	node.SetValue(id, value, cellID)
	// Add the node to the lookup map.
	a.lookupMutex.Lock()
	a.lookup[id] = node
	a.lookupMutex.Unlock()
}

// RemoveValue removes a value from the search tree.
// The function will return false if the value was not found and true if the value
// was removed successfully.
func (a *KNN[T]) RemoveValue(id string) bool {
	// The pruneMutex is used to prevent pruning while removing a value.
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()

	a.lookupMutex.Lock()
	defer a.lookupMutex.Unlock()

	node, ok := a.lookup[id]
	if !ok {
		return false
	}
	// Remove the value from the search index.
	node.RemoveValue(id)
	// Remove the value from the lookup map.
	delete(a.lookup, id)
	return true
}

// UpsertValue updates a value in the search tree or inserts the value if it does not exist.
// The function will panic if the latitude or longitude are out of bounds.
func (a *KNN[T]) UpsertValue(id string, value T, lat float64, long float64) {
	// The pruneMutex is used to prevent pruning while updating a value.
	a.pruneMutex.RLock()
	defer a.pruneMutex.Unlock()

	// Check if we have to update or insert the value.
	cellID := s2.CellIDFromLatLng(s2.LatLngFromDegrees(lat, long))
	a.lookupMutex.RLock()
	node, ok := a.lookup[id]
	a.lookupMutex.Unlock()

	// If the value does not exist, we add it.
	if !ok {
		a.AddValue(id, value, lat, long)
		return
	}
	// If the value exists, we update it.
	// If the cell is the same, we just have to update the value in the node.
	// This avoids removing and adding the valid from the node, which is more expensive.
	if node.cellID == cellID {
		node.SetValue(id, value, cellID)
		return
	}
	// If the cell has changed, the only way to update the value is to remove it and add it again.
	a.RemoveValue(id)
	a.AddValue(id, value, lat, long)
}

// SearchApproximate performs an approximate nearest neighbor search in the K-Nearest Neighbors (KNN) index.
// It searches for values in the tree that are closest to a given latitude and longitude.
// The callback function is called for each value found, and the search stops if the callback returns true or if the context is canceled.
//
// The found values are not guaranteed to be ordered perfectly by distance.
// It has an error margin which is defines by the precision of the KNN.
// A higher precision will result in a more accurate search but will be slower and consume more memory.
func (a *KNN[T]) SearchApproximate(ctx context.Context, lat float64, long float64, callback func(*Value[T]) bool) {
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, long))
	priorityQueue := lane.NewMinPriorityQueue[*Node[T], float64]()
	priorityQueue.Push(a.indexRoot, 0)
	for {
		if ctx.Err() != nil {
			return
		}
		poppedNode, _, ok := priorityQueue.Pop()
		if !ok {
			return
		}
		if poppedNode.IsLeaveNode() {
			if poppedNode.FilerValues(callback) {
				return
			}
		} else {
			poppedNode.AddChildrenToQueue(point, priorityQueue.Push)
		}
	}
}

// Search performs an exact nearest neighbor search in the K-Nearest Neighbors (KNN) index.
// It has the same specification as SearchApproximate, but the values are guaranteed to be ordered by distance.
func (a *KNN[T]) Search(ctx context.Context, lat float64, long float64, callback func(*Value[T]) bool) {
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, long))
	priorityQueue := lane.NewMinPriorityQueue[interface{}, float64]()
	priorityQueue.Push(a.indexRoot, 0)
	for {
		if ctx.Err() != nil {
			return
		}
		poppedNode, _, ok := priorityQueue.Pop()
		if !ok {
			return
		}
		switch node := poppedNode.(type) {
		case *Node[T]:
			if node.IsLeaveNode() {
				node.AddValuesToQueue(point, priorityQueue.Push)
			} else {
				node.AddChildrenToQueueInterface(point, priorityQueue.Push)
			}
		case *Value[T]:
			if callback(node) {
				return
			}
		}
	}
}

// Prune removes all nodes without values from the search tree.
func (a *KNN[T]) Prune() {
	a.pruneMutex.Lock()
	defer a.pruneMutex.Unlock()
	a.indexRoot.Prune()
}
