package v1

type FilterType int

const (
	FilterType_Continue FilterType = 0
	FilterType_Skip     FilterType = 1
	FilterType_Stop     FilterType = 2
)
