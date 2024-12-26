package v3

import (
	"github.com/golang/geo/s2"
)

type Value[T any] struct {
	key   string
	value T
	cell  s2.CellID
}
