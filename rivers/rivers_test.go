// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package rivers_test

import (
	"flag"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/rivers"
	"github.com/maloquacious/wg/terrain"
	"github.com/maloquacious/wg/voronoi"
)

// renderDir enables writing the test rivers as an image, for example
//
//	go test ./rivers -render=../var
var renderDir = flag.String("render", "", "write rendered rivers to this directory")

// defaultTerrain is the stage 3 terrain built from the default options.
var defaultTerrain = sync.OnceValues(func() (*terrain.Terrain, error) {
	hm, err := fracture.Generate(fracture.DefaultOptions())
	if err != nil {
		return nil, err
	}
	mesh, err := voronoi.Generate(hm, voronoi.DefaultOptions())
	if err != nil {
		return nil, err
	}
	return terrain.Generate(mesh, terrain.DefaultOptions())
})

func generate(t *testing.T, opts rivers.Options) *rivers.Network {
	t.Helper()
	tr, err := defaultTerrain()
	if err != nil {
		t.Fatal(err)
	}
	n, err := rivers.Generate(tr, opts)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFlowIsConserved checks that every corner passes on everything that
// flows into it, and that all the rain on the land reaches an outlet.
func TestFlowIsConserved(t *testing.T) {
	n := generate(t, rivers.DefaultOptions())
	tr := n.Terrain
	inflow := make([]float64, len(tr.Corners))
	rain := make([]float64, len(tr.Corners)) // each land cell's area, shared among its corners
	var land, out float64
	for i, c := range tr.Mesh.Cells {
		if !tr.Cells[i].Ocean {
			land += c.Area()
			for _, k := range c.Corners {
				rain[k] += c.Area() / float64(len(c.Corners))
			}
		}
	}
	for k, c := range tr.Corners {
		if c.Ocean && n.Flow[k] != 0 {
			t.Fatalf("ocean corner %d: flow %g", k, n.Flow[k])
		}
		if c.Downslope >= 0 {
			inflow[c.Downslope] += n.Flow[k]
		} else {
			out += n.Flow[k]
		}
	}
	for k := range inflow {
		if want := rain[k] + inflow[k]; math.Abs(n.Flow[k]-want) > 1e-6*max(want, 1) {
			t.Fatalf("corner %d: flow %g, want rain %g + inflow %g", k, n.Flow[k], rain[k], inflow[k])
		}
	}
	if math.Abs(out-land) > land*1e-9 {
		t.Errorf("outlets receive %g, want the land area %g", out, land)
	}
}

func TestRivers(t *testing.T) {
	opts := rivers.DefaultOptions()
	n := generate(t, opts)
	tr, mesh := n.Terrain, n.Terrain.Mesh

	var count int
	for e, flow := range n.River {
		if flow == 0 {
			continue
		}
		count++
		// the river runs from the upstream end of the edge to the other
		a, b := mesh.Edges[e].Corners[0], mesh.Edges[e].Corners[1]
		if tr.Corners[a].Downslope != b {
			a, b = b, a
		}
		if tr.Corners[a].Downslope != b {
			t.Fatalf("edge %d: neither end flows to the other", e)
		}
		if flow != n.Flow[a] || flow < opts.MinFlow {
			t.Fatalf("edge %d: river flow %g, corner %d flow %g, min %g", e, flow, a, n.Flow[a], opts.MinFlow)
		}
		if l := tr.Corners[a].Lake; l >= 0 && l == tr.Corners[b].Lake {
			t.Fatalf("edge %d: river inside lake %d", e, l)
		}
		// a river carries on until it reaches an outlet or a lake
		next := tr.Corners[b]
		if next.Downslope >= 0 && next.Lake < 0 && n.River[mesh.EdgeBetween(b, next.Downslope)] == 0 {
			t.Fatalf("edge %d: river stops at corner %d", e, b)
		}
	}
	if count == 0 {
		t.Fatal("want rivers, got none")
	}

	// every edge that carries enough water outside a lake is a river
	for k, c := range tr.Corners {
		if c.Downslope < 0 || n.Flow[k] < opts.MinFlow || (c.Lake >= 0 && c.Lake == tr.Corners[c.Downslope].Lake) {
			continue
		}
		if n.River[mesh.EdgeBetween(k, c.Downslope)] == 0 {
			t.Fatalf("corner %d: flow %g but no river", k, n.Flow[k])
		}
	}

	// each lake's outlet carries at least the rain on the lake
	for l, lake := range tr.Lakes {
		var area float64
		for _, i := range lake.Cells {
			area += mesh.Cells[i].Area()
		}
		if n.Flow[lake.Outlet] < area {
			t.Errorf("lake %d: outlet flow %g, lake area %g", l, n.Flow[lake.Outlet], area)
		}
	}
	t.Logf("%d river edges", count)
}

func TestMinFlow(t *testing.T) {
	few, many := generate(t, rivers.Options{MinFlow: 20_000}), generate(t, rivers.Options{MinFlow: 1000})
	count := func(n *rivers.Network) (c int) {
		for _, f := range n.River {
			if f > 0 {
				c++
			}
		}
		return c
	}
	if count(few) >= count(many) {
		t.Errorf("min flow 20000 gives %d river edges, 1000 gives %d", count(few), count(many))
	}
}

// TestDensity checks that the rivers depend on the land, not on the number
// of cells: meshing the same height map four times as finely must give about
// the same length of river and the same largest flow.
func TestDensity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 40,000 cell mesh")
	}
	hm, err := fracture.Generate(fracture.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	measure := func(cells int) (length, largest float64) {
		opts := voronoi.DefaultOptions()
		opts.Cells = cells
		mesh, err := voronoi.Generate(hm, opts)
		if err != nil {
			t.Fatal(err)
		}
		tr, err := terrain.Generate(mesh, terrain.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		n, err := rivers.Generate(tr, rivers.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		for e, flow := range n.River {
			if flow > 0 {
				a, b := mesh.Corners[mesh.Edges[e].Corners[0]].Point, mesh.Corners[mesh.Edges[e].Corners[1]].Point
				length += math.Hypot(b.X-a.X, b.Y-a.Y)
				largest = max(largest, flow)
			}
		}
		return length, largest
	}
	coarseLength, coarseLargest := measure(10_000)
	fineLength, fineLargest := measure(40_000)
	t.Logf("10,000 cells: %.0f px of river, largest flow %.0f", coarseLength, coarseLargest)
	t.Logf("40,000 cells: %.0f px of river, largest flow %.0f", fineLength, fineLargest)
	if r := fineLength / coarseLength; r < 0.6 || r > 1.6 {
		t.Errorf("river length changes by a factor of %.2f", r)
	}
	if r := fineLargest / coarseLargest; r < 0.6 || r > 1.6 {
		t.Errorf("largest flow changes by a factor of %.2f", r)
	}
}

func TestRender(t *testing.T) {
	if *renderDir == "" {
		t.Skip("set -render to write the rivers image")
	}
	n := generate(t, rivers.DefaultOptions())
	for name, borders := range map[string]bool{"rivers.png": false, "rivers-mesh.png": true} {
		opts := render.DefaultRiversOptions()
		opts.Borders = borders
		path := filepath.Join(*renderDir, name)
		if err := render.WritePNG(path, render.Rivers(n, opts)); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := generate(t, rivers.DefaultOptions()), generate(t, rivers.DefaultOptions())
	if !slices.Equal(a.Flow, b.Flow) || !slices.Equal(a.River, b.River) {
		t.Fatal("rivers differ between runs")
	}
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	tr, err := defaultTerrain()
	if err != nil {
		t.Fatal(err)
	}
	for _, flow := range []float64{0, -1, math.NaN()} {
		if _, err := rivers.Generate(tr, rivers.Options{MinFlow: flow}); err == nil {
			t.Errorf("min flow %g: want error, got nil", flow)
		}
	}
}
