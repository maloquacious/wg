// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package moisture implements the fifth stage of the map pipeline. It makes
// the land wetter the closer it lies to fresh water, then spreads the values
// evenly over 0...1, following section 6 of "Polygonal Map Generation for
// Games" and the original mapgen2 Map.as:
//
//   - Rivers and lakes are the sources. A lake corner has strength 1 and a
//     river corner has strength flow/MinFlow, up to 3, so large rivers wet
//     more land than small ones.
//   - The sea is a weaker source: every coast corner has strength
//     SeaStrength. Without it, an island with no rivers or lakes would be
//     the driest land on the map. mapgen2 has no sea source.
//   - Wetness falls by a factor of e every Spread pixels along the corner
//     graph, so a corner's wetness is strength·exp(-distance/Spread) from
//     its best source. It spreads over land and coast corners only, never
//     across the sea.
//   - Land that no source reaches is the driest.
//   - The land corners are ranked by wetness and given evenly spaced values
//     from 0 (driest) to 1 (wettest). Ocean and coast corners are 1.
//   - A cell's moisture is the mean of its corners.
//
// Distances are in pixels, so the result does not depend on the cell size.
package moisture

import (
	"cmp"
	"container/heap"
	"fmt"
	"math"
	"slices"

	"github.com/maloquacious/wg/rivers"
)

// Options configures the moisture.
type Options struct {
	// Spread is the distance in pixels over which wetness falls by a factor
	// of e. Ranking makes the land's moisture depend only on its order, so
	// Spread matters only in weighing large rivers against near ones.
	Spread float64
	// SeaStrength is the strength of the coast as a source, where a lake
	// is 1. Zero turns the sea off as a source.
	SeaStrength float64
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{Spread: 100, SeaStrength: 2}
}

// maxStrength caps how much wetter than a lake the largest rivers are.
const maxStrength = 3

// Map holds the moisture of every corner and cell, in 0...1.
type Map struct {
	Rivers  *rivers.Network
	Corners []float64
	Cells   []float64
}

// Generate computes the moisture for n. The same network and options always
// produce the same map.
func Generate(n *rivers.Network, opts Options) (*Map, error) {
	if !(opts.Spread > 0) || math.IsInf(opts.Spread, 1) {
		return nil, fmt.Errorf("moisture: invalid spread %g", opts.Spread)
	}
	if !(opts.SeaStrength >= 0) || opts.SeaStrength > maxStrength {
		return nil, fmt.Errorf("moisture: invalid sea strength %g", opts.SeaStrength)
	}
	t := n.Terrain
	mesh := t.Mesh

	// a corner's dryness is Spread·ln(1/wetness): its distance from its best
	// source, less Spread·ln(strength) of that source
	dryness := make([]float64, len(t.Corners))
	for k := range dryness {
		dryness[k] = math.Inf(1)
	}
	q := &queue{}
	for k, c := range t.Corners {
		strength := 0.0
		if c.Coast {
			strength = opts.SeaStrength
		}
		if c.Lake >= 0 {
			strength = max(strength, 1)
		}
		if slices.ContainsFunc(mesh.Corners[k].Edges, func(e int) bool { return n.River[e] > 0 }) {
			strength = max(strength, min(maxStrength, n.Flow[k]/n.MinFlow))
		}
		if strength > 0 && !c.Ocean {
			dryness[k] = -opts.Spread * math.Log(strength)
			heap.Push(q, entry{k, dryness[k]})
		}
	}
	done := make([]bool, len(t.Corners))
	for q.Len() > 0 {
		k := heap.Pop(q).(entry).corner
		if done[k] {
			continue
		}
		done[k] = true
		a := mesh.Corners[k].Point
		for _, j := range mesh.Corners[k].Adjacent {
			if t.Corners[j].Ocean {
				continue
			}
			b := mesh.Corners[j].Point
			if d := dryness[k] + math.Hypot(b.X-a.X, b.Y-a.Y); d < dryness[j] {
				dryness[j] = d
				heap.Push(q, entry{j, d})
			}
		}
	}

	m := &Map{
		Rivers:  n,
		Corners: make([]float64, len(t.Corners)),
		Cells:   make([]float64, len(mesh.Cells)),
	}
	var land []int
	for k, c := range t.Corners {
		if c.Ocean || c.Coast {
			m.Corners[k] = 1
		} else {
			land = append(land, k)
		}
	}
	// wettest first; corners that no fresh water reaches tie at +Inf and
	// fall back on their index
	slices.SortFunc(land, func(a, b int) int {
		return cmp.Or(cmp.Compare(dryness[a], dryness[b]), cmp.Compare(a, b))
	})
	for rank, k := range land {
		m.Corners[k] = 1
		if len(land) > 1 {
			m.Corners[k] = 1 - float64(rank)/float64(len(land)-1)
		}
	}

	for i, c := range mesh.Cells {
		var sum float64
		for _, k := range c.Corners {
			sum += m.Corners[k]
		}
		m.Cells[i] = sum / float64(len(c.Corners))
	}
	return m, nil
}

// entry is a corner queued at the dryness it had when it was pushed.
type entry struct {
	corner  int
	dryness float64
}

// queue is a min-heap of entries ordered by dryness, then by corner. A corner
// may be pushed more than once; only its first pop counts.
type queue []entry

func (q queue) Len() int { return len(q) }
func (q queue) Less(a, b int) bool {
	return cmp.Or(cmp.Compare(q[a].dryness, q[b].dryness), cmp.Compare(q[a].corner, q[b].corner)) < 0
}
func (q queue) Swap(a, b int) { q[a], q[b] = q[b], q[a] }
func (q *queue) Push(x any)   { *q = append(*q, x.(entry)) }
func (q *queue) Pop() any {
	e := (*q)[len(*q)-1]
	*q = (*q)[:len(*q)-1]
	return e
}
