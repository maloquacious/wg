// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"
	"math"

	"github.com/maloquacious/wg/rivers"
)

// RiversOptions configures Rivers.
type RiversOptions struct {
	// Scale sets the width of a river: Scale times the square root of its
	// flow, which is an area in square pixels, and never less than one
	// pixel.
	Scale float64
	// Borders draws the outline of every cell under the rivers.
	Borders bool
}

// DefaultRiversOptions returns the options used by the tests.
func DefaultRiversOptions() RiversOptions {
	return RiversOptions{Scale: 0.03}
}

// riverColor is the RIVER color from the Red Blob Games mapgen2 palette.
var riverColor = color.RGBA{R: 0x22, G: 0x55, B: 0x88, A: 255}

// Rivers renders the terrain under the network, without drainage lines, and
// draws each river with a width that grows with the square root of its flow.
func Rivers(n *rivers.Network, opts RiversOptions) *image.RGBA {
	img := Terrain(n.Terrain, TerrainOptions{Borders: opts.Borders})
	mesh := n.Terrain.Mesh
	for e, flow := range n.River {
		if flow == 0 {
			continue
		}
		a, b := mesh.Corners[mesh.Edges[e].Corners[0]].Point, mesh.Corners[mesh.Edges[e].Corners[1]].Point
		width := max(1, opts.Scale*math.Sqrt(flow))
		drawWideLine(img, a.X, a.Y, b.X, b.Y, width, riverColor)
	}
	return img
}

// drawWideLine paints the pixels whose centers lie within width/2 of the
// segment from (x0, y0) to (x1, y1).
func drawWideLine(img *image.RGBA, x0, y0, x1, y1, width float64, c color.RGBA) {
	r := max(width/2, 0.5)
	dx, dy := x1-x0, y1-y0
	length2 := dx*dx + dy*dy
	bounds := img.Bounds()
	for y := int(math.Floor(min(y0, y1) - r)); y <= int(math.Ceil(max(y0, y1)+r)); y++ {
		for x := int(math.Floor(min(x0, x1) - r)); x <= int(math.Ceil(max(x0, x1)+r)); x++ {
			if !image.Pt(x, y).In(bounds) {
				continue
			}
			// distance from the pixel center to the nearest point of the segment
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if length2 > 0 {
				t = clamp01(((px-x0)*dx + (py-y0)*dy) / length2)
			}
			if math.Hypot(px-(x0+t*dx), py-(y0+t*dy)) <= r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}
