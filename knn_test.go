package go_sknn

import (
	"context"
	"math/rand"
	"strconv"
	"testing"

	"github.com/golang/geo/s2"
	"github.com/stretchr/testify/assert"
)

func RandLat(r *rand.Rand) float64 {
	return -90 + r.Float64()*180
}
func RandLong(r *rand.Rand) float64 {
	return -180 + r.Float64()*360
}

var intFilter = func(*Value[int]) bool {
	return false
}

func Test_NewKNN_Success(t *testing.T) {
	for i := range 31 {
		index, err := NewKNN[int](i)
		assert.Nil(t, err)
		assert.NotNil(t, index)
		r := rand.New(rand.NewSource(1))
		index.AddValue("1", 2, RandLat(r), RandLong(r))
		index.SearchApproximate(context.Background(), 0, 0, intFilter)
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

	assert.Len(t, index.indexRoot.values, 2)
	assert.Len(t, index.indexRoot.children, 0)
	assert.Len(t, index.lookup, 2)

	assert.Equal(t, 1, index.indexRoot.values[0].value)
	assert.Equal(t, 2, index.indexRoot.values[1].value)
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

	assert.Len(t, index.indexRoot.values, 2)
	assert.Len(t, index.indexRoot.children, 0)

	index.RemoveValue("1")
	assert.Len(t, index.indexRoot.values, 1)
	assert.Equal(t, 2, index.indexRoot.values[0].value)
	assert.Len(t, index.lookup, 1)

	index.RemoveValue("2")
	assert.Len(t, index.indexRoot.values, 0)
	assert.Len(t, index.lookup, 0)
}

func Test_KNN_SearchApproximate_Partial(t *testing.T) {
	objectCount := 2_000_000
	index, err := NewKNN[int](25)
	assert.NoError(t, err)
	r := rand.New(rand.NewSource(1))

	searchLat, searchLong := 51.44, 13.55
	searchLocation := s2.PointFromLatLng(s2.LatLngFromDegrees(searchLat, searchLong))

	for i := range objectCount {
		index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
	}

	var results []*Value[int]
	filter := func(current *Value[int]) bool {
		results = append(results, current)
		return len(results) >= 100
	}

	index.SearchApproximate(context.Background(), searchLat, searchLong, filter)
	prev := 0.0
	for i := range results {
		dist := float64(s2.CellFromCellID(results[i].cell).Distance(searchLocation))
		assert.True(t, prev <= dist, "prev: %f, dist: %f", prev, dist)
		prev = dist
	}
	assert.Len(t, results, 100)
}

func Test_KNN_SearchApproximate_Full(t *testing.T) {
	objectCount := 10_000
	index, err := NewKNN[int](30)
	assert.NoError(t, err)
	r := rand.New(rand.NewSource(1))

	searchLat, searchLong := 51.44, 13.55
	searchLocation := s2.PointFromLatLng(s2.LatLngFromDegrees(searchLat, searchLong))

	for i := range objectCount {
		index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
	}

	var results []*Value[int]
	filter := func(current *Value[int]) bool {
		results = append(results, current)
		return false
	}

	index.SearchApproximate(context.Background(), searchLat, searchLong, filter)
	prev := 0.0
	for i := range results {
		dist := float64(s2.CellFromCellID(results[i].cell).Distance(searchLocation))
		assert.True(t, prev <= dist, "prev: %f, dist: %f", prev, dist)
		prev = dist
	}
}

func Test_KNN_Search_Full(t *testing.T) {
	objectCount := 5_000_000
	index, err := NewKNN[int](13)
	assert.NoError(t, err)
	r := rand.New(rand.NewSource(1))

	searchLat, searchLong := 51.44, 13.55
	searchLocation := s2.PointFromLatLng(s2.LatLngFromDegrees(searchLat, searchLong))

	for i := range objectCount {
		index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
	}

	var results []*Value[int]
	filter := func(current *Value[int]) bool {
		results = append(results, current)
		return false
	}

	index.Search(context.Background(), searchLat, searchLong, filter)

	prev := 0.0
	for i := range results {
		dist := float64(s2.CellFromCellID(results[i].cell).Distance(searchLocation))
		assert.True(t, prev <= dist, "prev: %f, dist: %f", prev, dist)
		prev = dist
	}
}
