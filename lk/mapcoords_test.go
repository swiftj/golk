package lk

import (
	"math"
	"testing"
)

// ooo is a 1024×768 image: z = ceil(log2(1024/256)) = 2, so the frame is
// [[-768/4, 0], [0, 1024/4]] (the spec's [-y / 2^z, x / 2^z]).
var ooo = MapImage{Width: 1024, Height: 768, Bounds: [2][2]float64{{-192, 0}, {0, 256}}}

func TestMapCoords_C_MAP_01_Corners(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float64
		want [2]float64
	}{
		{"top-left is [north, west]", 0, 0, [2]float64{0, 0}},
		{"top-right is [north, east]", 1024, 0, [2]float64{0, 256}},
		{"bottom-left is [south, west]", 0, 768, [2]float64{-192, 0}},
		{"bottom-right is [south, east]", 1024, 768, [2]float64{-192, 256}},
		{"centre", 512, 384, [2]float64{-96, 128}},
		{"spec formula [-y/2^z, x/2^z]", 520, 420, [2]float64{-105, 130}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PixelToPos(ooo, tc.x, tc.y); got != tc.want {
				t.Errorf("PixelToPos(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.want)
			}
			if x, y := PosToPixel(ooo, tc.want); x != tc.x || y != tc.y {
				t.Errorf("PosToPixel(%v) = %v, %v, want %v, %v", tc.want, x, y, tc.x, tc.y)
			}
		})
	}
}

func TestMapCoords_C_MAP_01_RoundTrip(t *testing.T) {
	imgs := []MapImage{
		ooo,
		// Bounds not anchored at 0 still map linearly.
		{Width: 300, Height: 200, Bounds: [2][2]float64{{-7.5, 3}, {2.5, 18}}},
	}
	for _, img := range imgs {
		for _, p := range [][2]float64{{0, 0}, {1, 1}, {220, 170}, {830.25, 610.75}, {-10, 2000}} {
			pos := PixelToPos(img, p[0], p[1])
			x, y := PosToPixel(img, pos)
			if math.Abs(x-p[0]) > 1e-9 || math.Abs(y-p[1]) > 1e-9 {
				t.Errorf("%+v: %v -> %v -> (%v, %v)", img, p, pos, x, y)
			}
		}
	}
}

func TestMapCoords_C_MAP_01_DegenerateIsNaNFree(t *testing.T) {
	for _, img := range []MapImage{
		{},
		{Width: 0, Height: 10, Bounds: [2][2]float64{{-1, 0}, {0, 1}}},
		{Width: 10, Height: 0, Bounds: [2][2]float64{{-1, 0}, {0, 1}}},
		{Width: -5, Height: 10},
		{Width: 10, Height: 10}, // zero-extent bounds
	} {
		pos := PixelToPos(img, 3, 4)
		x, y := PosToPixel(img, [2]float64{-1, 1})
		for _, v := range []float64{pos[0], pos[1], x, y} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Errorf("%+v: got NaN or Inf: %v %v %v", img, pos, x, y)
			}
		}
		if img.Width <= 0 || img.Height <= 0 {
			if pos != [2]float64{} || x != 0 || y != 0 {
				t.Errorf("%+v: want zero values, got %v %v %v", img, pos, x, y)
			}
		}
	}
}
