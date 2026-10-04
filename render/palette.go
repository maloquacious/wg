// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import "image/color"

// palette holds the discrete colors of the Red Blob Games mapgen2 renderer,
// copied from resources/biome-colors.js. TestPaletteMatchesResource keeps
// the two in step.
//
// From http://www.redblobgames.com/maps/mapgen2/
// Copyright 2017 Red Blob Games <redblobgames@gmail.com>
// License: Apache v2.0 <http://www.apache.org/licenses/LICENSE-2.0.html>
var palette = map[string]color.RGBA{
	"OCEAN":                      rgb(0x44447a),
	"COAST":                      rgb(0x33335a),
	"LAKESHORE":                  rgb(0x225588),
	"LAKE":                       rgb(0x336699),
	"RIVER":                      rgb(0x225588),
	"MARSH":                      rgb(0x2f6666),
	"ICE":                        rgb(0x99ffff),
	"BEACH":                      rgb(0xa09077),
	"SNOW":                       rgb(0xffffff),
	"TUNDRA":                     rgb(0xbbbbaa),
	"BARE":                       rgb(0x888888),
	"SCORCHED":                   rgb(0x555555),
	"TAIGA":                      rgb(0x99aa77),
	"SHRUBLAND":                  rgb(0x889977),
	"TEMPERATE_DESERT":           rgb(0xc9d29b),
	"TEMPERATE_RAIN_FOREST":      rgb(0x448855),
	"TEMPERATE_DECIDUOUS_FOREST": rgb(0x679459),
	"GRASSLAND":                  rgb(0x88aa55),
	"SUBTROPICAL_DESERT":         rgb(0xd2b98b),
	"TROPICAL_RAIN_FOREST":       rgb(0x337755),
	"TROPICAL_SEASONAL_FOREST":   rgb(0x559944),
}

// ownColors holds the colors of the biomes that mapgen2 does not have.
var ownColors = map[string]color.RGBA{
	"ROCKY_SHORE": rgb(0x8a8478),
	"CLIFF":       rgb(0x6b6e6a),
}

func rgb(hex uint32) color.RGBA {
	return color.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}
