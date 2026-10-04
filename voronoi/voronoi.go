// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package voronoi implements the second stage of the map pipeline. It lays a
// Voronoi mesh over a height map, gives each cell the mean elevation of the
// pixels it covers, and floods the lowest cells to make the ocean.
//
// The mesh holds two graphs, as in "Polygonal Map Generation for Games": cells
// joined to the neighbors they share an edge with, and the corners of the cell
// polygons joined by those edges.
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
	// Corners holds the index of the corner at each point of the polygon.
	Corners []int
	// Edges holds the index of the edge from Polygon[n] to Polygon[n+1] at
	// Edges[n].
	Edges []int
	// Neighbors holds the indices of the cells that share an edge with this
	// one, sorted.
	Neighbors []int
	// Elevation is the mean elevation of the pixels whose centers lie in the cell.
	Elevation float64
	// Ocean is true when the cell is below sea level.
	Ocean bool
}

// Corner is a point where cell polygons meet.
type Corner struct {
	Point Point
	// Touches holds the indices of the cells that meet at the corner, sorted.
	Touches []int
	// Adjacent holds the indices of the corners that share an edge with this
	// one, sorted.
	Adjacent []int
	// Edges holds the indices of the edges that end at the corner, sorted.
	Edges []int
	// Border is true when the corner lies on the edge of the map.
	Border bool
}

// Edge is a side of a cell polygon. It separates two cells, or a cell from
// the outside of the map.
type Edge struct {
	// Cells holds the cells on either side, with Cells[0] < Cells[1]. Cells[1]
	// is -1 when the edge lies on the edge of the map.
	Cells [2]int
	// Corners holds the ends of the edge, in the order they appear in the
	// polygon of Cells[0].
	Corners [2]int
}

// Border reports whether the edge lies on the edge of the map.
func (e *Edge) Border() bool {
	return e.Cells[1] == -1
}

// Mesh is a Voronoi mesh covering a Width x Height map.
type Mesh struct {
	Width, Height int
	Cells         []Cell
	Corners       []Corner
	Edges         []Edge
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
		polygons, err := build(sites, width, height)
		if err != nil {
			return nil, err
		}
		for i, p := range polygons {
			c := centroid(p.points)
			sites[i] = delaunay.Point{X: c.X, Y: c.Y}
		}
	}

	polygons, err := build(sites, width, height)
	if err != nil {
		return nil, err
	}
	cells, corners, edges, err := connect(polygons)
	if err != nil {
		return nil, err
	}
	for i := range cells {
		cells[i].Site = Point{X: sites[i].X, Y: sites[i].Y}
		cells[i].Elevation = elevation(hm, &cells[i])
	}
	return &Mesh{
		Width:    hm.Width,
		Height:   hm.Height,
		Cells:    cells,
		Corners:  corners,
		Edges:    edges,
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

// build returns the Voronoi polygons for the sites, clipped to the rectangle
// (0, 0)-(width, height). Each edge is labeled with the site on its other
// side.
func build(sites []delaunay.Point, width, height float64) ([]polygon, error) {
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

	polygons := make([]polygon, len(sites))
	for i, s := range sites {
		site := Point{X: s.X, Y: s.Y}
		p := polygon{
			points: []Point{{0, 0}, {width, 0}, {width, height}, {0, height}},
			edges:  []int{-1, -1, -1, -1},
		}
		for _, j := range adjacent[i] {
			p = p.clip(site, Point{X: sites[j].X, Y: sites[j].Y}, j)
		}
		polygons[i] = p
	}
	return polygons, nil
}

// mergeDistance is how close two polygon points must be to count as one
// corner. Neighboring cells compute the corners they share separately, so the
// copies differ by rounding error, and four or more sites on a circle leave
// edges with almost no length.
const mergeDistance = 1e-6

// connect merges the points that the polygons share into corners, links the
// corners with edges, and returns the cells described in terms of them. Edges
// that are shorter than mergeDistance are dropped.
func connect(polygons []polygon) ([]Cell, []Corner, []Edge, error) {
	// number every polygon point; the points of polygon i start at first[i]
	var points []Point
	first := make([]int, len(polygons))
	for i, p := range polygons {
		first[i] = len(points)
		points = append(points, p.points...)
	}

	// join the points that lie within mergeDistance of each other, keeping
	// the lowest numbered point of each group as its root
	parent := make([]int, len(points))
	for n := range parent {
		parent[n] = n
	}
	find := func(n int) int {
		for parent[n] != n {
			parent[n] = parent[parent[n]]
			n = parent[n]
		}
		return n
	}
	type bucket struct{ x, y int64 }
	grid := make(map[bucket][]int)
	for n, q := range points {
		b := bucket{int64(math.Floor(q.X / mergeDistance)), int64(math.Floor(q.Y / mergeDistance))}
		for dx := int64(-1); dx <= 1; dx++ {
			for dy := int64(-1); dy <= 1; dy++ {
				for _, m := range grid[bucket{b.x + dx, b.y + dy}] {
					if math.Hypot(points[m].X-q.X, points[m].Y-q.Y) <= mergeDistance {
						rn, rm := find(n), find(m)
						parent[max(rn, rm)] = min(rn, rm)
					}
				}
			}
		}
		grid[b] = append(grid[b], n)
	}

	// number the corners in the order their roots first appear
	var corners []Corner
	id := make([]int, len(points))
	for n := range points {
		if r := find(n); r == n {
			id[n] = len(corners)
			corners = append(corners, Corner{Point: points[n]})
		} else {
			id[n] = id[r]
		}
	}

	var edges []Edge
	seen := make([]int, 0, len(polygons)*3) // times each edge has been visited
	edgeAt := make(map[[2]int]int)          // edge index by its corners, lower first
	cells := make([]Cell, len(polygons))
	for i, p := range polygons {
		// drop the edges whose ends merged into one corner
		var ids, labels []int
		for n := range p.points {
			c := id[first[i]+n]
			if c != id[first[i]+(n+1)%len(p.points)] {
				ids = append(ids, c)
				labels = append(labels, p.edges[n])
			}
		}
		if len(ids) < 3 {
			return nil, nil, nil, fmt.Errorf("voronoi: cell %d collapsed to %d corners", i, len(ids))
		}

		cell := Cell{Corners: ids, Polygon: make([]Point, len(ids)), Edges: make([]int, len(ids))}
		for n, c := range ids {
			cell.Polygon[n] = corners[c].Point
			if slices.Contains(corners[c].Touches, i) {
				return nil, nil, nil, fmt.Errorf("voronoi: cell %d passes through corner %d twice", i, c)
			}
			corners[c].Touches = append(corners[c].Touches, i)

			d, label := ids[(n+1)%len(ids)], labels[n]
			key := [2]int{min(c, d), max(c, d)}
			e, ok := edgeAt[key]
			if !ok {
				e = len(edges)
				edgeAt[key] = e
				edges = append(edges, Edge{Cells: [2]int{i, label}, Corners: [2]int{c, d}})
				seen = append(seen, 0)
				corners[c].Edges = append(corners[c].Edges, e)
				corners[d].Edges = append(corners[d].Edges, e)
			}
			if edges[e].Cells != [2]int{min(i, label), max(i, label)} && edges[e].Cells != [2]int{i, -1} {
				return nil, nil, nil, fmt.Errorf("voronoi: cells %d and %d disagree about edge %d", i, label, e)
			}
			seen[e]++
			cell.Edges[n] = e
			if label >= 0 && !slices.Contains(cell.Neighbors, label) {
				cell.Neighbors = append(cell.Neighbors, label)
			}
		}
		slices.Sort(cell.Neighbors)
		cells[i] = cell
	}

	for e := range edges {
		want := 2
		if edges[e].Border() {
			want = 1
			corners[edges[e].Corners[0]].Border = true
			corners[edges[e].Corners[1]].Border = true
		}
		if seen[e] != want {
			return nil, nil, nil, fmt.Errorf("voronoi: edge %d belongs to %d cells, want %d", e, seen[e], want)
		}
	}
	for c := range corners {
		slices.Sort(corners[c].Edges)
		for _, e := range corners[c].Edges {
			other := edges[e].Corners[0]
			if other == c {
				other = edges[e].Corners[1]
			}
			corners[c].Adjacent = append(corners[c].Adjacent, other)
		}
		slices.Sort(corners[c].Adjacent)
	}
	return cells, corners, edges, nil
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
