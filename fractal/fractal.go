// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package fractal builds a height map with the diamond-square algorithm. It
// is an alternative to fracture for the first stage of the map pipeline, and
// gives smooth, continuous terrain instead of whole steps.
//
// The algorithm is adapted from the "fractal" generator in
// https://github.com/mdhender/mapgen, which follows Paul Martz's "Generating
// Random Fractal Terrain". It fills a square grid of 2ⁿ+1 points whose edges
// wrap, so the grid would tile seamlessly. The map is cropped from its top
// left corner, so the map itself does not tile.
package fractal

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/maloquacious/wg/heightmap"
)

// Options configures the generator.
type Options struct {
	Width, Height int // size of the map in pixels
	// Roughness is Martz's H. The random offsets shrink by a factor of 2^-H
	// each time the grid spacing halves, so 0 keeps fine detail as strong as
	// the continents (close to noise) and larger values give smoother land.
	Roughness float64
	Seed      uint64 // seed for the random number generator
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{
		Width:     1920,
		Height:    1080,
		Roughness: 0.8,
		Seed:      0x0123456789abcdef,
	}
}

// Generate returns a height map normalized to the range 0...1.
// The same options always produce the same map.
func Generate(opts Options) (*heightmap.Map, error) {
	if opts.Width < 2 || opts.Height < 2 {
		return nil, fmt.Errorf("fractal: invalid size %dx%d", opts.Width, opts.Height)
	}
	if !(opts.Roughness >= 0) || math.IsInf(opts.Roughness, 0) {
		return nil, fmt.Errorf("fractal: invalid roughness %g", opts.Roughness)
	}
	rnd := rand.New(rand.NewPCG(opts.Seed, opts.Seed))

	// side is the number of segments along the grid's edge, a power of 2.
	side := 1
	for side+1 < max(opts.Width, opts.Height) {
		side *= 2
	}
	grid := diamondSquare(side, math.Pow(2, -opts.Roughness), rnd)

	hm, err := heightmap.New(opts.Width, opts.Height)
	if err != nil {
		return nil, err
	}
	for y := range opts.Height {
		copy(hm.Data[y*opts.Width:(y+1)*opts.Width], grid[y*(side+1):])
	}
	normalize(hm.Data)
	return hm, nil
}

// diamondSquare returns a (side+1)² grid in row-major order. side must be a
// power of 2. Each pass halves the spacing and multiplies the size of the
// random offsets by ratio.
func diamondSquare(side int, ratio float64, rnd *rand.Rand) []float64 {
	size := side + 1
	g := make([]float64, size*size)
	at := func(x, y int) *float64 { return &g[y*size+x] }
	offset := func(scale float64) float64 { return scale * (rnd.Float64() - 0.5) }

	// the four corners share one value, so the grid tiles
	corner := 2*rnd.Float64() - 1
	*at(0, 0), *at(side, 0), *at(0, side), *at(side, side) = corner, corner, corner, corner

	scale := ratio
	for stride := side / 2; stride > 0; stride /= 2 {
		// diamond step: the center of each square is the mean of its corners
		for y := stride; y < side; y += 2 * stride {
			for x := stride; x < side; x += 2 * stride {
				mean := (*at(x-stride, y-stride) + *at(x+stride, y-stride) +
					*at(x-stride, y+stride) + *at(x+stride, y+stride)) / 4
				*at(x, y) = mean + offset(scale)
			}
		}
		// square step: the center of each diamond is the mean of its points,
		// wrapping at the edges; the last row and column copy the first
		for y := 0; y < side; y += stride {
			for x := (y/stride + 1) % 2 * stride; x < side; x += 2 * stride {
				mean := (*at((x-stride+side)%side, y) + *at(x+stride, y) +
					*at(x, (y-stride+side)%side) + *at(x, y+stride)) / 4
				*at(x, y) = mean + offset(scale)
				if x == 0 {
					*at(side, y) = *at(x, y)
				}
				if y == 0 {
					*at(x, side) = *at(x, y)
				}
			}
		}
		scale *= ratio
	}
	return g
}

// normalize scales data in place so that the lowest point is 0 and the
// highest is 1. A perfectly flat map is set to 0.
func normalize(data []float64) {
	lo, hi := data[0], data[0]
	for _, e := range data {
		lo, hi = min(lo, e), max(hi, e)
	}
	if lo == hi {
		clear(data)
		return
	}
	for n, e := range data {
		data[n] = (e - lo) / (hi - lo)
	}
}
