package v2

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/golang/geo/s2"
	"github.com/stretchr/testify/assert"
)

func RandLat(r *rand.Rand) float64 {
	return -90 + r.Float64()*180
}
func RandLong(r *rand.Rand) float64 {
	return -180 + r.Float64()*360
}

var intFilter = func(ctx context.Context, _ float64, user int, result []int) FilterType {
	return FilterType_Continue
}

func Test_NewKNN_Success(t *testing.T) {
	for i := range 31 {
		index, err := NewKNN[int](i)
		assert.Nil(t, err)
		assert.NotNil(t, index)
		r := rand.New(rand.NewSource(1))
		index.AddValue("1", 2, RandLat(r), RandLong(r))
		index.Search(context.Background(), 0, 0, intFilter)
	}
}

func Test_NewKNN_Error(t *testing.T) {
	index, err := NewKNN[int](-1)
	assert.EqualError(t, err, "invalid precision -1: precision must be between 0 and 30")
	assert.Nil(t, index)

	index, err = NewKNN[int](31)
	assert.EqualError(t, err, "invalid precision 31: precision must be between 0 and 30")
	assert.Nil(t, index)

	index, err = NewKNN[int](500)
	assert.EqualError(t, err, "invalid precision 500: precision must be between 0 and 30")
	assert.Nil(t, index)

	index, err = NewKNN[int](-500)
	assert.EqualError(t, err, "invalid precision -500: precision must be between 0 and 30")
	assert.Nil(t, index)
}

func Test_KNN_AddValue(t *testing.T) {
	index, err := NewKNN[int](5)
	assert.NoError(t, err)

	index.AddValue("1", 1, 1, 1)
	index.AddValue("2", 2, 1.001, 1.001)

	assert.Len(t, index.index.children, 1)
	assert.Len(t, index.lookup, 2)

	node := index.index
	cellID := s2.CellIDFromLatLng(s2.LatLngFromDegrees(1, 1))
	for i := range 5 {
		cell := cellID.Parent(i)
		node = node.children[0]
		assert.Equal(t, node.cellID, cell)
	}

	assert.Equal(t, 1, node.values["1"])
	assert.Equal(t, 2, node.values["2"])
}

func Test_KNN_AddValue_Panic(t *testing.T) {
	index, err := NewKNN[int](10)
	assert.Nil(t, err)
	assert.NotNil(t, index)

	assert.PanicsWithValue(t,
		"invalid latitude 0.000000 (Min:-90, Max 90) or longitude 181.000000 (Min: -180, Max 180)",
		func() { index.AddValue("1", 2, 0, 181) },
	)
	assert.PanicsWithValue(t,
		"invalid latitude 0.000000 (Min:-90, Max 90) or longitude -181.000000 (Min: -180, Max 180)",
		func() { index.AddValue("1", 2, 0, -181) },
	)
	assert.PanicsWithValue(t,
		"invalid latitude 91.000000 (Min:-90, Max 90) or longitude 0.000000 (Min: -180, Max 180)",
		func() { index.AddValue("1", 2, 91, 0) },
	)
	assert.PanicsWithValue(t,
		"invalid latitude -91.000000 (Min:-90, Max 90) or longitude 0.000000 (Min: -180, Max 180)",
		func() { index.AddValue("1", 2, -91, 0) },
	)

	index.AddValue("1", 2, -90, 0)
	index.AddValue("2", 2, 90, 0)
	index.AddValue("3", 2, 0, 180)
	index.AddValue("4", 2, 0, -180)
}

func Test_KNN_RemoveValue(t *testing.T) {
	index, err := NewKNN[int](5)
	assert.NoError(t, err)

	index.AddValue("1", 1, 1, 1)
	index.AddValue("2", 2, 1.001, 1.001)

	node := index.index
	cellID := s2.CellIDFromLatLng(s2.LatLngFromDegrees(1, 1))

	index.RemoveValue("1")
	assert.Equal(t, 0, node.values["1"])
	assert.Equal(t, 2, node.values["2"])
	assert.Len(t, index.lookup, 1)

	index.RemoveValue("2")
	assert.Equal(t, 0, node.values["2"])
	assert.Equal(t, 0, node.values["1"])
	assert.Len(t, index.lookup, 0)

	node = index.index
	cellID = s2.CellIDFromLatLng(s2.LatLngFromDegrees(1, 1))
	for i := range 5 {
		cell := cellID.Parent(i)
		node = node.children[0]
		assert.Equal(t, node.cellID, cell)
	}
}

func Test_KNN_Prune(t *testing.T) {
	index, err := NewKNN[int](5)
	assert.NoError(t, err)

	index.AddValue("1", 1, 1, 1)
	index.RemoveValue("1")

	index.Prune()
	assert.Len(t, index.index.children, 0)
}

func Test_KNN_Search(t *testing.T) {
	objectCount := 5000000
	index, err := NewKNN[int](15)
	assert.NoError(t, err)
	r := rand.New(rand.NewSource(1))
	type object struct {
		id   int
		dist float64
	}

	searchLat, searchLong := 51.44, 13.55
	searchLocation := s2.PointFromLatLng(s2.LatLngFromDegrees(searchLat, searchLong))
	values := make([]*object, objectCount)

	for i := range objectCount {
		lat, long := RandLat(r), RandLong(r)
		dist := searchLocation.Distance(s2.PointFromLatLng(s2.LatLngFromDegrees(lat, long)))
		values[i] = &object{id: i, dist: float64(dist)}
		index.AddValue(strconv.Itoa(i), i, lat, long)
	}

	sortFunc := func(a, b *object) int {
		if a.dist > b.dist {
			return 1
		}
		return -1
	}
	start1 := time.Now()
	slices.SortFunc(values, sortFunc)
	fmt.Println("Sort took", time.Since(start1))

	filter := func(ctx context.Context, dist float64, current int, result []int) FilterType {
		if len(result) >= 100 {
			return FilterType_Stop
		}
		return FilterType_Continue
	}

	start2 := time.Now()
	result := index.Search(context.Background(), searchLat, searchLong, filter)
	fmt.Println("Search took", time.Since(start2))
	for i := range result {
		assert.Equal(t, values[i].id, result[i])
		fmt.Println(result[i], values[i])
	}
}
