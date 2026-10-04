// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package moisture_test

import (
	"flag"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/moisture"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/rivers"
	"github.com/maloquacious/wg/terrain"
	"github.com/maloquacious/wg/voronoi"
)

// renderDir enables writing the test moisture as an image, for example
//
//	go test ./moisture -render=../var
var renderDir = flag.String("render", "", "write rendered moisture to this directory")

// defaultNetwork is the stage 4 river network built from the default options.
var defaultNetwork = sync.OnceValues(func() (*rivers.Network, error) {
	hm, err := fracture.Generate(fracture.DefaultOptions())
	if err != nil {
		return nil, err
	}
	mesh, err := voronoi.Generate(hm, voronoi.DefaultOptions())
	if err != nil {
		return nil, err
	}
	tr, err := terrain.Generate(mesh, terrain.DefaultOptions())
	if err != nil {
		return nil, err
	}
	return rivers.Generate(tr, rivers.DefaultOptions())
})

func generate(t *testing.T, opts moisture.Options) *moisture.Map {
	t.Helper()
	n, err := defaultNetwork()
	if err != nil {
		t.Fatal(err)
	}
	m, err := moisture.Generate(n, opts)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// freshWater reports whether corner k is on a river or in a lake.
func freshWater(n *rivers.Network, k int) bool {
	mesh := n.Terrain.Mesh
	return n.Terrain.Corners[k].Lake >= 0 || slices.ContainsFunc(mesh.Corners[k].Edges, func(e int) bool { return n.River[e] > 0 })
}

// TestEvenlySpread checks that ocean and coast corners are 1 and that the
// land corners take evenly spaced values from 0 to 1.
func TestEvenlySpread(t *testing.T) {
	checkSpread(t, generate(t, moisture.DefaultOptions()), 1)
}

// TestSkew checks that the land corners take evenly spaced values raised to
// the power Skew, that the order of the land does not change, and that a
// larger skew leaves more of the land in the desert range.
func TestSkew(t *testing.T) {
	even := generate(t, moisture.DefaultOptions())
	prevDry := -1.0
	for _, skew := range []float64{0.5, 1, 2, 3} {
		opts := moisture.DefaultOptions()
		opts.Skew = skew
		m := generate(t, opts)
		checkSpread(t, m, skew)

		var dry, land float64
		for k, c := range m.Rivers.Terrain.Corners {
			if c.Ocean || c.Coast {
				continue
			}
			land++
			if m.Corners[k] < 0.16 {
				dry++
			}
			// the skew is a monotone map of the even values
			if want := math.Pow(even.Corners[k], skew); math.Abs(m.Corners[k]-want) > 1e-12 {
				t.Fatalf("skew %g: corner %d moisture %g, want %g", skew, k, m.Corners[k], want)
			}
		}
		if want := math.Pow(0.16, 1/skew); math.Abs(dry/land-want) > 0.01 {
			t.Errorf("skew %g: %.3f of land below 0.16, want %.3f", skew, dry/land, want)
		}
		if dry/land <= prevDry {
			t.Errorf("skew %g: %.3f of land below 0.16, not more than at the smaller skew", skew, dry/land)
		}
		t.Logf("skew %.1f: %4.1f%% of land below 0.16", skew, 100*dry/land)
		prevDry = dry / land
	}
}

func checkSpread(t *testing.T, m *moisture.Map, skew float64) {
	t.Helper()
	tr := m.Rivers.Terrain
	var land []float64
	for k, c := range tr.Corners {
		if c.Ocean || c.Coast {
			if m.Corners[k] != 1 {
				t.Fatalf("corner %d: ocean or coast with moisture %g", k, m.Corners[k])
			}
			continue
		}
		land = append(land, m.Corners[k])
	}
	slices.Sort(land)
	for rank, v := range land {
		if want := math.Pow(float64(rank)/float64(len(land)-1), skew); math.Abs(v-want) > 1e-12 {
			t.Fatalf("rank %d of %d: moisture %g, want %g", rank, len(land), v, want)
		}
	}
}

// TestNoDryHollows checks that wetness only spreads out from its sources,
// over land: every land corner that a source reaches without crossing the
// sea, and is not itself a source, has a wetter neighbor; and every corner
// that no source can reach is drier than all that one can. With the sea
// turned off, only fresh water is a source.
func TestNoDryHollows(t *testing.T) {
	for _, opts := range []moisture.Options{moisture.DefaultOptions(), {Spread: 100, SeaStrength: 0, Skew: 1}} {
		t.Run(fmt.Sprintf("sea %g", opts.SeaStrength), func(t *testing.T) {
			checkNoDryHollows(t, generate(t, opts), opts.SeaStrength > 0)
		})
	}
}

func checkNoDryHollows(t *testing.T, m *moisture.Map, sea bool) {
	n := m.Rivers
	tr, mesh := n.Terrain, n.Terrain.Mesh
	source := func(k int) bool { return freshWater(n, k) || (sea && tr.Corners[k].Coast) }

	// the corners that a source reaches over land and coast corners
	reached := make([]bool, len(tr.Corners))
	var stack []int
	for k := range tr.Corners {
		if !tr.Corners[k].Ocean && source(k) {
			reached[k] = true
			stack = append(stack, k)
		}
	}
	for len(stack) > 0 {
		k := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, j := range mesh.Corners[k].Adjacent {
			if !reached[j] && !tr.Corners[j].Ocean {
				reached[j] = true
				stack = append(stack, j)
			}
		}
	}

	var fresh, other []float64
	wettestDry, driestReached := -1.0, 2.0
	for k, c := range tr.Corners {
		if c.Ocean || c.Coast {
			continue
		}
		if !reached[k] {
			wettestDry = max(wettestDry, m.Corners[k])
			continue
		}
		driestReached = min(driestReached, m.Corners[k])
		if freshWater(n, k) {
			fresh = append(fresh, m.Corners[k])
			continue
		}
		other = append(other, m.Corners[k])
		wetter := slices.ContainsFunc(mesh.Corners[k].Adjacent, func(j int) bool {
			return !tr.Corners[j].Ocean && m.Corners[j] > m.Corners[k]
		})
		if !wetter {
			t.Fatalf("corner %d: moisture %g, but no neighbor is wetter", k, m.Corners[k])
		}
	}
	if wettestDry >= driestReached {
		t.Errorf("land fresh water cannot reach has moisture up to %g, above reached land at %g", wettestDry, driestReached)
	}
	if wettestDry < 0 {
		t.Log("every land corner is reached")
	} else {
		t.Logf("unreached land: moisture up to %.3f", wettestDry)
	}
	mean := func(v []float64) (s float64) {
		for _, x := range v {
			s += x
		}
		return s / float64(len(v))
	}
	// land beside a large river ranks wetter than the bank of a small creek
	// or lake, so fresh water is not always wettest, but it is on average
	if mean(fresh) < mean(other)+0.2 {
		t.Errorf("mean moisture: fresh water %.2f, elsewhere %.2f", mean(fresh), mean(other))
	}
	t.Logf("mean moisture: fresh water %.2f (%d corners), elsewhere %.2f (%d corners)", mean(fresh), len(fresh), mean(other), len(other))
}

// TestCells checks that a cell's moisture is the mean of its corners.
func TestCells(t *testing.T) {
	m := generate(t, moisture.DefaultOptions())
	mesh := m.Rivers.Terrain.Mesh
	for i, c := range mesh.Cells {
		var sum float64
		for _, k := range c.Corners {
			sum += m.Corners[k]
		}
		if want := sum / float64(len(c.Corners)); math.Abs(m.Cells[i]-want) > 1e-12 {
			t.Fatalf("cell %d: moisture %g, want %g", i, m.Cells[i], want)
		}
		if m.Rivers.Terrain.Cells[i].Ocean && m.Cells[i] != 1 {
			t.Fatalf("ocean cell %d: moisture %g", i, m.Cells[i])
		}
	}
}

// TestBigRiversReachFurther checks that Spread weighs large rivers: with a
// longer Spread, the corners near the largest rivers rank wetter.
func TestBigRiversReachFurther(t *testing.T) {
	short, long := generate(t, moisture.Options{Spread: 10, SeaStrength: 2, Skew: 1}), generate(t, moisture.Options{Spread: 1000, SeaStrength: 2, Skew: 1})
	n := short.Rivers
	mesh := n.Terrain.Mesh

	// the land corners next to the river edges that carry the most water
	largest := slices.Max(n.River)
	var near []int
	for e, flow := range n.River {
		if flow < largest/2 {
			continue
		}
		for _, k := range mesh.Edges[e].Corners {
			for _, j := range mesh.Corners[k].Adjacent {
				c := n.Terrain.Corners[j]
				if !c.Ocean && !c.Coast && !freshWater(n, j) {
					near = append(near, j)
				}
			}
		}
	}
	if len(near) == 0 {
		t.Fatal("no land corners near the largest rivers")
	}
	var sumShort, sumLong float64
	for _, k := range near {
		sumShort += short.Corners[k]
		sumLong += long.Corners[k]
	}
	if sumLong <= sumShort {
		t.Errorf("near the largest rivers: mean moisture %.3f with spread 1000, %.3f with spread 10",
			sumLong/float64(len(near)), sumShort/float64(len(near)))
	}
}

// TestSeaWetsTheCoast checks that the sea makes the land just behind the
// coast wetter and leaves the driest land inland.
func TestSeaWetsTheCoast(t *testing.T) {
	without := generate(t, moisture.Options{Spread: 100, SeaStrength: 0, Skew: 1})
	with := generate(t, moisture.DefaultOptions())
	tr, mesh := with.Rivers.Terrain, with.Rivers.Terrain.Mesh

	// steps from each cell to the nearest ocean cell
	steps := make([]int, len(mesh.Cells))
	var queue []int
	for i := range steps {
		steps[i] = -1
		if tr.Cells[i].Ocean {
			steps[i] = 0
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, j := range mesh.Cells[i].Neighbors {
			if steps[j] < 0 {
				steps[j] = steps[i] + 1
				queue = append(queue, j)
			}
		}
	}
	mean := func(m *moisture.Map, near bool) float64 {
		var sum float64
		var n int
		for i, s := range steps {
			if (near && s == 2) || (!near && s >= 5) {
				sum += m.Cells[i]
				n++
			}
		}
		return sum / float64(n)
	}
	t.Logf("behind the coast: %.2f without the sea, %.2f with it", mean(without, true), mean(with, true))
	t.Logf("deep inland:      %.2f without the sea, %.2f with it", mean(without, false), mean(with, false))
	if mean(with, true) <= mean(without, true)+0.1 {
		t.Errorf("the sea does not wet the land behind the coast")
	}
	if mean(with, false) >= mean(with, true) {
		t.Errorf("deep inland is not drier than behind the coast")
	}
}

func TestRender(t *testing.T) {
	if *renderDir == "" {
		t.Skip("set -render to write the moisture image")
	}
	path := filepath.Join(*renderDir, "moisture.png")
	if err := render.WritePNG(path, render.Moisture(generate(t, moisture.DefaultOptions()), render.DefaultMoistureOptions())); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := generate(t, moisture.DefaultOptions()), generate(t, moisture.DefaultOptions())
	if !slices.Equal(a.Corners, b.Corners) || !slices.Equal(a.Cells, b.Cells) {
		t.Fatal("moisture differs between runs")
	}
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	n, err := defaultNetwork()
	if err != nil {
		t.Fatal(err)
	}
	for _, spread := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := moisture.Generate(n, moisture.Options{Spread: spread, Skew: 1}); err == nil {
			t.Errorf("spread %g: want error, got nil", spread)
		}
	}
	for _, skew := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := moisture.Generate(n, moisture.Options{Spread: 100, SeaStrength: 2, Skew: skew}); err == nil {
			t.Errorf("skew %g: want error, got nil", skew)
		}
	}
	for _, sea := range []float64{-1, 3.5, math.NaN()} {
		if _, err := moisture.Generate(n, moisture.Options{Spread: 100, SeaStrength: sea, Skew: 1}); err == nil {
			t.Errorf("sea strength %g: want error, got nil", sea)
		}
	}
}
