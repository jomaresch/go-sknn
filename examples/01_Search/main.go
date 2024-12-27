package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"

	go_sknn "go-sknn"
)

func main() {
	// Create a new KNN index with a precision of 14.
	// 14 is a good value for most use-cases.
	// The average cell size is 0.32 km².
	// http://s2geometry.io/resources/s2cell_statistics.html
	index, err := go_sknn.NewKNN[int](14)
	if err != nil {
		log.Fatalln("Error creating index:", err)
	}
	r := rand.New(rand.NewSource(1))

	// Add 2 Mio random "user-ids" to the index.
	for i := range 2_000_000 {
		index.AddValue(fmt.Sprintf("user-%d", i), i, RandLat(r), RandLong(r))
	}

	// Run the approximate search for 10 users.
	fmt.Println("Search:")
	Search(index, 51.0504, 13.7373)
	fmt.Println("\nApproximate search:")
	SearchApproximate(index, 51.0504, 13.7373)

	// This highlights the difference between the search and the approximate search.
	// While the search is exact, the approximate search is faster but not as accurate.
	// The difference can be seen in the ordering of the results at position 8 and 9.

	// Output:
	//Search:
	//0 User: user-1828677, Distance: 6.19 km
	//1 User: user-1459638, Distance: 9.47 km
	//2 User: user-914041, Distance: 13.10 km
	//3 User: user-1291899, Distance: 15.58 km
	//4 User: user-621903, Distance: 15.62 km
	//5 User: user-438482, Distance: 24.90 km
	//6 User: user-77947, Distance: 25.01 km
	//7 User: user-115390, Distance: 27.43 km
	//8 User: user-1501847, Distance: 28.91 km
	//9 User: user-626679, Distance: 28.96 km
	//
	//Approximate search:
	//0 User: user-1828677, Distance: 6.19 km
	//1 User: user-1459638, Distance: 9.47 km
	//2 User: user-914041, Distance: 13.10 km
	//3 User: user-1291899, Distance: 15.58 km
	//4 User: user-621903, Distance: 15.62 km
	//5 User: user-438482, Distance: 24.90 km
	//6 User: user-77947, Distance: 25.01 km
	//7 User: user-115390, Distance: 27.43 km
	//8 User: user-626679, Distance: 28.96 km
	//9 User: user-1501847, Distance: 28.91 km
}

func SearchApproximate(index *go_sknn.KNN[int], lat, long float64) {
	result := make([]*go_sknn.Value[int], 0, 10)
	searchFunc := func(value *go_sknn.Value[int]) bool {
		result = append(result, value)
		return len(result) >= 10
	}
	index.SearchApproximate(context.Background(), lat, long, searchFunc)
	for i, value := range result {
		fmt.Printf("%d User: %s, Distance: %.2f km\n", i, value.Key(), value.DistanceKM(lat, long))
	}
}

func Search(index *go_sknn.KNN[int], lat, long float64) {
	result := make([]*go_sknn.Value[int], 0, 10)
	searchFunc := func(value *go_sknn.Value[int]) bool {
		result = append(result, value)
		return len(result) >= 10
	}
	index.Search(context.Background(), lat, long, searchFunc)
	for i, value := range result {
		fmt.Printf("%d User: %s, Distance: %.2f km\n", i, value.Key(), value.DistanceKM(lat, long))
	}
}

func RandLat(r *rand.Rand) float64 {
	return -90 + r.Float64()*180
}
func RandLong(r *rand.Rand) float64 {
	return -180 + r.Float64()*360
}
