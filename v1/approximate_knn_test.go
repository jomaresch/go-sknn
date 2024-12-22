package v1

import (
	"context"
	"fmt"
	"image/color"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/golang/geo/s1"
	"github.com/golang/geo/s2"
	"github.com/morikuni/go-geoplot"
	"github.com/stretchr/testify/assert"

	sm "github.com/flopp/go-staticmaps"
	"github.com/fogleman/gg"
)

var intFilter = func(ctx context.Context, _ float64, user int, result []int) FilterType {
	return FilterType_Continue
}

func RandLat(r *rand.Rand) float64 {
	return -90 + r.Float64()*180
}
func RandLong(r *rand.Rand) float64 {
	return -180 + r.Float64()*360
}

func Test_NewApproximateKNN_Success(t *testing.T) {
	for i := range 31 {
		index, err := NewApproximateKNNConcurrent[int](i)
		assert.Nil(t, err)
		assert.NotNil(t, index)
		r := rand.New(rand.NewSource(1))
		index.AddValue("1", 2, RandLat(r), RandLong(r))
		index.Search(context.Background(), 0, 0, intFilter)
	}
}

func Test_ApproximateKNNConcurrent_Error(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](-1)
	assert.EqualError(t, err, "invalid precision -1: precision must be between 0 and 30")
	assert.Nil(t, index)

	index, err = NewApproximateKNNConcurrent[int](31)
	assert.EqualError(t, err, "invalid precision 31: precision must be between 0 and 30")
	assert.Nil(t, index)

	index, err = NewApproximateKNNConcurrent[int](500)
	assert.EqualError(t, err, "invalid precision 500: precision must be between 0 and 30")
	assert.Nil(t, index)

	index, err = NewApproximateKNNConcurrent[int](-500)
	assert.EqualError(t, err, "invalid precision -500: precision must be between 0 and 30")
	assert.Nil(t, index)
}

func Test_ApproximateKNNConcurrent_AddAndRemove(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](5)
	assert.NoError(t, err)

	index.AddValue("1", 1, 0, 0)
	assert.Len(t, index.index.children, 1)
	assert.Len(t, index.lookup, 1)

	index.RemoveValue("1")
	assert.Len(t, index.index.children, 0)
	assert.Len(t, index.lookup, 0)
}

func Test_ApproximateKNNConcurrent_AddUser(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](10)
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

func Test_ApproximateKNNConcurrent_Search_ContextCancel(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](10)
	assert.Nil(t, err)
	assert.NotNil(t, index)
	index.AddValue("1", 2, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := index.Search(ctx, 0, 0, intFilter)
	assert.Nil(t, result)
}

func Test_ApproximateKNNConcurrent_Search_FilterSkip(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](10)
	assert.Nil(t, err)
	assert.NotNil(t, index)
	index.AddValue("1", 2, 0, 0)
	index.AddValue("2", 3, 1, 1)
	filter := func(ctx context.Context, _ float64, val int, result []int) FilterType {
		if val == 3 {
			return FilterType_Skip
		}
		return FilterType_Continue
	}
	result := index.Search(context.Background(), 0, 0, filter)
	assert.Equal(t, result, []int{2})
}

func Test_ApproximateKNNConcurrent_Search_FilterStop(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](10)
	assert.Nil(t, err)
	assert.NotNil(t, index)
	index.AddValue("1", 2, 0, 0)
	index.AddValue("2", 3, 1, 1)
	filter := func(ctx context.Context, _ float64, val int, result []int) FilterType {
		return FilterType_Stop
	}
	result := index.Search(context.Background(), 0, 0, filter)
	assert.Nil(t, result)
}

func Test_ApproximateKNNConcurrent(t *testing.T) {
	go func() {
		fmt.Println(http.ListenAndServe("localhost:9090", nil))
	}()
	r := rand.New(rand.NewSource(1))
	index, _ := NewApproximateKNNConcurrent[int](13)
	start := time.Now()
	for i := range 1000000 {
		index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
	}
	fmt.Println(time.Since(start))
	//fmt.Println(index.IndexStats())
	filter := func(ctx context.Context, _ float64, user int, result []int) FilterType {
		if len(result) >= 1000 {
			return FilterType_Stop
		}
		return FilterType_Continue
	}

	wg := &sync.WaitGroup{}
	for i := range 100 {
		lat, long := RandLat(r), RandLong(r)
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			index.Search(context.Background(), lat, long, filter)
			fmt.Println(i, " Took: ", time.Since(start))
		}()
	}
	wg.Wait()
}

func Test_ApproximateKNNConcurrent_ConcurrentAddAndSearch(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](10)
	assert.Nil(t, err)
	assert.NotNil(t, index)

	r := rand.New(rand.NewSource(1))
	wg := &sync.WaitGroup{}
	ctx := context.Background()

	// Add 2 million initial values
	for i := 0; i < 2000000; i++ {
		index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
	}

	// Function to add values concurrently
	addValues := func(start, end int) {
		defer wg.Done()
		for i := start; i < end; i++ {
			index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
		}
	}

	// Function to search values concurrently and measure timings
	searchValues := func(totalTime *time.Duration) {
		defer wg.Done()
		filter := func(ctx context.Context, _ float64, val int, result []int) FilterType {
			if len(result) >= 10000 {
				return FilterType_Stop
			}
			return FilterType_Continue
		}
		for i := 0; i < 100; i++ {
			start := time.Now()
			index.Search(ctx, RandLat(r), RandLong(r), filter)
			*totalTime += time.Since(start)
		}
	}

	// Start concurrent adding and searching
	wg.Add(4)
	go addValues(2000000, 2050000)
	go addValues(2050000, 2100000)

	var totalTime1, totalTime2 time.Duration
	go searchValues(&totalTime1)
	go searchValues(&totalTime2)

	wg.Wait()

	// Calculate and print the average search time
	avgTime := (totalTime1 + totalTime2) / 200
	fmt.Printf("Average search time: %s\n", avgTime)
}

func Show(users []*User, lat, lng float64) {
	dresden := &geoplot.LatLng{
		Latitude:  lat,
		Longitude: lng,
	}
	cellid := s2.CellIDFromLatLng(s2.LatLngFromDegrees(51.0504, 13.7373))
	cellid = cellid.Parent(7)
	cell := s2.CellFromCellID(cellid)
	icon := geoplot.ColorIcon(255, 255, 0)
	icon2 := geoplot.ColorIcon(255, 0, 0)

	m := &geoplot.Map{
		Center: dresden,
		Zoom:   7,
		Area: &geoplot.Area{
			From: dresden.Offset(-10, -10),
			To:   dresden.Offset(10, 10),
		},
	}
	fmt.Println(cell.RectBound().Lo())
	fmt.Println(cell.RectBound().Hi())
	fmt.Println([]*geoplot.LatLng{
		{Latitude: float64(cell.RectBound().Lo().Lat), Longitude: float64(cell.RectBound().Lo().Lng)},
		{Latitude: float64(cell.RectBound().Lo().Lat), Longitude: float64(cell.RectBound().Hi().Lng)},
		{Latitude: float64(cell.RectBound().Hi().Lat), Longitude: float64(cell.RectBound().Hi().Lng)},
		{Latitude: float64(cell.RectBound().Hi().Lat), Longitude: float64(cell.RectBound().Lo().Lng)},
	})

	m.AddPolyline(&geoplot.Polyline{
		LatLngs: []*geoplot.LatLng{
			{Latitude: float64(cell.RectBound().Lo().Lat), Longitude: float64(cell.RectBound().Lo().Lng)},
			{Latitude: float64(cell.RectBound().Lo().Lat), Longitude: float64(cell.RectBound().Hi().Lng)},
			{Latitude: float64(cell.RectBound().Hi().Lat), Longitude: float64(cell.RectBound().Hi().Lng)},
			{Latitude: float64(cell.RectBound().Hi().Lat), Longitude: float64(cell.RectBound().Lo().Lng)},
		},
		Popup: "sdjflks",
		Color: &color.RGBA{0xff, 0, 0, 0},
	})

	for _, user := range users {
		m.AddMarker(&geoplot.Marker{
			LatLng:  &geoplot.LatLng{Latitude: user.Lat, Longitude: user.Lon},
			Tooltip: "Hello",
			Icon:    icon,
		})
	}
	m.AddMarker(&geoplot.Marker{
		LatLng:  dresden,
		Tooltip: "Center",
		Icon:    icon2,
	})
	err := http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := geoplot.ServeMap(w, r, m)
		if err != nil {
			fmt.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	fmt.Println(err)
}

func Show2(users []*User, lat, lng float64) {
	ctx := sm.NewContext()
	ctx.SetSize(4000, 2000)
	ctx.SetZoom(5)
	ctx.SetCache(sm.NewTileCache("./cache", 777))
	for _, user := range users {
		ctx.AddObject(
			sm.NewMarker(
				s2.LatLngFromDegrees(user.Lat, user.Lon),
				color.RGBA{0xff, 0, 0, 0xff},
				16.0,
			),
		)
	}

	img, err := ctx.Render()
	if err != nil {
		panic(err)
	}

	if err := gg.SavePNG("my-map.png", img); err != nil {
		panic(err)
	}
}

func Test_ApproximateKNNConcurrent_AddAndSearchAndShow(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[*User](13)
	assert.Nil(t, err)
	assert.NotNil(t, index)

	r := rand.New(rand.NewSource(1))

	// Add 2 million users
	s := time.Now()
	for i := 0; i < 1_000_000; i++ {
		user := &User{
			Lat:    RandLat(r),
			Lon:    RandLong(r),
			Gender: "m",
		}
		index.AddValue(strconv.Itoa(i), user, user.Lat, user.Lon)
	}
	fmt.Println("Add values: ", time.Since(s))

	// Search for 5000 users
	maxDist := 0.0
	filter := func(ctx context.Context, dist float64, user *User, result []*User) FilterType {
		if dist > maxDist {
			maxDist = dist
		}
		if len(result) >= 20_000 {
			return FilterType_Stop
		}
		return FilterType_Continue
	}
	//results := index.Search(context.Background(), RandLat(r), RandLong(r), filter)s
	start := time.Now()
	results := index.Search(context.Background(), 13.203269, 0.212961, filter)
	const earthRadiusKm = 6371.01
	fmt.Println("took", time.Since(start), "maxDist", maxDist*earthRadiusKm, "a distance", s1.ChordAngle(maxDist).Angle()*earthRadiusKm)
	// Show the 5000 users
	Show2(results, 13.203269, 0.212961)
}

type User struct {
	Lat    float64 `json:"current_loc_lat"`
	Lon    float64 `json:"current_loc_lon"`
	Gender string  `json:"gender"`
}

func Test_ApproximateKNNConcurrent_AvgSearchTime(t *testing.T) {
	index, err := NewApproximateKNNConcurrent[int](10)
	assert.Nil(t, err)
	assert.NotNil(t, index)

	r := rand.New(rand.NewSource(1))

	start1 := time.Now()
	// Add 2 million values
	for i := 0; i < 2000000; i++ {
		index.AddValue(strconv.Itoa(i), i, RandLat(r), RandLong(r))
	}
	fmt.Println("Add values: ", time.Since(start1))

	// Define the filter function to stop after finding 10,000 values
	filter := func(ctx context.Context, _ float64, val int, result []int) FilterType {
		if len(result) >= 100_000 {
			return FilterType_Stop
		}
		return FilterType_Continue
	}

	// Measure the average search time
	var totalTime time.Duration
	for i := 0; i < 100; i++ {
		start := time.Now()
		index.Search(context.Background(), RandLat(r), RandLong(r), filter)
		totalTime += time.Since(start)
	}

	avgTime := totalTime / 100
	fmt.Printf("Average search time: %s\n", avgTime)
}
