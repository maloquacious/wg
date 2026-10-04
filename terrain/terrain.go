// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package terrain implements the third stage of the map pipeline. It gives
// the corners of a Voronoi mesh elevations, fills the pits so that every
// corner drains to the ocean or off the edge of the map, turns the deeper
// pits into lakes, and points every corner downhill.
//
// It follows sections 3 and 4 of "Polygonal Map Generation for Games", except
// that the elevations come from the height map rather than from the distance
// to the coast.
package terrain

import (
	"cmp"
	"container/heap"
	"fmt"
	"slices"

	"github.com/maloquacious/wg/voronoi"
)

// Options configures the terrain.
type Options struct {
	// LakeDepth is how far below its spill point a pit must reach to become
	// a lake. Shallower pits are filled and stay land.
	LakeDepth float64
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{LakeDepth: 0.01}
}

// epsilon is the rise added to every filled corner so that filled pits slope
// down toward their spill point instead of lying flat.
const epsilon = 1e-9

// Cell describes one cell of the mesh.
type Cell struct {
	// Elevation is the cell's elevation from the mesh, or the lake level for
	// a lake cell.
	Elevation float64
	// Ocean is true when the cell is below sea level.
	Ocean bool
	// Coast is true for a land cell that has an ocean neighbor.
	Coast bool
	// Lake is the index of the lake that covers the cell, or -1.
	Lake int
}

// Corner describes one corner of the mesh.
type Corner struct {
	// Elevation is the mean elevation of the cells that meet at the corner,
	// raised where it lay in a pit. Every lake corner is at the lake level.
	Elevation float64
	// Ocean is true when every cell at the corner is ocean.
	Ocean bool
	// Coast is true when the corner touches both ocean and land cells.
	Coast bool
	// Lake is the index of the lake the corner lies in, or -1.
	Lake int
	// Downslope is the adjacent corner that water flows to, or -1 for an
	// outlet: a corner that touches the ocean or lies on the edge of the map.
	Downslope int
	// Outlet is the outlet that water from this corner reaches. Corners
	// with the same outlet form a watershed.
	Outlet int
}

// Lake is a pit that was filled with water.
type Lake struct {
	// Level is the elevation of the lake surface: that of its outlet.
	Level float64
	// Depth is how far the lowest corner of the pit lay below Level.
	Depth float64
	// Outlet is the corner outside the lake that the lake drains through.
	Outlet int
	// Cells and Corners hold the indices of the cells and corners that the
	// lake covers, sorted.
	Cells, Corners []int
}

// Terrain annotates a mesh. Cells and Corners are indexed like the mesh's.
type Terrain struct {
	Mesh    *voronoi.Mesh
	Cells   []Cell
	Corners []Corner
	Lakes   []Lake
}

// Generate builds the terrain for mesh. The same mesh and options always
// produce the same terrain.
func Generate(mesh *voronoi.Mesh, opts Options) (*Terrain, error) {
	if opts.LakeDepth < 0 {
		return nil, fmt.Errorf("terrain: invalid lake depth %g", opts.LakeDepth)
	}
	t := &Terrain{
		Mesh:    mesh,
		Cells:   make([]Cell, len(mesh.Cells)),
		Corners: make([]Corner, len(mesh.Corners)),
	}
	for i, c := range mesh.Cells {
		t.Cells[i] = Cell{Elevation: c.Elevation, Ocean: c.Ocean, Lake: -1}
		if !c.Ocean {
			t.Cells[i].Coast = slices.ContainsFunc(c.Neighbors, func(j int) bool { return mesh.Cells[j].Ocean })
		}
	}

	raw := make([]float64, len(mesh.Corners))
	var outlets []int
	for k, corner := range mesh.Corners {
		var sum float64
		var ocean int
		for _, i := range corner.Touches {
			sum += mesh.Cells[i].Elevation
			if mesh.Cells[i].Ocean {
				ocean++
			}
		}
		raw[k] = sum / float64(len(corner.Touches))
		t.Corners[k] = Corner{
			Ocean:     ocean == len(corner.Touches),
			Coast:     ocean > 0 && ocean < len(corner.Touches),
			Lake:      -1,
			Downslope: -1,
			Outlet:    k,
		}
		if ocean > 0 || corner.Border {
			outlets = append(outlets, k)
		}
	}

	filled, parent, order := fill(mesh, raw, outlets)
	for _, k := range order {
		t.Corners[k].Elevation = filled[k]
		if parent[k] < 0 {
			continue
		}
		// flow to the lowest neighbor; the corner the flood came from is
		// lower, so there is always one
		low := parent[k]
		for _, j := range mesh.Corners[k].Adjacent {
			if filled[j] < filled[low] || (filled[j] == filled[low] && j < low) {
				low = j
			}
		}
		t.Corners[k].Downslope = low
		t.Corners[k].Outlet = t.Corners[low].Outlet
	}

	t.makeLakes(raw, filled, parent, order, opts.LakeDepth)
	return t, nil
}

// fill floods the corners upward from the outlets in order of elevation
// ("Priority-Flood", Barnes, Lehman and Mulla, 2014). A corner reached from a
// higher one lies in a pit and is raised to just above it. fill returns the
// raised elevations, the corner each corner was reached from (-1 for an
// outlet), and the corners in the order they were reached.
func fill(mesh *voronoi.Mesh, raw []float64, outlets []int) (filled []float64, parent, order []int) {
	filled = slices.Clone(raw)
	parent = make([]int, len(raw))
	reached := make([]bool, len(raw))
	q := &queue{elevation: filled}
	for _, k := range outlets {
		parent[k] = -1
		reached[k] = true
		heap.Push(q, k)
	}
	for q.Len() > 0 {
		k := heap.Pop(q).(int)
		order = append(order, k)
		for _, j := range mesh.Corners[k].Adjacent {
			if reached[j] {
				continue
			}
			reached[j] = true
			parent[j] = k
			if filled[j] <= filled[k] {
				filled[j] = filled[k] + epsilon
			}
			heap.Push(q, j)
		}
	}
	return filled, parent, order
}

// makeLakes groups the raised corners into pits and turns each pit that is
// at least depth deep, and covers at least one whole cell, into a lake.
func (t *Terrain) makeLakes(raw, filled []float64, parent, order []int, depth float64) {
	mesh := t.Mesh
	raised := func(k int) bool { return filled[k] > raw[k] }
	pit := make([]int, len(raw))
	for k := range pit {
		pit[k] = -1
	}

	// walking the corners in flood order, the first corner met in each pit
	// was reached from the pit's spill point
	var pits [][]int
	for _, k := range order {
		if !raised(k) || pit[k] >= 0 {
			continue
		}
		n := len(pits)
		pit[k] = n
		corners := []int{k}
		for s := 0; s < len(corners); s++ {
			for _, j := range mesh.Corners[corners[s]].Adjacent {
				if raised(j) && pit[j] < 0 {
					pit[j] = n
					corners = append(corners, j)
				}
			}
		}
		pits = append(pits, corners)
	}

	for n, corners := range pits {
		outlet := parent[corners[0]]
		level, lowest := filled[outlet], raw[corners[0]]
		for _, k := range corners {
			lowest = min(lowest, raw[k])
		}
		if level-lowest < depth {
			continue
		}
		var cells []int
		for _, k := range corners {
			for _, i := range mesh.Corners[k].Touches {
				if !slices.Contains(cells, i) && !slices.ContainsFunc(mesh.Cells[i].Corners, func(c int) bool { return pit[c] != n }) {
					cells = append(cells, i)
				}
			}
		}
		if len(cells) == 0 {
			continue
		}

		lake := len(t.Lakes)
		slices.Sort(cells)
		slices.Sort(corners)
		for _, i := range cells {
			t.Cells[i].Elevation = level
			t.Cells[i].Lake = lake
		}
		for _, k := range corners {
			t.Corners[k].Elevation = level
			t.Corners[k].Lake = lake
		}
		t.Lakes = append(t.Lakes, Lake{Level: level, Depth: level - lowest, Outlet: outlet, Cells: cells, Corners: corners})
	}
}

// queue is a min-heap of corners ordered by elevation, then by index.
type queue struct {
	corners   []int
	elevation []float64
}

func (q *queue) Len() int { return len(q.corners) }
func (q *queue) Less(a, b int) bool {
	ka, kb := q.corners[a], q.corners[b]
	return cmp.Or(cmp.Compare(q.elevation[ka], q.elevation[kb]), cmp.Compare(ka, kb)) < 0
}
func (q *queue) Swap(a, b int) { q.corners[a], q.corners[b] = q.corners[b], q.corners[a] }
func (q *queue) Push(x any)    { q.corners = append(q.corners, x.(int)) }
func (q *queue) Pop() any {
	k := q.corners[len(q.corners)-1]
	q.corners = q.corners[:len(q.corners)-1]
	return k
}
