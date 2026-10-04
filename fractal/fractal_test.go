// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fractal_test

import (
	"flag"
	"math"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"

	"github.com/maloquacious/wg/fractal"
	"github.com/maloquacious/wg/render"
)

// renderDir enables writing the test maps as images, for example
//
//	go test ./fractal -render=/tmp/maps
var renderDir = flag.String("render", "", "write rendered maps to this directory")

func TestDefaultOptions(t *testing.T) {
	opts := fractal.DefaultOptions()
	if opts.Width != 1025 || opts.Height != 1025 {
		t.Errorf("size: want 1025x1025, got %dx%d", opts.Width, opts.Height)
	}
	if opts.Seed != 0x0123456789abcdef {
		t.Errorf("seed: want 0x0123456789abcdef, got %#x", opts.Seed)
	}
	if opts.Roughness <= 0 {
		t.Errorf("roughness: want > 0, got %g", opts.Roughness)
	}
}

func TestGenerate(t *testing.T) {
	opts := fractal.DefaultOptions()
	hm, err := fractal.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if hm.Width != opts.Width || hm.Height != opts.Height || len(hm.Data) != opts.Width*opts.Height {
		t.Fatalf("size: want %dx%d, got %dx%d with %d pixels", opts.Width, opts.Height, hm.Width, hm.Height, len(hm.Data))
	}
	if lo, hi := slices.Min(hm.Data), slices.Max(hm.Data); lo != 0 || hi != 1 {
		t.Errorf("range: want 0...1, got %g...%g", lo, hi)
	}
	if *renderDir != "" {
		path := filepath.Join(*renderDir, "fractal-topo.png")
		if err := render.WritePNG(path, render.Topo(hm, render.DefaultTopoOptions())); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}

	again, err := fractal.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hm.Data, again.Data) {
		t.Errorf("same options produced different maps")
	}

	opts.Seed++
	other, err := fractal.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(hm.Data, other.Data) {
		t.Errorf("different seeds produced the same map")
	}
}

// TestMatchesReference checks the generator against a direct port of the
// original's fill, normalized the same way.
func TestMatchesReference(t *testing.T) {
	for _, opts := range []fractal.Options{
		{Width: 129, Height: 129, Roughness: 0.8, Seed: 0x0123456789abcdef},
		{Width: 33, Height: 33, Roughness: 0.001, Seed: 7},
		{Width: 3, Height: 3, Roughness: 1, Seed: 1},
	} {
		hm, err := fractal.Generate(opts)
		if err != nil {
			t.Fatal(err)
		}
		want := reference(opts)
		for n := range want {
			if math.Abs(hm.Data[n]-want[n]) > 1e-12 {
				t.Fatalf("%+v: pixel (%d, %d): want %g, got %g", opts, n%opts.Width, n/opts.Width, want[n], hm.Data[n])
			}
		}
	}
}

// reference is mapgen's grid.fill, with its first index as y.
func reference(opts fractal.Options) []float64 {
	rnd := rand.New(rand.NewPCG(opts.Seed, opts.Seed))
	randnum := func(lo, hi float64) float64 { return rnd.Float64()*(hi-lo) + lo }
	size := opts.Width
	subSize := size - 1
	fa := make([]float64, size*size)
	avgDiamond := func(i, j, stride int) float64 {
		switch {
		case i == 0:
			return (fa[i*size+j-stride] + fa[i*size+j+stride] + fa[(subSize-stride)*size+j] + fa[(i+stride)*size+j]) * 0.25
		case j == 0:
			return (fa[(i-stride)*size+j] + fa[(i+stride)*size+j] + fa[i*size+j+stride] + fa[i*size+subSize-stride]) * 0.25
		}
		return (fa[(i-stride)*size+j] + fa[(i+stride)*size+j] + fa[i*size+j-stride] + fa[i*size+j+stride]) * 0.25
	}

	ratio := math.Pow(2, -opts.Roughness)
	scale := ratio
	fa[0] = randnum(-1, 1)
	fa[subSize*size], fa[subSize*size+subSize], fa[subSize] = fa[0], fa[0], fa[0]
	for stride := subSize / 2; stride != 0; stride /= 2 {
		for i := stride; i < subSize; i += 2 * stride {
			for j := stride; j < subSize; j += 2 * stride {
				avg := (fa[(i-stride)*size+j-stride] + fa[(i-stride)*size+j+stride] + fa[(i+stride)*size+j-stride] + fa[(i+stride)*size+j+stride]) * 0.25
				fa[i*size+j] = scale*randnum(-0.5, 0.5) + avg
			}
		}
		oddline := false
		for i := 0; i < subSize; i += stride {
			oddline = !oddline
			for j := 0; j < subSize; j += stride {
				if oddline && j == 0 {
					j += stride
				}
				fa[i*size+j] = scale*randnum(-0.5, 0.5) + avgDiamond(i, j, stride)
				if i == 0 {
					fa[subSize*size+j] = fa[i*size+j]
				}
				if j == 0 {
					fa[i*size+subSize] = fa[i*size+j]
				}
				j += stride
			}
		}
		scale *= ratio
	}

	out := fa
	lo, hi := slices.Min(out), slices.Max(out)
	for n := range out {
		out[n] = (out[n] - lo) / (hi - lo)
	}
	return out
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	for _, opts := range []fractal.Options{
		{Width: 1920, Height: 1080, Roughness: 1},
		{Width: 1024, Height: 1024, Roughness: 1},
		{Width: 1025, Height: 513, Roughness: 1},
		{Width: 2, Height: 2, Roughness: 1},
		{Width: 1, Height: 1, Roughness: 1},
		{Width: 0, Height: 0, Roughness: 1},
		{Width: -3, Height: -3, Roughness: 1},
		{Width: 1025, Height: 1025, Roughness: -1},
		{Width: 1025, Height: 1025, Roughness: math.NaN()},
		{Width: 1025, Height: 1025, Roughness: math.Inf(1)},
	} {
		if _, err := fractal.Generate(opts); err == nil {
			t.Errorf("%+v: want error, got nil", opts)
		}
	}
}
