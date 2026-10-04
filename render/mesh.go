// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"
	"math"

	"github.com/maloquacious/wg/voronoi"
)

// MeshOptions configures Mesh.
type MeshOptions struct {
	// Borders draws the outline of every cell.
	Borders bool
}

// DefaultMeshOptions returns the options used by the tests.
func DefaultMeshOptions() MeshOptions {
	return MeshOptions{Borders: true}
}

// Mesh renders the cells of a Voronoi mesh. Land cells use the same
// elevation tint as Topo, ocean cells are tinted by depth, and a coastline
// separates them.
func Mesh(mesh *voronoi.Mesh, opts MeshOptions) *image.RGBA {
	width, height := mesh.Width, mesh.Height

	colors := cellColors(mesh)
	owner := owners(mesh)
	img := paint(owner, colors, width, height)

	// draw a border where a pixel's right or lower neighbor is in another cell
	border := color.RGBA{R: 50, G: 40, B: 30, A: 255}
	coast := color.RGBA{R: 40, G: 60, B: 90, A: 255}
	for y := range height {
		for x := range width {
			i := owner[y*width+x]
			for _, d := range [][2]int{{1, 0}, {0, 1}} {
				px, py := x+d[0], y+d[1]
				if px >= width || py >= height {
					continue
				}
				j := owner[py*width+px]
				if i == j {
					continue
				}
				if mesh.Cells[i].Ocean != mesh.Cells[j].Ocean {
					img.SetRGBA(x, y, coast)
					break
				}
				if opts.Borders {
					img.SetRGBA(x, y, blend(img.RGBAAt(x, y), border, 0.35))
					break
				}
			}
		}
	}
	return img
}

// cellColors returns the color of each cell: an elevation tint for land and
// a depth tint for ocean.
func cellColors(mesh *voronoi.Mesh) []color.RGBA {
	// the tints run from sea level to the highest and lowest cells
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, c := range mesh.Cells {
		lo, hi = min(lo, c.Elevation), max(hi, c.Elevation)
	}
	sea := mesh.SeaLevel
	if sea < lo {
		sea = lo
	}
	colors := make([]color.RGBA, len(mesh.Cells))
	for i, c := range mesh.Cells {
		if c.Ocean {
			colors[i] = waterAt(ratio(sea-c.Elevation, sea-lo))
			continue
		}
		tint := tintAt(ratio(c.Elevation-sea, hi-sea))
		colors[i] = color.RGBA{R: uint8(tint[0]), G: uint8(tint[1]), B: uint8(tint[2]), A: 255}
	}
	return colors
}

// owners returns the cell that each pixel belongs to.
func owners(mesh *voronoi.Mesh) []int32 {
	width, height := mesh.Width, mesh.Height
	owner := make([]int32, width*height)
	for i := range owner {
		owner[i] = -1
	}
	for i, c := range mesh.Cells {
		fillPolygon(c.Polygon, width, height, func(y, x0, x1 int) {
			for x := x0; x < x1; x++ {
				owner[y*width+x] = int32(i)
			}
		})
	}
	// rounding can leave a pixel on a shared edge unclaimed; give it to the
	// pixel before it
	for n := range owner {
		if owner[n] < 0 {
			owner[n] = 0
			if n > 0 {
				owner[n] = owner[n-1]
			}
		}
	}
	return owner
}

// paint returns an image with every pixel in the color of its cell.
func paint(owner []int32, colors []color.RGBA, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for n, i := range owner {
		img.Pix[n*4+0] = colors[i].R
		img.Pix[n*4+1] = colors[i].G
		img.Pix[n*4+2] = colors[i].B
		img.Pix[n*4+3] = 255
	}
	return img
}

// fillPolygon calls span for every row of pixels whose centers lie inside the
// convex polygon. A pixel x is inside when left <= x+0.5 < right.
func fillPolygon(polygon []voronoi.Point, width, height int, span func(y, x0, x1 int)) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, q := range polygon {
		minY, maxY = min(minY, q.Y), max(maxY, q.Y)
	}
	for y := max(int(minY), 0); y < min(int(math.Ceil(maxY)), height); y++ {
		yc := float64(y) + 0.5
		left, right := math.Inf(1), math.Inf(-1)
		for n, a := range polygon {
			b := polygon[(n+1)%len(polygon)]
			if (a.Y <= yc) != (b.Y <= yc) {
				x := a.X + (yc-a.Y)*(b.X-a.X)/(b.Y-a.Y)
				left, right = min(left, x), max(right, x)
			}
		}
		if left > right {
			continue
		}
		x0 := max(int(math.Ceil(left-0.5)), 0)
		x1 := min(int(math.Ceil(right-0.5)), width)
		if x0 < x1 {
			span(y, x0, x1)
		}
	}
}

// ratio returns a/b clamped to 0...1, or 0 when b is zero.
func ratio(a, b float64) float64 {
	if b <= 0 {
		return 0
	}
	return clamp01(a / b)
}
