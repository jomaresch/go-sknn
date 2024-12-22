package v1

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/golang/geo/s2"
	"github.com/oleiade/lane/v2"
)

const (
	MinPrecision = 0
	MaxPrecision = 30
)

type KNN[T any] struct {
	sync.RWMutex
	index     *Node[T]
	lookup    map[string]*Node[T]
	precision int
}

func NewApproximateKNNConcurrent[T any](precision int) (*KNN[T], error) {
	if precision < MinPrecision || precision > MaxPrecision {
		return nil, fmt.Errorf("invalid precision %d: precision must be between %d and %d", precision, MinPrecision, MaxPrecision)
	}
	return &KNN[T]{
		precision: precision,
		lookup:    make(map[string]*Node[T]),
		index: &Node[T]{
			cellID:      0,
			values:      make(map[string]T),
			children:    make([]*Node[T], 0),
			isLeaveNode: false,
		},
	}, nil
}

// AddValue adds a new value to the search tree.
// The function will panic if the latitude or longitude are out of bounds.
func (a *KNN[T]) AddValue(id string, value T, lat float64, long float64) {
	if long < -180 || long > 180 || lat < -90 || lat > 90 {
		panic(fmt.Sprintf("invalid latitude %f (Min:-90, Max 90) or longitude %f (Min: -180, Max 180)", lat, long))
	}
	// Calculate the Cell which the value belongs to.
	cellID := s2.CellIDFromLatLng(s2.LatLngFromDegrees(lat, long))
	a.Lock()
	defer a.Unlock()
	node := a.index
	// Building the tree-path, by going from precision 0 to a.precision.
	for i := range a.precision {
		node = node.GetOrCreateChild(cellID.Parent(i), a.precision-1 == i)
	}
	// Add the value to the leave node.
	node.AddValue(id, value)
	// Add the node to the lookup map.
	a.lookup[id] = node
}

// RemoveValue removes a value from the search tree.
func (a *KNN[T]) RemoveValue(id string) bool {
	a.Lock()
	defer a.Unlock()
	node, ok := a.lookup[id]
	if !ok {
		return false
	}
	isEmptyNode := node.RemoveValue(id)
	// To avoid a lot of empty nodes, we recursively remove all empty nodes.
	if isEmptyNode {
		for {
			// Get the parent of the current node.
			parentNode := node.parent
			// If the parent is nil, we are at the root node and can break the loop.
			if parentNode == nil {
				break
			}
			// Remove the child node from the parent.
			parentNode.RemoveChild(node.cellID)
			if len(parentNode.children) > 0 {
				break
			}
			node = parentNode
		}
	}
	delete(a.lookup, id)
	return true
}

// UpdateValue updates a value in the search tree.
// The function will panic if the latitude or longitude are out of bounds.
func (a *KNN[T]) UpdateValue(id string, value T, lat float64, long float64) {
	a.RemoveValue(id)
	a.AddValue(id, value, lat, long)
}

func (a *KNN[T]) Search(ctx context.Context, lat float64, long float64, filter func(context.Context, float64, T, []T) FilterType) []T {
	// Define the search location as a S2 point.
	point := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, long))
	// Create a new priority queue, where the lowest value is always at the root.
	// This way we can assure, that we always pop the node with the shortest
	// distance to the search location
	priorityQueue := lane.NewMinPriorityQueue[*Node[T], float64]()
	// Add the root of the index to the queue.
	priorityQueue.Push(a.index, 0)
	// Init the result array.
	var results []T

	a.RLock()
	defer a.RUnlock()
	for {
		// Check for a canceled context.
		if ctx.Err() != nil {
			return results
		}
		// Pop one node from the queue. Since the queue is a priority queue,
		// this node always has the shortest distance to the search location.
		poppedNode, distance, ok := priorityQueue.Pop()
		// Not ok means that the queue is empty and we can return the results.
		if !ok {
			return results
		}
		// If the node is a leave node, we can iterate of the values and filter them.
		// Due to the nature of the priority queue, this leave node has the shortest
		// distance off all nodes to the search location. Since this search is just an
		// approximate nearest neighbour search, we assume that all items in this node
		// have roughly the same distance to the search location.
		if poppedNode.IsLeaveNode() {
			// Filter the values and add the to the result slice.
			for _, value := range poppedNode.Values() {
				switch filter(ctx, distance, value, results) {
				case FilterType_Continue:
					results = append(results, value)
				case FilterType_Stop:
					return results
				case FilterType_Skip:
					continue
				default:
					panic("unknown filter type")
				}
			}
		} else {
			// Calculate the distance to all children nodes and add them to the queue.
			// The queue will sort the children nodes, together with the other nodes,
			// by distance to the search location.
			for _, child := range poppedNode.Children() {
				priorityQueue.Push(child, float64(s2.CellFromCellID(child.cellID).Distance(point)))
			}
		}
	}
}

func (a *KNN[T]) IndexStats() IndexStats {
	valuesCount := a.index.ValuesCount()
	stats := IndexStats{}
	for _, value := range valuesCount {
		if value > stats.MaxValuesPerLeave {
			stats.MaxValuesPerLeave = value
		}
		stats.ValueCount += value
	}
	stats.LeaveCount = len(valuesCount)
	stats.AVGValuesPerLeave = float64(stats.ValueCount) / float64(stats.LeaveCount)
	return stats
}

type IndexStats struct {
	ValueCount        int
	MaxValuesPerLeave int
	AVGValuesPerLeave float64
	LeaveCount        int
}

func (receiver IndexStats) String() string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Index Stats: \n")
	fmt.Fprintf(builder, "Count: %d\n", receiver.ValueCount)
	fmt.Fprintf(builder, "Max ValuesPerLeave: %d\n", receiver.MaxValuesPerLeave)
	fmt.Fprintf(builder, "AVG ValuesPerLeave: %f\n", receiver.AVGValuesPerLeave)
	fmt.Fprintf(builder, "Leave Count: %d\n", receiver.LeaveCount)
	return builder.String()
}
