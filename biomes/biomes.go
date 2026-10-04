// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package biomes implements the sixth stage of the map pipeline. It gives
// every cell a biome from its altitude and moisture, using the table in
// section 7 of "Polygonal Map Generation for Games" (the Whittaker diagram as
// adapted in mapgen2's Map.as). Altitude stands in for temperature.
//
// mapgen2 reshapes elevations so that every map has the same share of land
// in each altitude zone. This package does not: altitude runs linearly from
// sea level (0) to the highest cell (1), so a low world has little snow.
//
// mapgen2 makes every land cell next to the ocean a beach. Here only low
// coastal cells are beaches; the rest, where the land rises quickly from the
// sea, are rocky shore.
package biomes

import (
	"fmt"

	"github.com/maloquacious/wg/moisture"
)

// Biome is the kind of land or water in a cell.
type Biome uint8

// The biomes, named as in mapgen2.
const (
	Ocean Biome = iota
	Lake
	Marsh      // a lake at low altitude
	Ice        // a lake at high altitude
	Beach      // low land next to the ocean
	RockyShore // higher land next to the ocean
	Snow
	Tundra
	Bare
	Scorched
	Taiga
	Shrubland
	TemperateDesert
	TemperateRainForest
	TemperateDeciduousForest
	Grassland
	SubtropicalDesert
	TropicalRainForest
	TropicalSeasonalForest
	numBiomes
)

var names = [numBiomes]string{
	Ocean:                    "OCEAN",
	Lake:                     "LAKE",
	Marsh:                    "MARSH",
	Ice:                      "ICE",
	Beach:                    "BEACH",
	RockyShore:               "ROCKY_SHORE",
	Snow:                     "SNOW",
	Tundra:                   "TUNDRA",
	Bare:                     "BARE",
	Scorched:                 "SCORCHED",
	Taiga:                    "TAIGA",
	Shrubland:                "SHRUBLAND",
	TemperateDesert:          "TEMPERATE_DESERT",
	TemperateRainForest:      "TEMPERATE_RAIN_FOREST",
	TemperateDeciduousForest: "TEMPERATE_DECIDUOUS_FOREST",
	Grassland:                "GRASSLAND",
	SubtropicalDesert:        "SUBTROPICAL_DESERT",
	TropicalRainForest:       "TROPICAL_RAIN_FOREST",
	TropicalSeasonalForest:   "TROPICAL_SEASONAL_FOREST",
}

// String returns the biome's name, such as "TEMPERATE_DESERT". Every biome
// but ROCKY_SHORE is named as in mapgen2.
func (b Biome) String() string {
	if b < numBiomes {
		return names[b]
	}
	return fmt.Sprintf("Biome(%d)", uint8(b))
}

// All returns every biome, in order.
func All() []Biome {
	all := make([]Biome, numBiomes)
	for b := range all {
		all[b] = Biome(b)
	}
	return all
}

// Options configures the biomes.
type Options struct {
	// RockyAltitude is the altitude above which a cell next to the ocean is
	// rocky shore rather than beach.
	RockyAltitude float64
}

// DefaultOptions returns the options used by the tests.
func DefaultOptions() Options {
	return Options{RockyAltitude: 0.04}
}

// Map holds the biome of every cell.
type Map struct {
	Moisture *moisture.Map
	// Altitude is each cell's height above sea level, from 0 at sea level to
	// 1 at the highest cell. It is 0 for ocean cells.
	Altitude []float64
	Cells    []Biome
}

// Generate assigns the biomes for m. The same moisture map always produces
// the same biomes.
func Generate(m *moisture.Map, opts Options) (*Map, error) {
	if !(opts.RockyAltitude >= 0) {
		return nil, fmt.Errorf("biomes: invalid rocky altitude %g", opts.RockyAltitude)
	}
	t := m.Rivers.Terrain
	sea := max(t.Mesh.SeaLevel, 0)
	top := sea
	for _, c := range t.Cells {
		top = max(top, c.Elevation)
	}

	b := &Map{
		Moisture: m,
		Altitude: make([]float64, len(t.Cells)),
		Cells:    make([]Biome, len(t.Cells)),
	}
	for i, c := range t.Cells {
		if !c.Ocean && top > sea {
			b.Altitude[i] = min(max((c.Elevation-sea)/(top-sea), 0), 1)
		}
		b.Cells[i] = opts.Classify(c.Ocean, c.Lake >= 0, c.Coast, b.Altitude[i], m.Cells[i])
	}
	return b, nil
}

// Classify returns the biome of a cell from mapgen2's table, with coastal
// cells split into beach and rocky shore. altitude and moisture are in 0...1.
func (o Options) Classify(ocean, lake, coast bool, altitude, moisture float64) Biome {
	switch {
	case ocean:
		return Ocean
	case lake:
		switch {
		case altitude < 0.1:
			return Marsh
		case altitude > 0.8:
			return Ice
		}
		return Lake
	case coast && altitude > o.RockyAltitude:
		return RockyShore
	case coast:
		return Beach
	case altitude > 0.8:
		switch {
		case moisture > 0.5:
			return Snow
		case moisture > 0.33:
			return Tundra
		case moisture > 0.16:
			return Bare
		}
		return Scorched
	case altitude > 0.6:
		switch {
		case moisture > 0.66:
			return Taiga
		case moisture > 0.33:
			return Shrubland
		}
		return TemperateDesert
	case altitude > 0.3:
		switch {
		case moisture > 0.83:
			return TemperateRainForest
		case moisture > 0.5:
			return TemperateDeciduousForest
		case moisture > 0.16:
			return Grassland
		}
		return TemperateDesert
	}
	switch {
	case moisture > 0.66:
		return TropicalRainForest
	case moisture > 0.33:
		return TropicalSeasonalForest
	case moisture > 0.16:
		return Grassland
	}
	return SubtropicalDesert
}
