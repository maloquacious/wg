// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"
	"math"

	"github.com/maloquacious/wg/terrain"
)

// TerrainOptions configures Terrain.
type TerrainOptions struct {
	// Drainage draws a line from every land corner to the corner downhill
	// of it.
	Drainage bool
}

// DefaultTerrainOptions returns the options used by the tests.
func DefaultTerrainOptions() TerrainOptions {
	return TerrainOptions{Drainage: true}
}

// lakeColor is the LAKE color from the Red Blob Games mapgen2 palette.
var lakeColor = color.RGBA{R: 0x33, G: 0x66, B: 0x99, A: 255}

// Terrain renders the cells of a terrain like Mesh, with lakes, coastlines
// and lake shores, and optionally the drainage between corners.
func Terrain(t *terrain.Terrain, opts TerrainOptions) *image.RGBA {
	mesh := t.Mesh
	width, height := mesh.Width, mesh.Height
	colors := cellColors(mesh)
	for i, c := range t.Cells {
		if c.Lake >= 0 {
			colors[i] = lakeColor
		}
	}
	owner := owners(mesh)
	img := paint(owner, colors, width, height)

	// outline the ocean and the lakes
	shore := color.RGBA{R: 40, G: 60, B: 90, A: 255}
	for y := range height {
		for x := range width {
			i := owner[y*width+x]
			for _, d := range [][2]int{{1, 0}, {0, 1}} {
				px, py := x+d[0], y+d[1]
				if px >= width || py >= height {
					continue
				}
				j := owner[py*width+px]
				if t.Cells[i].Ocean != t.Cells[j].Ocean || (t.Cells[i].Lake >= 0) != (t.Cells[j].Lake >= 0) {
					img.SetRGBA(x, y, shore)
					break
				}
			}
		}
	}

	if opts.Drainage {
		drain := color.RGBA{R: 20, G: 40, B: 140, A: 255}
		for k, c := range t.Corners {
			if c.Downslope < 0 || c.Ocean || c.Lake >= 0 {
				continue
			}
			a, b := mesh.Corners[k].Point, mesh.Corners[c.Downslope].Point
			drawLine(img, a.X, a.Y, b.X, b.Y, drain, 0.6)
		}
	}
	return img
}

// drawLine blends a one pixel wide line from (x0, y0) to (x1, y1) into img.
func drawLine(img *image.RGBA, x0, y0, x1, y1 float64, c color.RGBA, alpha float64) {
	steps := int(math.Ceil(max(math.Abs(x1-x0), math.Abs(y1-y0))))
	bounds := img.Bounds()
	for s := range steps + 1 {
		t := 0.0
		if steps > 0 {
			t = float64(s) / float64(steps)
		}
		x, y := int(x0+(x1-x0)*t), int(y0+(y1-y0)*t)
		if image.Pt(x, y).In(bounds) {
			img.SetRGBA(x, y, blend(img.RGBAAt(x, y), c, alpha))
		}
	}
}
