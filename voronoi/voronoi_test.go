// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package voronoi_test

import (
	"flag"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/heightmap"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/voronoi"
)

// renderDir enables writing the test meshes as images, for example
//
//	go test ./voronoi -render=../var
var renderDir = flag.String("render", "", "write rendered meshes to this directory")

// defaultMap is the stage 1 map built from the default options.
var defaultMap = sync.OnceValues(func() (*heightmap.Map, error) {
	return fracture.Generate(fracture.DefaultOptions())
})

// sizeFor returns the cell size that gives hm exactly n cells.
func sizeFor(hm *heightmap.Map, n int) float64 {
	return math.Sqrt(float64(hm.Width*hm.Height) / float64(n))
}

func defaultMesh(t *testing.T) *voronoi.Mesh {
	t.Helper()
	hm, err := defaultMap()
	if err != nil {
		t.Fatal(err)
	}
	mesh, err := voronoi.Generate(hm, voronoi.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	return mesh
}

func TestMeshCoversMap(t *testing.T) {
	mesh := defaultMesh(t)
	opts := voronoi.DefaultOptions()
	if want := opts.CellCount(mesh.Width, mesh.Height); len(mesh.Cells) != want {
		t.Fatalf("cells: want %d, got %d", want, len(mesh.Cells))
	}
	if mesh.Width != 1920 || mesh.Height != 1080 {
		t.Errorf("size: want 1920x1080, got %dx%d", mesh.Width, mesh.Height)
	}

	const eps = 1e-6
	var total float64
	for i, c := range mesh.Cells {
		if len(c.Polygon) < 3 {
			t.Fatalf("cell %d: polygon has %d points", i, len(c.Polygon))
		}
		for _, q := range c.Polygon {
			if q.X < -eps || q.X > float64(mesh.Width)+eps || q.Y < -eps || q.Y > float64(mesh.Height)+eps {
				t.Fatalf("cell %d: point %v is outside the map", i, q)
			}
		}
		if c.Elevation < 0 || c.Elevation > 1 {
			t.Errorf("cell %d: elevation %g is outside 0...1", i, c.Elevation)
		}
		total += c.Area()
	}
	// the cells tile the map without gaps or overlaps
	if want := float64(mesh.Width * mesh.Height); math.Abs(total-want) > want*1e-9 {
		t.Errorf("area: want %g, got %g", want, total)
	}
}

func TestRender(t *testing.T) {
	if *renderDir == "" {
		t.Skip("set -render to write the mesh image")
	}
	path := filepath.Join(*renderDir, "voronoi-mesh.png")
	if err := render.WritePNG(path, render.Mesh(defaultMesh(t), render.DefaultMeshOptions())); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func TestNeighborsAreSymmetric(t *testing.T) {
	mesh := defaultMesh(t)
	for i, c := range mesh.Cells {
		if len(c.Neighbors) == 0 {
			t.Errorf("cell %d: no neighbors", i)
		}
		for _, j := range c.Neighbors {
			if j == i {
				t.Errorf("cell %d: is its own neighbor", i)
			}
			if !slices.Contains(mesh.Cells[j].Neighbors, i) {
				t.Errorf("cell %d: lists %d, which does not list it back", i, j)
			}
		}
	}
}

func TestGraph(t *testing.T) {
	checkGraph(t, defaultMesh(t))

	// small meshes have cells that touch several sides of the map
	hm, err := fracture.Generate(fracture.Options{Width: 160, Height: 90, Rounds: 200, Seed: 0x0123456789abcdef})
	if err != nil {
		t.Fatal(err)
	}
	for _, cells := range []int{3, 4, 5, 10, 100} {
		for seed := range uint64(20) {
			mesh, err := voronoi.Generate(hm, voronoi.Options{CellSize: sizeFor(hm, cells), OceanPercent: 70, Seed: seed})
			if err != nil {
				t.Fatalf("%d cells, seed %d: %v", cells, seed, err)
			}
			if len(mesh.Cells) != cells {
				t.Fatalf("%d cells, seed %d: got %d cells", cells, seed, len(mesh.Cells))
			}
			checkGraph(t, mesh)
		}
	}
}

// checkGraph checks that the cells, corners and edges of the mesh agree with
// each other.
func checkGraph(t *testing.T, mesh *voronoi.Mesh) {
	t.Helper()
	const eps = 1e-6
	width, height := float64(mesh.Width), float64(mesh.Height)
	onBorder := func(q voronoi.Point) bool {
		return math.Abs(q.X) < eps || math.Abs(q.Y) < eps || math.Abs(q.X-width) < eps || math.Abs(q.Y-height) < eps
	}

	pairs := make(map[[2]int]int) // edges between each pair of cells
	for e, edge := range mesh.Edges {
		a, b := edge.Cells[0], edge.Cells[1]
		if edge.Border() {
			if !onBorder(mesh.Corners[edge.Corners[0]].Point) || !onBorder(mesh.Corners[edge.Corners[1]].Point) {
				t.Fatalf("edge %d: border edge is not on the map border", e)
			}
		} else if a >= b {
			t.Fatalf("edge %d: cells %v are out of order", e, edge.Cells)
		} else {
			pairs[edge.Cells]++
		}
		if edge.Corners[0] == edge.Corners[1] {
			t.Fatalf("edge %d: both ends are corner %d", e, edge.Corners[0])
		}
	}

	for i, c := range mesh.Cells {
		if len(c.Corners) != len(c.Polygon) || len(c.Edges) != len(c.Polygon) {
			t.Fatalf("cell %d: %d points, %d corners, %d edges", i, len(c.Polygon), len(c.Corners), len(c.Edges))
		}
		for n, k := range c.Corners {
			if mesh.Corners[k].Point != c.Polygon[n] {
				t.Fatalf("cell %d: point %d is not at corner %d", i, n, k)
			}
			if !slices.Contains(mesh.Corners[k].Touches, i) {
				t.Fatalf("cell %d: corner %d does not touch it", i, k)
			}
			edge := mesh.Edges[c.Edges[n]]
			next := c.Corners[(n+1)%len(c.Corners)]
			if edge.Corners != [2]int{k, next} && edge.Corners != [2]int{next, k} {
				t.Fatalf("cell %d: edge %d does not join corners %d and %d", i, c.Edges[n], k, next)
			}
			if edge.Cells[0] != i && edge.Cells[1] != i {
				t.Fatalf("cell %d: edge %d belongs to cells %v", i, c.Edges[n], edge.Cells)
			}
		}
		for _, j := range c.Neighbors {
			if n := pairs[[2]int{min(i, j), max(i, j)}]; n != 1 {
				t.Fatalf("cells %d and %d: %d edges between them, want 1", i, j, n)
			}
		}
	}

	var touches int
	for k, corner := range mesh.Corners {
		if corner.Border != onBorder(corner.Point) {
			t.Fatalf("corner %d: border is %v at %v", k, corner.Border, corner.Point)
		}
		if !corner.Border && len(corner.Touches) < 3 {
			t.Fatalf("corner %d: touches %d cells, want at least 3", k, len(corner.Touches))
		}
		if len(corner.Adjacent) != len(corner.Edges) {
			t.Fatalf("corner %d: %d adjacent corners, %d edges", k, len(corner.Adjacent), len(corner.Edges))
		}
		for _, j := range corner.Adjacent {
			if !slices.Contains(mesh.Corners[j].Adjacent, k) {
				t.Fatalf("corner %d: lists %d, which does not list it back", k, j)
			}
		}
		for _, e := range corner.Edges {
			if mesh.Edges[e].Corners[0] != k && mesh.Edges[e].Corners[1] != k {
				t.Fatalf("corner %d: edge %d does not end at it", k, e)
			}
		}
		touches += len(corner.Touches)
	}
	var points int
	for _, c := range mesh.Cells {
		points += len(c.Corners)
	}
	if touches != points {
		t.Fatalf("corners touch %d cells, cells list %d corners", touches, points)
	}

	// Euler's formula for a subdivided rectangle: V - E + F = 1
	if v, e, f := len(mesh.Corners), len(mesh.Edges), len(mesh.Cells); v-e+f != 1 {
		t.Errorf("%d corners - %d edges + %d cells = %d, want 1", v, e, f, v-e+f)
	}
}

func TestOcean(t *testing.T) {
	hm, err := defaultMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, pct := range []int{0, 30, 70, 100} {
		opts := voronoi.DefaultOptions()
		opts.OceanPercent = pct
		mesh, err := voronoi.Generate(hm, opts)
		if err != nil {
			t.Fatal(err)
		}
		var ocean int
		for i, c := range mesh.Cells {
			if c.Ocean {
				ocean++
				if c.Elevation > mesh.SeaLevel {
					t.Errorf("%d%%: ocean cell %d is above sea level", pct, i)
				}
			} else if c.Elevation < mesh.SeaLevel {
				t.Errorf("%d%%: land cell %d is below sea level", pct, i)
			}
		}
		if want := len(mesh.Cells) * pct / 100; ocean != want {
			t.Errorf("%d%%: want %d ocean cells, got %d", pct, want, ocean)
		}
		if pct == 0 && mesh.SeaLevel != -1 {
			t.Errorf("0%%: want sea level -1, got %g", mesh.SeaLevel)
		}
	}
}

// TestElevationMatchesNearestSite checks the polygon scan against assigning
// every pixel to its nearest site.
func TestElevationMatchesNearestSite(t *testing.T) {
	hm, err := fracture.Generate(fracture.Options{Width: 160, Height: 90, Rounds: 200, Seed: 0x0123456789abcdef})
	if err != nil {
		t.Fatal(err)
	}
	opts := voronoi.DefaultOptions()
	opts.CellSize = sizeFor(hm, 50)
	mesh, err := voronoi.Generate(hm, opts)
	if err != nil {
		t.Fatal(err)
	}

	sums := make([]float64, len(mesh.Cells))
	counts := make([]int, len(mesh.Cells))
	for y := range hm.Height {
		for x := range hm.Width {
			px, py := float64(x)+0.5, float64(y)+0.5
			nearest, best := 0, math.Inf(1)
			for i, c := range mesh.Cells {
				if d := math.Hypot(c.Site.X-px, c.Site.Y-py); d < best {
					nearest, best = i, d
				}
			}
			sums[nearest] += hm.At(x, y)
			counts[nearest]++
		}
	}
	for i, c := range mesh.Cells {
		if counts[i] == 0 {
			continue
		}
		if want := sums[i] / float64(counts[i]); math.Abs(c.Elevation-want) > 1e-9 {
			t.Errorf("cell %d: want elevation %g, got %g", i, want, c.Elevation)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := defaultMesh(t), defaultMesh(t)
	for i := range a.Cells {
		if a.Cells[i].Site != b.Cells[i].Site || a.Cells[i].Elevation != b.Cells[i].Elevation {
			t.Fatalf("cell %d differs between runs", i)
		}
	}
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	hm, err := defaultMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []voronoi.Options{
		{CellSize: 0, OceanPercent: 70},
		{CellSize: -14, OceanPercent: 70},
		{CellSize: math.NaN(), OceanPercent: 70},
		{CellSize: 2000, OceanPercent: 70}, // fewer than 3 cells
		{CellSize: 14, OceanPercent: -1},
		{CellSize: 14, OceanPercent: 101},
	} {
		if _, err := voronoi.Generate(hm, opts); err == nil {
			t.Errorf("%+v: want error, got nil", opts)
		}
	}
}
