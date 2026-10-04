// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package stitch_test

import (
	"flag"
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/maloquacious/wg/heightmap"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/stitch"
)

// renderDir enables writing the test maps as images, for example
//
//	go test ./stitch -render=/tmp/maps
var renderDir = flag.String("render", "", "write rendered maps to this directory")

func TestDefaultOptions(t *testing.T) {
	opts := stitch.DefaultOptions()
	if w, h := opts.Size(); w != 2049 || h != 1025 {
		t.Errorf("size: want 2049x1025, got %dx%d", w, h)
	}
	if opts.Seed != 0x0123456789abcdef {
		t.Errorf("seed: want 0x0123456789abcdef, got %#x", opts.Seed)
	}
}

func TestGenerate(t *testing.T) {
	opts := stitch.DefaultOptions()
	hm, err := stitch.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	w, h := opts.Size()
	if hm.Width != w || hm.Height != h || len(hm.Data) != w*h {
		t.Fatalf("size: want %dx%d, got %dx%d with %d pixels", w, h, hm.Width, hm.Height, len(hm.Data))
	}
	if lo, hi := slices.Min(hm.Data), slices.Max(hm.Data); lo != 0 || hi != 1 {
		t.Errorf("range: want 0...1, got %g...%g", lo, hi)
	}
	if *renderDir != "" {
		path := filepath.Join(*renderDir, "stitch-topo.png")
		if err := render.WritePNG(path, render.Topo(hm, render.DefaultTopoOptions())); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}

	again, err := stitch.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hm.Data, again.Data) {
		t.Errorf("same options produced different maps")
	}

	opts.Seed++
	other, err := stitch.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(hm.Data, other.Data) {
		t.Errorf("different seeds produced the same map")
	}
}

// TestNoSeams checks that the land is no steeper across the edges between
// blocks than across the middle of a block. Diamond-square leaves a crease
// along every coarse grid line, so the middle line is the fair comparison.
func TestNoSeams(t *testing.T) {
	opts := stitch.Options{BlocksWide: 4, BlocksHigh: 4, BlockSize: 65, Roughness: 0.8}
	side := opts.BlockSize - 1
	var seams, middles float64
	for seed := range uint64(50) {
		opts.Seed = seed
		hm, err := stitch.Generate(opts)
		if err != nil {
			t.Fatal(err)
		}
		for b := 1; b < opts.BlocksWide; b++ {
			seams += step(hm, b*side) + stepY(hm, b*side)
		}
		for b := 0; b < opts.BlocksWide-1; b++ {
			middles += step(hm, b*side+side/2) + stepY(hm, b*side+side/2)
		}
	}
	if ratio := seams / middles; ratio > 1.25 {
		t.Errorf("block edges are %.2f times as steep as block middles", ratio)
	} else {
		t.Logf("block edges are %.2f times as steep as block middles", ratio)
	}
}

// step returns the mean absolute change in height from each pixel in column
// x−1 to the pixel two columns over, across column x.
func step(hm *heightmap.Map, x int) float64 {
	var sum float64
	for y := range hm.Height {
		sum += math.Abs(hm.At(x+1, y) - hm.At(x-1, y))
	}
	return sum / float64(hm.Height)
}

// stepY is step across row y.
func stepY(hm *heightmap.Map, y int) float64 {
	var sum float64
	for x := range hm.Width {
		sum += math.Abs(hm.At(x, y+1) - hm.At(x, y-1))
	}
	return sum / float64(hm.Width)
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	for _, opts := range []stitch.Options{
		{BlocksWide: 0, BlocksHigh: 1, BlockSize: 1025, Roughness: 1},
		{BlocksWide: 1, BlocksHigh: -1, BlockSize: 1025, Roughness: 1},
		{BlocksWide: 1, BlocksHigh: 1, BlockSize: 1024, Roughness: 1},
		{BlocksWide: 1, BlocksHigh: 1, BlockSize: 2, Roughness: 1},
		{BlocksWide: 1, BlocksHigh: 1, BlockSize: 0, Roughness: 1},
		{BlocksWide: 1, BlocksHigh: 1, BlockSize: 1025, Roughness: -1},
		{BlocksWide: 1, BlocksHigh: 1, BlockSize: 1025, Roughness: math.NaN()},
		{BlocksWide: 1000, BlocksHigh: 1000, BlockSize: 1025, Roughness: 1},
	} {
		if _, err := stitch.Generate(opts); err == nil {
			t.Errorf("%+v: want error, got nil", opts)
		}
	}
}
