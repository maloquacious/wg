// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"

	"github.com/maloquacious/wg/biomes"
)

// BiomesOptions configures Biomes.
type BiomesOptions struct {
	// Scale sets the width of the rivers, as in RiversOptions.
	Scale float64
	// Borders draws the outline of every cell.
	Borders bool
	// Icons draws a hand-drawn Red Blob Games icon in each cell, as
	// mapgen2 does: waves, mountains, trees, grass and so on. Credit Red
	// Blob Games wherever the icons appear.
	Icons bool
	// Seed chooses among the drawings of each icon.
	Seed uint64
}

// DefaultBiomesOptions returns the options used by the tests.
func DefaultBiomesOptions() BiomesOptions {
	return BiomesOptions{Scale: DefaultRiversOptions().Scale, Seed: 0x0123456789abcdef}
}

// BiomeColor returns the color of b: mapgen2's, or ours for the biomes
// that mapgen2 does not have.
func BiomeColor(b biomes.Biome) color.RGBA {
	if c, ok := ownColors[b.String()]; ok {
		return c
	}
	return palette[b.String()]
}

// Biomes renders every cell in the mapgen2 color of its biome, outlines the
// coast and the lake shores in mapgen2's COAST and LAKESHORE colors, and
// draws the rivers on top. With opts.Icons, it draws the icons over all of
// that.
func Biomes(b *biomes.Map, opts BiomesOptions) *image.RGBA {
	t := b.Moisture.Rivers.Terrain
	mesh := t.Mesh
	width, height := mesh.Width, mesh.Height
	colors := make([]color.RGBA, len(b.Cells))
	for i, biome := range b.Cells {
		colors[i] = BiomeColor(biome)
	}
	owner := owners(mesh)
	img := paint(owner, colors, width, height)

	coast, lakeshore := palette["COAST"], palette["LAKESHORE"]
	border := color.RGBA{R: 50, G: 40, B: 30, A: 255}
	for y := range height {
		for x := range width {
			i := owner[y*width+x]
			for _, d := range [][2]int{{1, 0}, {0, 1}} {
				px, py := x+d[0], y+d[1]
				if px >= width || py >= height {
					continue
				}
				j := owner[py*width+px]
				if i == j {
					continue
				}
				if t.Cells[i].Ocean != t.Cells[j].Ocean {
					img.SetRGBA(x, y, coast)
					break
				}
				if (t.Cells[i].Lake >= 0) != (t.Cells[j].Lake >= 0) {
					img.SetRGBA(x, y, lakeshore)
					break
				}
				if opts.Borders {
					img.SetRGBA(x, y, blend(img.RGBAAt(x, y), border, 0.2))
					break
				}
			}
		}
	}
	drawRivers(img, b.Moisture.Rivers, opts.Scale)
	if opts.Icons {
		drawIcons(img, b, opts.Seed)
	}
	return img
}
