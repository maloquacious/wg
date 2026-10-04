// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package rivers implements the fourth stage of the map pipeline. It rains
// one unit of water on every corner that is not ocean, runs the water down
// the terrain's downslopes, and makes a river of every edge that carries
// enough of it.
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
	// MinFlow is the least flow, in corners' worth of rain, that an edge
	// must carry to be a river.
	MinFlow int
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{MinFlow: 20}
}

// Network holds the flow of water over a terrain.
type Network struct {
	Terrain *terrain.Terrain
	// Flow is the water passing through each corner: its own rain plus
	// everything that flows into it. It is 0 for ocean corners.
	Flow []int
	// River is the flow along each edge of the mesh that is a river, and 0
	// for every other edge.
	River []int
}

// Generate runs water over t. The same terrain and options always produce
// the same network.
func Generate(t *terrain.Terrain, opts Options) (*Network, error) {
	if opts.MinFlow < 1 {
		return nil, fmt.Errorf("rivers: invalid min flow %d", opts.MinFlow)
	}
	mesh := t.Mesh
	n := &Network{
		Terrain: t,
		Flow:    make([]int, len(t.Corners)),
		River:   make([]int, len(mesh.Edges)),
	}

	// count the corners that flow into each corner, then pass the water
	// down from the corners that nothing flows into
	upstream := make([]int, len(t.Corners))
	for k, c := range t.Corners {
		if !c.Ocean {
			n.Flow[k] = 1
		}
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
