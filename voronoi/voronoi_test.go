// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package voronoi_test

import (
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/heightmap"
	"github.com/maloquacious/wg/voronoi"
)

// defaultMap is the stage 1 map built from the default options.
var defaultMap = sync.OnceValues(func() (*heightmap.Map, error) {
	return fracture.Generate(fracture.DefaultOptions())
})

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
	if len(mesh.Cells) != opts.Cells {
		t.Fatalf("cells: want %d, got %d", opts.Cells, len(mesh.Cells))
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
		if want := opts.Cells * pct / 100; ocean != want {
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
	opts.Cells = 50
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
		{Cells: 2, OceanPercent: 70},
		{Cells: 100, OceanPercent: -1},
		{Cells: 100, OceanPercent: 101},
	} {
		if _, err := voronoi.Generate(hm, opts); err == nil {
			t.Errorf("%+v: want error, got nil", opts)
		}
	}
}
