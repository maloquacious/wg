// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package rivers_test

import (
	"flag"
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

// TestFlowIsConserved checks that every corner passes on its own rain plus
// all the water that flows into it, and that all the rain reaches an outlet.
func TestFlowIsConserved(t *testing.T) {
	n := generate(t, rivers.DefaultOptions())
	tr := n.Terrain
	want := make([]int, len(tr.Corners))
	var rain, out int
	for k, c := range tr.Corners {
		if !c.Ocean {
			want[k]++
			rain++
		}
		if c.Downslope >= 0 {
			want[c.Downslope] += n.Flow[k]
		} else {
			out += n.Flow[k]
		}
	}
	for k := range want {
		if n.Flow[k] != want[k] {
			t.Fatalf("corner %d: flow %d, want %d", k, n.Flow[k], want[k])
		}
	}
	if out != rain {
		t.Errorf("outlets receive %d, want all %d units of rain", out, rain)
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
			t.Fatalf("edge %d: river flow %d, corner %d flow %d, min %d", e, flow, a, n.Flow[a], opts.MinFlow)
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
			t.Fatalf("corner %d: flow %d but no river", k, n.Flow[k])
		}
	}

	// each lake's outlet carries at least the lake's own rain
	for l, lake := range tr.Lakes {
		if n.Flow[lake.Outlet] <= len(lake.Corners) {
			t.Errorf("lake %d: outlet flow %d, lake has %d corners", l, n.Flow[lake.Outlet], len(lake.Corners))
		}
	}
	t.Logf("%d river edges", count)
}

func TestMinFlow(t *testing.T) {
	few, many := generate(t, rivers.Options{MinFlow: 200}), generate(t, rivers.Options{MinFlow: 10})
	count := func(n *rivers.Network) (c int) {
		for _, f := range n.River {
			if f > 0 {
				c++
			}
		}
		return c
	}
	if count(few) >= count(many) {
		t.Errorf("min flow 200 gives %d river edges, 10 gives %d", count(few), count(many))
	}
}

func TestRender(t *testing.T) {
	if *renderDir == "" {
		t.Skip("set -render to write the rivers image")
	}
	path := filepath.Join(*renderDir, "rivers.png")
	img := render.Rivers(generate(t, rivers.DefaultOptions()), render.DefaultRiversOptions())
	if err := render.WritePNG(path, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
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
	if _, err := rivers.Generate(tr, rivers.Options{MinFlow: 0}); err == nil {
		t.Error("min flow 0: want error, got nil")
	}
}
