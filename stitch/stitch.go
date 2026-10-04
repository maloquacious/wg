// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package stitch builds a height map with the diamond-square algorithm over
// a row-and-column layout of square blocks, so that, unlike fractal, the map
// need not be square.
//
// Each block is 2ⁿ+1 points on a side, and neighboring blocks share the
// points along their common edge. Every block corner starts with its own
// random value, so the land is not laid out the same way in every block.
// The diamond and square steps then run over the whole map at once, halving
// the spacing each pass, so a point on a shared edge is the mean of points
// in both blocks and the blocks join without seams. The edges of the map do
// not wrap: a point on the map's edge averages only the neighbors it has.
package stitch

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/maloquacious/wg/heightmap"
)

// Options configures the generator. The map is
// BlocksWide·(BlockSize−1)+1 pixels wide and
// BlocksHigh·(BlockSize−1)+1 pixels high.
type Options struct {
	BlocksWide, BlocksHigh int // number of blocks across and down the map
	// BlockSize is the length of a block's side in points, one more than a
	// power of 2: 3, 5, 9, ..., 513, 1025, ...
	BlockSize int
	// Roughness is Martz's H, as in fractal. The random offsets shrink by a
	// factor of 2^-H each time the spacing halves.
	Roughness float64
	Seed      uint64 // seed for the random number generator
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{
		BlocksWide: 2,
		BlocksHigh: 1,
		BlockSize:  1025,
		Roughness:  0.8,
		Seed:       0x0123456789abcdef,
	}
}

// Size returns the width and height of the map in pixels.
func (o Options) Size() (width, height int) {
	return o.BlocksWide*(o.BlockSize-1) + 1, o.BlocksHigh*(o.BlockSize-1) + 1
}

// Generate returns a height map normalized to the range 0...1.
// The same options always produce the same map.
func Generate(opts Options) (*heightmap.Map, error) {
	if opts.BlocksWide < 1 || opts.BlocksHigh < 1 {
		return nil, fmt.Errorf("stitch: invalid layout of %dx%d blocks", opts.BlocksWide, opts.BlocksHigh)
	}
	if side := opts.BlockSize - 1; side < 2 || side&(side-1) != 0 {
		return nil, fmt.Errorf("stitch: invalid block size %d: want 2ⁿ+1, such as 513 or 1025", opts.BlockSize)
	}
	if !(opts.Roughness >= 0) || math.IsInf(opts.Roughness, 0) {
		return nil, fmt.Errorf("stitch: invalid roughness %g", opts.Roughness)
	}
	width, height := opts.Size()
	if width*height > 1<<28 {
		return nil, fmt.Errorf("stitch: map of %dx%d pixels is too large", width, height)
	}
	rnd := rand.New(rand.NewPCG(opts.Seed, opts.Seed))
	hm := &heightmap.Map{
		Width:  width,
		Height: height,
		Data:   diamondSquare(width, height, opts.BlockSize-1, math.Pow(2, -opts.Roughness), rnd),
	}
	normalize(hm.Data)
	return hm, nil
}

// diamondSquare returns a width×height grid in row-major order. The block
// corners lie every side points, and side must be a power of 2. Each pass
// halves the spacing and multiplies the size of the random offsets by ratio.
func diamondSquare(width, height, side int, ratio float64, rnd *rand.Rand) []float64 {
	g := make([]float64, width*height)
	at := func(x, y int) *float64 { return &g[y*width+x] }
	offset := func(scale float64) float64 { return scale * (rnd.Float64() - 0.5) }

	// every block corner starts with its own value
	for y := 0; y < height; y += side {
		for x := 0; x < width; x += side {
			*at(x, y) = 2*rnd.Float64() - 1
		}
	}

	scale := ratio
	for stride := side / 2; stride > 0; stride /= 2 {
		// diamond step: the center of each square is the mean of its corners
		for y := stride; y < height; y += 2 * stride {
			for x := stride; x < width; x += 2 * stride {
				mean := (*at(x-stride, y-stride) + *at(x+stride, y-stride) +
					*at(x-stride, y+stride) + *at(x+stride, y+stride)) / 4
				*at(x, y) = mean + offset(scale)
			}
		}
		// square step: the center of each diamond is the mean of its points,
		// leaving out the ones beyond the edge of the map
		for y := 0; y < height; y += stride {
			for x := (y/stride + 1) % 2 * stride; x < width; x += 2 * stride {
				var sum float64
				var n int
				if x >= stride {
					sum, n = sum+*at(x-stride, y), n+1
				}
				if x+stride < width {
					sum, n = sum+*at(x+stride, y), n+1
				}
				if y >= stride {
					sum, n = sum+*at(x, y-stride), n+1
				}
				if y+stride < height {
					sum, n = sum+*at(x, y+stride), n+1
				}
				*at(x, y) = sum/float64(n) + offset(scale)
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
