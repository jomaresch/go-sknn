package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"math/rand"

	go_sknn "go-sknn"

	sm "github.com/flopp/go-staticmaps"
	"github.com/fogleman/gg"
	"github.com/golang/geo/s2"
)

func main() {
	index, err := go_sknn.NewKNN[int](14)
	if err != nil {
		log.Fatalln("Error creating index:", err)
	}
	r := rand.New(rand.NewSource(1))

	for i := range 500_000 {
		index.AddValue(fmt.Sprintf("user-%d", i), i, RandLat(r), RandLong(r))
	}

	result := make([]*go_sknn.Value[int], 0, 400)
	searchFunc := func(value *go_sknn.Value[int]) bool {
		result = append(result, value)
		return len(result) >= 400
	}
	index.Search(context.Background(), 51.0504, 13.7373, searchFunc)
	ctx := sm.NewContext()
	ctx.SetSize(1920, 1080)

	ctx.AddObject(sm.NewMarker(s2.LatLngFromDegrees(51.0504, 13.7373), color.RGBA{0xff, 0, 0, 0xff}, 16.0))

	for _, value := range result {
		ctx.AddObject(sm.NewMarker(value.CellID().LatLng(), color.RGBA{0, 0, 0xff, 0xff}, 16.0))
	}

	img, err := ctx.Render()
	if err != nil {
		panic(err)
	}

	if err := gg.SavePNG("map.png", img); err != nil {
		panic(err)
	}
}

func RandLat(r *rand.Rand) float64 {
	return -90 + r.Float64()*180
}
func RandLong(r *rand.Rand) float64 {
	return -180 + r.Float64()*360
}
