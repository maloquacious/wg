// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package fracture implements the first stage of the map pipeline. It builds
// a height map by repeatedly raising or lowering the terrain inside random
// circles, which leaves fault lines where the circles overlap.
//
// The algorithm is adapted from the "flat" generator in
// https://github.com/mdhender/mapgen.
package fracture

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/maloquacious/wg/heightmap"
)

// Options configures the generator.
type Options struct {
	Width, Height int    // size of the map in pixels
	Rounds        int    // number of circles to raise or lower
	Seed          uint64 // seed for the random number generator
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{
		Width:  1920,
		Height: 1080,
		Rounds: 1_000,
		Seed:   0x0123456789abcdef,
	}
}

// Generate returns a height map normalized to the range 0...1.
// The same options always produce the same map.
func Generate(opts Options) (*heightmap.Map, error) {
	if opts.Width < 2 || opts.Height < 2 {
		return nil, fmt.Errorf("fracture: invalid size %dx%d", opts.Width, opts.Height)
	}
	if opts.Rounds < 1 {
		return nil, fmt.Errorf("fracture: invalid rounds %d", opts.Rounds)
	}
	rnd := rand.New(rand.NewPCG(opts.Seed, opts.Seed))
	width, height := opts.Width, opts.Height

	// accumulate the raw elevations as integers
	raw := make([]int32, width*height)
	maxR := min(width, height) / 2

	for range opts.Rounds {
		// decide whether to raise or lower the circle
		bump := int32(1)
		if rnd.IntN(2) == 1 {
			bump = -1
		}
		radius := rnd.IntN(maxR) + 1
		cx, cy := rnd.IntN(width), rnd.IntN(height)

		// bump every pixel strictly inside the circle, one row at a time
		rSquared := radius * radius
		for y := max(cy-radius+1, 0); y < min(cy+radius, height); y++ {
			dy := y - cy
			// dx is the widest offset with dx*dx + dy*dy < rSquared
			dx := isqrt(rSquared - dy*dy - 1)
			row := raw[y*width : (y+1)*width]
			for x := max(cx-dx, 0); x <= min(cx+dx, width-1); x++ {
				row[x] += bump
			}
		}
	}

	hm, err := heightmap.New(width, height)
	if err != nil {
		return nil, err
	}
	normalize(raw, hm.Data)
	return hm, nil
}

// normalize scales src into dst so that the lowest point is 0 and the
// highest is 1. A perfectly flat map is set to 0.
func normalize(src []int32, dst []float64) {
	lo, hi := src[0], src[0]
	for _, e := range src {
		lo, hi = min(lo, e), max(hi, e)
	}
	if lo == hi {
		clear(dst)
		return
	}
	scale := 1 / float64(hi-lo)
	for n, e := range src {
		dst[n] = float64(e-lo) * scale
	}
}

// isqrt returns the largest integer whose square is at most n.
func isqrt(n int) int {
	r := int(math.Sqrt(float64(n)))
	for r*r > n {
		r--
	}
	for (r+1)*(r+1) <= n {
		r++
	}
	return r
}
