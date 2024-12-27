package go_sknn

import (
	"github.com/golang/geo/s2"
)

const earthRadiusKm = 6371.01

type Value[T any] struct {
	key   string
	value T
	cell  s2.CellID
}

func (v *Value[T]) Value() T {
	return v.value
}

func (v *Value[T]) Key() string {
	return v.key
}

func (v *Value[T]) DistanceKM(lat, long float64) float64 {
	return float64(s2.LatLngFromDegrees(lat, long).Distance(v.cell.LatLng())) * earthRadiusKm
}
