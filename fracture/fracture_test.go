// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fracture_test

import (
	"flag"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"

	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/render"
)

// renderDir enables writing the test maps as images, for example
//
//	go test ./fracture -render=/tmp/maps
var renderDir = flag.String("render", "", "write rendered maps to this directory")

func TestDefaultOptions(t *testing.T) {
	opts := fracture.DefaultOptions()
	if opts.Width != 1920 || opts.Height != 1080 {
		t.Errorf("size: want 1920x1080, got %dx%d", opts.Width, opts.Height)
	}
	if opts.Seed != 0x0123456789abcdef {
		t.Errorf("seed: want 0x0123456789abcdef, got %#x", opts.Seed)
	}
	if opts.Rounds < 1 {
		t.Errorf("rounds: want > 0, got %d", opts.Rounds)
	}
}

func TestGenerate(t *testing.T) {
	opts := fracture.DefaultOptions()
	hm, err := fracture.Generate(opts)
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
		path := filepath.Join(*renderDir, "fracture-topo.png")
		if err := render.WritePNG(path, render.Topo(hm, render.DefaultTopoOptions())); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}

	again, err := fracture.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hm.Data, again.Data) {
		t.Errorf("same options produced different maps")
	}

	opts.Seed++
	other, err := fracture.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(hm.Data, other.Data) {
		t.Errorf("different seeds produced the same map")
	}
}

// TestMatchesReference checks the row-span fill against the per-pixel circle
// test used by the original generator.
func TestMatchesReference(t *testing.T) {
	opts := fracture.Options{Width: 97, Height: 61, Rounds: 300, Seed: 0x0123456789abcdef}
	hm, err := fracture.Generate(opts)
	if err != nil {
		t.Fatal(err)
	}
	want := reference(opts)
	for n := range want {
		if hm.Data[n] != want[n] {
			t.Fatalf("pixel (%d, %d): want %g, got %g", n%opts.Width, n/opts.Width, want[n], hm.Data[n])
		}
	}
}

func reference(opts fracture.Options) []float64 {
	rnd := rand.New(rand.NewPCG(opts.Seed, opts.Seed))
	raw := make([]int, opts.Width*opts.Height)
	maxR := min(opts.Width, opts.Height) / 2
	for range opts.Rounds {
		bump := 1
		if rnd.IntN(2) == 1 {
			bump = -1
		}
		radius := rnd.IntN(maxR) + 1
		cx, cy := rnd.IntN(opts.Width), rnd.IntN(opts.Height)
		for y := range opts.Height {
			for x := range opts.Width {
				if dx, dy := x-cx, y-cy; dx*dx+dy*dy < radius*radius {
					raw[y*opts.Width+x] += bump
				}
			}
		}
	}
	lo, hi := slices.Min(raw), slices.Max(raw)
	out := make([]float64, len(raw))
	for n, e := range raw {
		out[n] = float64(e-lo) * (1 / float64(hi-lo))
	}
	return out
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	for _, opts := range []fracture.Options{
		{Width: 0, Height: 1080, Rounds: 1},
		{Width: 1920, Height: 1, Rounds: 1},
		{Width: 1920, Height: 1080, Rounds: 0},
	} {
		if _, err := fracture.Generate(opts); err == nil {
			t.Errorf("%+v: want error, got nil", opts)
		}
	}
}
