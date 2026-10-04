// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package rivers implements the fourth stage of the map pipeline. It rains
// on the land, runs the water down the terrain's downslopes, and makes a
// river of every edge that carries enough of it.
//
// Water is measured as the area of land it fell on, in square pixels, so a
// river's size does not depend on how many cells the mesh has.
//
// "Polygonal Map Generation for Games" (section 5) starts rivers at random
// corners in the mountains. Rain everywhere instead lets the shape of the
// height map decide where the rivers run.
package rivers

import (
	"fmt"

	"github.com/maloquacious/wg/terrain"
)

// Options configures the rivers.
type Options struct {
	// MinFlow is the least flow, in square pixels of land drained, that an
	// edge must carry to be a river.
	MinFlow float64
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{MinFlow: 2000}
}

// Network holds the flow of water over a terrain.
type Network struct {
	Terrain *terrain.Terrain
	// Flow is the water passing through each corner: its own rain plus
	// everything that flows into it. It is 0 for ocean corners.
	Flow []float64
	// River is the flow along each edge of the mesh that is a river, and 0
	// for every other edge.
	River []float64
}

// Generate runs water over t. The same terrain and options always produce
// the same network.
func Generate(t *terrain.Terrain, opts Options) (*Network, error) {
	if !(opts.MinFlow > 0) {
		return nil, fmt.Errorf("rivers: invalid min flow %g", opts.MinFlow)
	}
	mesh := t.Mesh
	n := &Network{
		Terrain: t,
		Flow:    make([]float64, len(t.Corners)),
		River:   make([]float64, len(mesh.Edges)),
	}

	// each land cell shares its area equally among its corners
	for i, c := range mesh.Cells {
		if t.Cells[i].Ocean {
			continue
		}
		share := c.Area() / float64(len(c.Corners))
		for _, k := range c.Corners {
			n.Flow[k] += share
		}
	}

	// count the corners that flow into each corner, then pass the water
	// down from the corners that nothing flows into
	upstream := make([]int, len(t.Corners))
	for _, c := range t.Corners {
		if c.Downslope >= 0 {
			upstream[c.Downslope]++
		}
	}
	var ready []int
	for k := range t.Corners {
		if upstream[k] == 0 {
			ready = append(ready, k)
		}
	}
	for len(ready) > 0 {
		k := ready[0]
		ready = ready[1:]
		d := t.Corners[k].Downslope
		if d < 0 {
			continue
		}
		n.Flow[d] += n.Flow[k]
		if upstream[d]--; upstream[d] == 0 {
			ready = append(ready, d)
		}

		// water inside a lake is the lake, not a river
		inLake := t.Corners[k].Lake >= 0 && t.Corners[k].Lake == t.Corners[d].Lake
		if n.Flow[k] >= opts.MinFlow && !inLake {
			n.River[mesh.EdgeBetween(k, d)] = n.Flow[k]
		}
	}
	return n, nil
}
