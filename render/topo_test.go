// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render_test

import (
	"image/color"
	"testing"

	"github.com/maloquacious/wg/heightmap"
	"github.com/maloquacious/wg/render"
)

func TestTopoFlatMap(t *testing.T) {
	hm, err := heightmap.New(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	opts := render.DefaultTopoOptions()
	opts.OceanPercent = 0
	img := render.Topo(hm, opts)
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 4 {
		t.Fatalf("size: want 8x4, got %dx%d", b.Dx(), b.Dy())
	}
	// flat ground at elevation 0 is the lowest tint, evenly shaded, with no contours
	want := img.RGBAAt(0, 0)
	if want.G <= want.R || want.G <= want.B {
		t.Errorf("lowland: want green, got %v", want)
	}
	for y := range 4 {
		for x := range 8 {
			if got := img.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d, %d): want %v, got %v", x, y, want, got)
			}
		}
	}
}

func TestTopoDrawsContours(t *testing.T) {
	// a ramp from 0 to 1 across the map crosses every contour level
	hm, err := heightmap.New(100, 3)
	if err != nil {
		t.Fatal(err)
	}
	for y := range hm.Height {
		for x := range hm.Width {
			hm.Data[y*hm.Width+x] = float64(x) / float64(hm.Width-1)
		}
	}
	opts := render.DefaultTopoOptions()
	opts.Contours, opts.IndexEvery, opts.OceanPercent, opts.Smooth = 10, 5, 0, 0
	img := render.Topo(hm, opts)

	index := color.RGBA{R: 70, G: 40, B: 25, A: 255}
	var indexColumns int
	for x := range hm.Width {
		if img.RGBAAt(x, 1) == index {
			indexColumns++
		}
	}
	// levels 5 and 10 are index lines, each drawn 3 pixels wide
	if indexColumns != 6 {
		t.Errorf("index lines: want 6 columns, got %d", indexColumns)
	}
}

func TestTopoOcean(t *testing.T) {
	// a ramp from 0 to 1 across the map
	hm, err := heightmap.New(100, 3)
	if err != nil {
		t.Fatal(err)
	}
	for y := range hm.Height {
		for x := range hm.Width {
			hm.Data[y*hm.Width+x] = float64(x) / float64(hm.Width-1)
		}
	}
	for _, pct := range []int{0, 30, 70, 100} {
		opts := render.DefaultTopoOptions()
		opts.OceanPercent, opts.Contours, opts.Smooth = pct, 0, 0
		img := render.Topo(hm, opts)
		var water int
		for x := range hm.Width {
			if c := img.RGBAAt(x, 1); c.B > c.R && c.B > c.G {
				water++
			}
		}
		if water != pct {
			t.Errorf("%d%%: want %d water columns, got %d", pct, pct, water)
		}
	}
}
