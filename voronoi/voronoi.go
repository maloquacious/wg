// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package voronoi implements the second stage of the map pipeline. It lays a
// Voronoi mesh over a height map, gives each cell the mean elevation of the
// pixels it covers, and floods the lowest cells to make the ocean.
package voronoi

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/fogleman/delaunay"
	"github.com/maloquacious/wg/heightmap"
)

// Options configures the mesh.
type Options struct {
	Cells        int    // number of cells in the mesh
	OceanPercent int    // percentage of cells, 0...100, that are ocean
	Seed         uint64 // seed for the random number generator
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{
		Cells:        10_000,
		OceanPercent: 70,
		Seed:         0x0123456789abcdef,
	}
}

// relaxPasses is the number of Lloyd relaxation passes used to even out the
// size of the cells.
const relaxPasses = 2

// Point is a location in pixel coordinates.
type Point struct {
	X, Y float64
}

// Cell is one region of the mesh.
type Cell struct {
	// Site is the point that every location in the cell is closest to.
	Site Point
	// Polygon is the convex outline of the cell, clipped to the map bounds.
	Polygon []Point
	// Neighbors holds the indices of the cells that share an edge with this one.
	Neighbors []int
	// Elevation is the mean elevation of the pixels whose centers lie in the cell.
	Elevation float64
	// Ocean is true when the cell is below sea level.
	Ocean bool
}

// Mesh is a Voronoi mesh covering a Width x Height map.
type Mesh struct {
	Width, Height int
	Cells         []Cell
	// SeaLevel is the elevation of the highest ocean cell, or -1 when there
	// is no ocean.
	SeaLevel float64
}

// Generate lays a mesh over hm. The same map and options always produce the
// same mesh.
func Generate(hm *heightmap.Map, opts Options) (*Mesh, error) {
	if opts.Cells < 3 {
		return nil, fmt.Errorf("voronoi: invalid cells %d", opts.Cells)
	}
	if opts.OceanPercent < 0 || opts.OceanPercent > 100 {
		return nil, fmt.Errorf("voronoi: invalid ocean percent %d", opts.OceanPercent)
	}
	rnd := rand.New(rand.NewPCG(opts.Seed, opts.Seed))
	width, height := float64(hm.Width), float64(hm.Height)

	sites := make([]delaunay.Point, opts.Cells)
	for i := range sites {
		sites[i] = delaunay.Point{X: rnd.Float64() * width, Y: rnd.Float64() * height}
	}

	// Lloyd relaxation: move each site to the centroid of its cell
	for range relaxPasses {
		cells, err := build(sites, width, height)
		if err != nil {
			return nil, err
		}
		for i := range sites {
			c := centroid(cells[i].Polygon)
			sites[i] = delaunay.Point{X: c.X, Y: c.Y}
		}
	}

	cells, err := build(sites, width, height)
	if err != nil {
		return nil, err
	}
	for i := range cells {
		cells[i].Elevation = elevation(hm, &cells[i])
	}
	return &Mesh{
		Width:    hm.Width,
		Height:   hm.Height,
		Cells:    cells,
		SeaLevel: flood(cells, opts.OceanPercent),
	}, nil
}

// flood marks the lowest percent of the cells as ocean and returns the sea
// level. Cells with equal elevations are flooded in index order so that the
// count is exact.
func flood(cells []Cell, percent int) float64 {
	order := make([]int, len(cells))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(cells[a].Elevation, cells[b].Elevation), cmp.Compare(a, b))
	})
	seaLevel := -1.0
	for _, i := range order[:len(cells)*percent/100] {
		cells[i].Ocean = true
		seaLevel = cells[i].Elevation
	}
	return seaLevel
}

// build returns the Voronoi cells for the sites, clipped to the rectangle
// (0, 0)-(width, height).
func build(sites []delaunay.Point, width, height float64) ([]Cell, error) {
	tri, err := delaunay.Triangulate(sites)
	if err != nil {
		return nil, fmt.Errorf("voronoi: %w", err)
	}

	// Voronoi neighbors are a subset of the Delaunay neighbors
	adjacent := make([][]int, len(sites))
	for e, a := range tri.Triangles {
		b := tri.Triangles[nextHalfedge(e)]
		if !slices.Contains(adjacent[a], b) {
			adjacent[a] = append(adjacent[a], b)
			adjacent[b] = append(adjacent[b], a)
		}
	}

	cells := make([]Cell, len(sites))
	for i, s := range sites {
		site := Point{X: s.X, Y: s.Y}
		p := polygon{
			points: []Point{{0, 0}, {width, 0}, {width, height}, {0, height}},
			edges:  []int{-1, -1, -1, -1},
		}
		for _, j := range adjacent[i] {
			p = p.clip(site, Point{X: sites[j].X, Y: sites[j].Y}, j)
		}
		cells[i] = Cell{Site: site, Polygon: p.points, Neighbors: p.neighbors()}
	}
	return cells, nil
}

func nextHalfedge(e int) int {
	if e%3 == 2 {
		return e - 2
	}
	return e + 1
}

// polygon is a convex polygon whose edge n runs from points[n] to
// points[n+1]. edges[n] is the cell on the other side of edge n, or -1 for
// the map border.
type polygon struct {
	points []Point
	edges  []int
}

// clip keeps the part of the polygon that is closer to site than to other.
// Edges created along the bisector are labeled with label.
func (p polygon) clip(site, other Point, label int) polygon {
	// a point q is kept when (q - mid) . normal <= 0
	normal := Point{X: other.X - site.X, Y: other.Y - site.Y}
	mid := Point{X: (site.X + other.X) / 2, Y: (site.Y + other.Y) / 2}
	side := func(q Point) float64 {
		return (q.X-mid.X)*normal.X + (q.Y-mid.Y)*normal.Y
	}

	var out polygon
	for n, a := range p.points {
		b := p.points[(n+1)%len(p.points)]
		sa, sb := side(a), side(b)
		switch {
		case sa <= 0 && sb <= 0:
			out.add(a, p.edges[n])
		case sa <= 0:
			// leaving: the next edge runs along the bisector
			out.add(a, p.edges[n])
			out.add(lerp(a, b, sa/(sa-sb)), label)
		case sb <= 0:
			// entering: the next edge continues the original edge
			out.add(lerp(a, b, sa/(sa-sb)), p.edges[n])
		}
	}
	return out
}

func (p *polygon) add(q Point, edge int) {
	p.points = append(p.points, q)
	p.edges = append(p.edges, edge)
}

// neighbors returns the cells on the other side of the edges that have a
// non-trivial length.
func (p polygon) neighbors() []int {
	const minLength = 1e-6
	var out []int
	for n, label := range p.edges {
		a, b := p.points[n], p.points[(n+1)%len(p.points)]
		if label >= 0 && math.Hypot(b.X-a.X, b.Y-a.Y) > minLength && !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	slices.Sort(out)
	return out
}

func lerp(a, b Point, t float64) Point {
	return Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
}

// Area returns the area of the cell in square pixels.
func (c *Cell) Area() float64 {
	var sum float64
	for n, a := range c.Polygon {
		b := c.Polygon[(n+1)%len(c.Polygon)]
		sum += a.X*b.Y - b.X*a.Y
	}
	return math.Abs(sum) / 2
}

// centroid returns the center of mass of the polygon.
func centroid(points []Point) Point {
	var sum, cx, cy float64
	for n, a := range points {
		b := points[(n+1)%len(points)]
		cross := a.X*b.Y - b.X*a.Y
		sum += cross
		cx += (a.X + b.X) * cross
		cy += (a.Y + b.Y) * cross
	}
	return Point{X: cx / (3 * sum), Y: cy / (3 * sum)}
}

// elevation returns the mean elevation of the pixels whose centers lie
// inside the cell. A cell too small to hold a pixel center takes the
// elevation of the pixel under its site.
func elevation(hm *heightmap.Map, c *Cell) float64 {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, q := range c.Polygon {
		minY, maxY = min(minY, q.Y), max(maxY, q.Y)
	}

	var sum float64
	var count int
	for y := max(int(minY), 0); y < min(int(math.Ceil(maxY)), hm.Height); y++ {
		// find where the row through the pixel centers crosses the polygon
		yc := float64(y) + 0.5
		left, right := math.Inf(1), math.Inf(-1)
		for n, a := range c.Polygon {
			b := c.Polygon[(n+1)%len(c.Polygon)]
			if (a.Y <= yc) != (b.Y <= yc) {
				x := a.X + (yc-a.Y)*(b.X-a.X)/(b.Y-a.Y)
				left, right = min(left, x), max(right, x)
			}
		}
		if left > right {
			continue
		}
		// a pixel belongs to the cell when left <= x+0.5 < right
		x0 := max(int(math.Ceil(left-0.5)), 0)
		x1 := min(int(math.Ceil(right-0.5)), hm.Width)
		for x := x0; x < x1; x++ {
			sum += hm.At(x, y)
			count++
		}
	}
	if count == 0 {
		x := min(int(c.Site.X), hm.Width-1)
		y := min(int(c.Site.Y), hm.Height-1)
		return hm.At(x, y)
	}
	return sum / float64(count)
}
