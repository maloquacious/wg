// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package terrain_test

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
	"github.com/maloquacious/wg/terrain"
	"github.com/maloquacious/wg/voronoi"
)

// renderDir enables writing the test terrain as an image, for example
//
//	go test ./terrain -render=../var
var renderDir = flag.String("render", "", "write rendered terrain to this directory")

// defaultMesh is the stage 2 mesh built from the default options.
var defaultMesh = sync.OnceValues(func() (*voronoi.Mesh, error) {
	hm, err := fracture.Generate(fracture.DefaultOptions())
	if err != nil {
		return nil, err
	}
	return voronoi.Generate(hm, voronoi.DefaultOptions())
})

func generate(t *testing.T, opts terrain.Options) *terrain.Terrain {
	t.Helper()
	mesh, err := defaultMesh()
	if err != nil {
		t.Fatal(err)
	}
	tr, err := terrain.Generate(mesh, opts)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestDefault(t *testing.T) {
	tr := generate(t, terrain.DefaultOptions())
	checkDrainage(t, tr)
	checkLakes(t, tr, terrain.DefaultOptions().LakeDepth)
	if len(tr.Lakes) == 0 {
		t.Error("want lakes, got none")
	}
	t.Logf("%d lakes", len(tr.Lakes))
}

func TestRender(t *testing.T) {
	if *renderDir == "" {
		t.Skip("set -render to write the terrain image")
	}
	path := filepath.Join(*renderDir, "terrain.png")
	img := render.Terrain(generate(t, terrain.DefaultOptions()), render.DefaultTerrainOptions())
	if err := render.WritePNG(path, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func TestLakeDepth(t *testing.T) {
	none := generate(t, terrain.Options{LakeDepth: 1})
	if len(none.Lakes) != 0 {
		t.Errorf("depth 1: want no lakes, got %d", len(none.Lakes))
	}
	for i, c := range none.Cells {
		if c.Lake != -1 {
			t.Fatalf("depth 1: cell %d is in lake %d", i, c.Lake)
		}
	}
	all := generate(t, terrain.Options{LakeDepth: 0})
	checkDrainage(t, all)
	checkLakes(t, all, 0)
	if def := generate(t, terrain.DefaultOptions()); len(all.Lakes) < len(def.Lakes) {
		t.Errorf("depth 0: want at least the %d default lakes, got %d", len(def.Lakes), len(all.Lakes))
	}
}

// TestCrater builds a cone with a crater in its top, ringed by a flat rim.
// The crater must become a single lake whose surface is near the height of
// the rim.
func TestCrater(t *testing.T) {
	const size, floor, rim = 200, 0.5, 0.9
	hm, err := heightmap.New(size, size)
	if err != nil {
		t.Fatal(err)
	}
	for y := range size {
		for x := range size {
			r := math.Hypot(float64(x)+0.5-size/2, float64(y)+0.5-size/2)
			var e float64
			switch {
			case r < 40:
				e = floor
			case r < 60:
				e = rim
			default:
				// the outside of the cone falls from the rim to 0 at the
				// corners of the map
				e = rim * (1 - (r-60)/(size/math.Sqrt2-60))
			}
			hm.Data[y*size+x] = max(e, 0)
		}
	}
	mesh, err := voronoi.Generate(hm, voronoi.Options{Cells: 400, OceanPercent: 20, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := terrain.Generate(mesh, terrain.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	checkDrainage(t, tr)
	checkLakes(t, tr, terrain.DefaultOptions().LakeDepth)

	if len(tr.Lakes) != 1 {
		t.Fatalf("want 1 lake, got %d", len(tr.Lakes))
	}
	lake := tr.Lakes[0]
	for i, c := range mesh.Cells {
		if r := math.Hypot(c.Site.X-size/2, c.Site.Y-size/2); r < 25 && tr.Cells[i].Lake != 0 {
			t.Errorf("cell %d: %.1f from the center but not in the lake", i, r)
		}
	}
	if lake.Level < rim-0.05 || lake.Level > rim {
		t.Errorf("level: want close to the rim at %g, got %g", rim, lake.Level)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := generate(t, terrain.DefaultOptions()), generate(t, terrain.DefaultOptions())
	if !slices.Equal(a.Corners, b.Corners) || !slices.Equal(a.Cells, b.Cells) || len(a.Lakes) != len(b.Lakes) {
		t.Fatal("terrain differs between runs")
	}
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	mesh, err := defaultMesh()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := terrain.Generate(mesh, terrain.Options{LakeDepth: -0.1}); err == nil {
		t.Error("negative lake depth: want error, got nil")
	}
}

// checkDrainage checks that water from every corner runs downhill to an
// outlet: a corner that touches the ocean or lies on the edge of the map.
func checkDrainage(t *testing.T, tr *terrain.Terrain) {
	t.Helper()
	mesh := tr.Mesh
	for k, c := range tr.Corners {
		outlet := c.Ocean || c.Coast || mesh.Corners[k].Border
		if outlet != (c.Downslope < 0) {
			t.Fatalf("corner %d: outlet %v, but downslope %d", k, outlet, c.Downslope)
		}
		if outlet {
			if c.Outlet != k {
				t.Fatalf("corner %d: outlet drains to %d", k, c.Outlet)
			}
			continue
		}
		if !slices.Contains(mesh.Corners[k].Adjacent, c.Downslope) {
			t.Fatalf("corner %d: downslope %d is not adjacent", k, c.Downslope)
		}
		// only lake corners, which lie flat, may flow to a corner at the
		// same elevation
		down := tr.Corners[c.Downslope]
		if down.Elevation > c.Elevation || (down.Elevation == c.Elevation && c.Lake < 0) {
			t.Fatalf("corner %d: flows up from %g to %g", k, c.Elevation, down.Elevation)
		}

		// follow the water to its outlet
		at := k
		for range len(tr.Corners) {
			if tr.Corners[at].Downslope < 0 {
				break
			}
			at = tr.Corners[at].Downslope
		}
		if at != c.Outlet {
			t.Fatalf("corner %d: drains to %d, want outlet %d", k, at, c.Outlet)
		}
	}
}

// checkLakes checks that every lake is a flat, enclosed pit that drains
// through its outlet.
func checkLakes(t *testing.T, tr *terrain.Terrain, depth float64) {
	t.Helper()
	mesh := tr.Mesh
	for n, lake := range tr.Lakes {
		if lake.Depth < depth {
			t.Errorf("lake %d: depth %g is less than %g", n, lake.Depth, depth)
		}
		if len(lake.Cells) == 0 {
			t.Errorf("lake %d: covers no cells", n)
		}
		for _, i := range lake.Cells {
			if tr.Cells[i].Lake != n || tr.Cells[i].Ocean || tr.Cells[i].Elevation != lake.Level {
				t.Errorf("lake %d: cell %d is %+v", n, i, tr.Cells[i])
			}
			for _, k := range mesh.Cells[i].Corners {
				if tr.Corners[k].Lake != n {
					t.Errorf("lake %d: cell %d has corner %d outside the lake", n, i, k)
				}
			}
		}
		for _, k := range lake.Corners {
			c := tr.Corners[k]
			if c.Lake != n || c.Elevation != lake.Level || c.Ocean || c.Coast || mesh.Corners[k].Border {
				t.Errorf("lake %d: corner %d is %+v", n, k, c)
			}
		}
		out := tr.Corners[lake.Outlet]
		if out.Lake >= 0 || out.Elevation != lake.Level {
			t.Errorf("lake %d: outlet %d is %+v, want land at %g", n, lake.Outlet, out, lake.Level)
		}
		if !slices.ContainsFunc(mesh.Corners[lake.Outlet].Adjacent, func(k int) bool { return tr.Corners[k].Lake == n }) {
			t.Errorf("lake %d: outlet %d does not touch the lake", n, lake.Outlet)
		}
	}
	for i, c := range tr.Cells {
		if c.Lake >= 0 && !slices.Contains(tr.Lakes[c.Lake].Cells, i) {
			t.Errorf("cell %d: lake %d does not list it", i, c.Lake)
		}
	}
}
