// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package biomes_test

import (
	"flag"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/maloquacious/wg/biomes"
	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/moisture"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/rivers"
	"github.com/maloquacious/wg/terrain"
	"github.com/maloquacious/wg/voronoi"
)

// renderDir enables writing the test biomes as images, for example
//
//	go test ./biomes -render=../var
var renderDir = flag.String("render", "", "write rendered biomes to this directory")

// defaultMoisture is the stage 5 moisture map built from the default options.
var defaultMoisture = sync.OnceValues(func() (*moisture.Map, error) {
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
	n, err := rivers.Generate(tr, rivers.DefaultOptions())
	if err != nil {
		return nil, err
	}
	return moisture.Generate(n, moisture.DefaultOptions())
})

func generate(t *testing.T) *biomes.Map {
	t.Helper()
	return generateWith(t, biomes.DefaultOptions())
}

func generateWith(t *testing.T, opts biomes.Options) *biomes.Map {
	t.Helper()
	m, err := defaultMoisture()
	if err != nil {
		t.Fatal(err)
	}
	b, err := biomes.Generate(m, opts)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestClassify checks the table, including its zone and moisture boundaries.
func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		ocean, lake, coast bool
		altitude, moisture float64
		want               biomes.Biome
	}{
		{ocean: true, altitude: 0.9, moisture: 0.9, want: biomes.Ocean},
		{lake: true, altitude: 0.05, want: biomes.Marsh},
		{lake: true, altitude: 0.5, want: biomes.Lake},
		{lake: true, altitude: 0.85, want: biomes.Ice},
		{lake: true, coast: true, altitude: 0.5, want: biomes.Lake},
		{coast: true, altitude: 0.04, moisture: 0.9, want: biomes.Beach},
		{coast: true, altitude: 0.041, moisture: 0, want: biomes.RockyShore},
		{coast: true, altitude: 0.9, moisture: 0.9, want: biomes.RockyShore},

		{altitude: 0.81, moisture: 0.51, want: biomes.Snow},
		{altitude: 0.81, moisture: 0.5, want: biomes.Tundra},
		{altitude: 0.81, moisture: 0.33, want: biomes.Bare},
		{altitude: 0.81, moisture: 0.16, want: biomes.Scorched},

		{altitude: 0.8, moisture: 0.67, want: biomes.Taiga},
		{altitude: 0.61, moisture: 0.66, want: biomes.Shrubland},
		{altitude: 0.61, moisture: 0.33, want: biomes.TemperateDesert},

		{altitude: 0.6, moisture: 0.84, want: biomes.TemperateRainForest},
		{altitude: 0.31, moisture: 0.83, want: biomes.TemperateDeciduousForest},
		{altitude: 0.31, moisture: 0.5, want: biomes.Grassland},
		{altitude: 0.31, moisture: 0.16, want: biomes.TemperateDesert},

		{altitude: 0.3, moisture: 0.67, want: biomes.TropicalRainForest},
		{altitude: 0, moisture: 0.66, want: biomes.TropicalSeasonalForest},
		{altitude: 0, moisture: 0.33, want: biomes.Grassland},
		{altitude: 0, moisture: 0.16, want: biomes.SubtropicalDesert},
	} {
		if got := biomes.DefaultOptions().Classify(tc.ocean, tc.lake, tc.coast, tc.altitude, tc.moisture); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

// TestGenerate checks each cell against Classify and checks the altitudes.
func TestGenerate(t *testing.T) {
	b := generate(t)
	tr := b.Moisture.Rivers.Terrain
	var top float64
	count := make(map[biomes.Biome]int)
	for i, c := range tr.Cells {
		a := b.Altitude[i]
		if a < 0 || a > 1 || (c.Ocean && a != 0) {
			t.Fatalf("cell %d: altitude %g", i, a)
		}
		top = max(top, a)
		if want := biomes.DefaultOptions().Classify(c.Ocean, c.Lake >= 0, c.Coast, a, b.Moisture.Cells[i]); b.Cells[i] != want {
			t.Fatalf("cell %d: biome %v, want %v", i, b.Cells[i], want)
		}
		count[b.Cells[i]]++
	}
	if top != 1 {
		t.Errorf("highest altitude %g, want 1", top)
	}
	var land int
	for _, biome := range biomes.All() {
		if biome != biomes.Ocean {
			land += count[biome]
		}
	}
	t.Logf("%d ocean cells, %d land cells", count[biomes.Ocean], land)
	for _, biome := range biomes.All()[1:] {
		t.Logf("%-26v %5d cells %5.1f%% of land", biome, count[biome], 100*float64(count[biome])/float64(land))
	}
	// the default map has land in every altitude zone
	for _, biome := range []biomes.Biome{biomes.Beach, biomes.RockyShore, biomes.Grassland, biomes.TropicalRainForest, biomes.TemperateDeciduousForest} {
		if count[biome] == 0 {
			t.Errorf("no %v cells", biome)
		}
	}
}

// TestRockyAltitude checks that raising the threshold turns rocky shore
// into beach and leaves every other cell alone.
func TestRockyAltitude(t *testing.T) {
	var prev *biomes.Map
	prevRocky := -1
	for _, th := range []float64{1, 0.08, 0.04, 0.02, 0} {
		b := generateWith(t, biomes.Options{RockyAltitude: th})
		var rocky int
		for i, biome := range b.Cells {
			if biome == biomes.RockyShore {
				rocky++
				if b.Altitude[i] <= th {
					t.Fatalf("threshold %g: cell %d at altitude %g is rocky", th, i, b.Altitude[i])
				}
			}
			if prev != nil && biome != prev.Cells[i] && (prev.Cells[i] != biomes.Beach || biome != biomes.RockyShore) {
				t.Fatalf("threshold %g: cell %d changed from %v to %v", th, i, prev.Cells[i], biome)
			}
		}
		if th == 1 && rocky != 0 {
			t.Errorf("threshold 1: %d rocky cells, want none", rocky)
		}
		if rocky < prevRocky {
			t.Errorf("threshold %g: %d rocky cells, fewer than the %d at the higher threshold", th, rocky, prevRocky)
		}
		t.Logf("threshold %.2f: %d rocky shore cells", th, rocky)
		prev, prevRocky = b, rocky
	}
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	m, err := defaultMoisture()
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range []float64{-0.1, math.NaN()} {
		if _, err := biomes.Generate(m, biomes.Options{RockyAltitude: th}); err == nil {
			t.Errorf("rocky altitude %g: want error, got nil", th)
		}
	}
}

func TestBiomeNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, b := range biomes.All() {
		name := b.String()
		if name == "" || seen[name] {
			t.Errorf("biome %d: name %q is empty or repeated", b, name)
		}
		seen[name] = true
	}
	if got := biomes.Biome(200).String(); got != "Biome(200)" {
		t.Errorf("unknown biome: got %q", got)
	}
}

func TestRender(t *testing.T) {
	if *renderDir == "" {
		t.Skip("set -render to write the biome images")
	}
	b := generate(t)
	for name, borders := range map[string]bool{"biomes.png": false, "biomes-mesh.png": true} {
		opts := render.DefaultBiomesOptions()
		opts.Borders = borders
		path := filepath.Join(*renderDir, name)
		if err := render.WritePNG(path, render.Biomes(b, opts)); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := generate(t), generate(t)
	if !slices.Equal(a.Cells, b.Cells) || !slices.Equal(a.Altitude, b.Altitude) {
		t.Fatal("biomes differ between runs")
	}
}
