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
		indexRoot:   &Node[T]{},
		lookup:      make(map[string]*Node[T]),
		precision:   precision,
		lookupMutex: sync.RWMutex{},
		pruneMutex:  sync.RWMutex{},
	}, nil
}

// AddValue adds a new value to the search tree.
// The function will panic if the latitude or longitude are out of bounds.
func (a *KNN[T]) AddValue(id string, value T, lat float64, long float64) {
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
	defer a.lookupMutex.Unlock()
	a.lookup[id] = node
}

// RemoveValue removes a value from the search tree.
func (a *KNN[T]) RemoveValue(id string) bool {
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()

	a.lookupMutex.Lock()
	defer a.lookupMutex.Unlock()

	node, ok := a.lookup[id]
	if !ok {
		return false
	}

	node.RemoveValue(id)
	delete(a.lookup, id)
	return true
}

// UpdateValue updates a value in the search tree.
// The function will panic if the latitude or longitude are out of bounds.
func (a *KNN[T]) UpdateValue(id string, value T, lat float64, long float64) {
	a.pruneMutex.RLock()
	defer a.pruneMutex.Unlock()

	cellID := s2.CellIDFromLatLng(s2.LatLngFromDegrees(lat, long))
	a.lookupMutex.RLock()
	node, ok := a.lookup[id]
	a.lookupMutex.Unlock()
	if !ok {
		a.AddValue(id, value, lat, long)
		return
	}
	if node.cellID == cellID {
		node.SetValue(id, value, cellID)
		return
	}
	a.RemoveValue(id)
	a.AddValue(id, value, lat, long)
}

func (a *KNN[T]) SearchApproximate(ctx context.Context, lat float64, long float64, callback func(*Value[T]) bool) {
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()
	// Define the search location as a S2 point.
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, long))
	// Create a new priority queue, where the lowest value is always at the root.
	// This way we can assure, that we always pop the node with the shortest
	// distance to the search location
	priorityQueue := lane.NewMinPriorityQueue[*Node[T], float64]()
	// Add the root of the index to the queue.
	priorityQueue.Push(a.indexRoot, 0)
	for {
		// Check for a canceled context.
		if ctx.Err() != nil {
			return
		}
		// Pop one node from the queue. Since the queue is a priority queue,
		// this node always has the shortest distance to the search location.
		poppedNode, _, ok := priorityQueue.Pop()
		// Not ok means that the queue is empty and we can return the results.
		if !ok {
			return
		}
		// If the node is a leave node, we can iterate of the values and filter them.
		// Due to the nature of the priority queue, this leave node has the shortest
		// distance off all nodes to the search location. Since this search is just an
		// approximate nearest neighbour search, we assume that all items in this node
		// have roughly the same distance to the search location.
		if poppedNode.IsLeaveNode() {
			// Filter the values and add the to the result slice.
			if poppedNode.FilerValues(callback) {
				return
			}
		} else {
			// Calculate the distance to all children nodes and add them to the queue.
			// The queue will sort the children nodes, together with the other nodes,
			// by distance to the search location.
			poppedNode.AddChildrenToQueue(point, priorityQueue.Push)
		}
	}
}

func (a *KNN[T]) Search(ctx context.Context, lat float64, long float64, callback func(*Value[T]) bool) {
	a.pruneMutex.RLock()
	defer a.pruneMutex.RUnlock()
	// Define the search location as a S2 point.
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, long))
	// Create a new priority queue, where the lowest value is always at the root.
	// This way we can assure, that we always pop the node with the shortest
	// distance to the search location
	priorityQueue := lane.NewMinPriorityQueue[interface{}, float64]()
	// Add the root of the index to the queue.
	priorityQueue.Push(a.indexRoot, 0)
	for {
		// Check for a canceled context.
		if ctx.Err() != nil {
			return
		}
		// Pop one node from the queue. Since the queue is a priority queue,
		// this node always has the shortest distance to the search location.
		poppedNode, _, ok := priorityQueue.Pop()
		// Not ok means that the queue is empty and we can return the results.
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

func (a *KNN[T]) Prune() {
	a.pruneMutex.Lock()
	defer a.pruneMutex.Unlock()
	a.indexRoot.Prune()
}
