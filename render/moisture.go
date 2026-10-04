// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"

	"github.com/maloquacious/wg/moisture"
)

// MoistureOptions configures Moisture.
type MoistureOptions struct {
	// Scale sets the width of the rivers drawn on top, as in RiversOptions.
	Scale float64
}

// DefaultMoistureOptions returns the options used by the tests.
func DefaultMoistureOptions() MoistureOptions {
	return MoistureOptions{Scale: DefaultRiversOptions().Scale}
}

// The moisture ramp runs from the SUBTROPICAL_DESERT to the
// TROPICAL_RAIN_FOREST colors of the Red Blob Games mapgen2 palette.
var (
	dryColor = palette["SUBTROPICAL_DESERT"]
	wetColor = palette["TROPICAL_RAIN_FOREST"]
)

// Moisture renders the moisture of the land cells on a ramp from dry to wet,
// with the ocean tinted by depth as in Mesh, lakes, and the rivers on top.
func Moisture(m *moisture.Map, opts MoistureOptions) *image.RGBA {
	t := m.Rivers.Terrain
	mesh := t.Mesh
	colors := cellColors(mesh)
	for i, c := range t.Cells {
		switch {
		case c.Lake >= 0:
			colors[i] = lakeColor
		case !c.Ocean:
			colors[i] = blend(dryColor, wetColor, m.Cells[i])
		}
	}
	owner := owners(mesh)
	img := paint(owner, colors, mesh.Width, mesh.Height)
	drawRivers(img, m.Rivers, opts.Scale)
	return img
}
