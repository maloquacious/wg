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
		biomes.Cell
		want biomes.Biome
	}{
		{Cell: biomes.Cell{Ocean: true, Altitude: 0.9, Moisture: 0.9}, want: biomes.Ocean},
		{Cell: biomes.Cell{Lake: true, Altitude: 0.05}, want: biomes.Marsh},
		{Cell: biomes.Cell{Lake: true, Altitude: 0.5}, want: biomes.Lake},
		{Cell: biomes.Cell{Lake: true, Altitude: 0.85}, want: biomes.Ice},
		{Cell: biomes.Cell{Lake: true, Coast: true, Altitude: 0.5}, want: biomes.Lake},
		{Cell: biomes.Cell{Coast: true, Altitude: 0.04, Moisture: 0.9}, want: biomes.Beach},
		{Cell: biomes.Cell{Coast: true, Altitude: 0.041, Moisture: 0}, want: biomes.RockyShore},
		{Cell: biomes.Cell{Coast: true, Altitude: 0.9, Moisture: 0.9}, want: biomes.RockyShore},
		{Cell: biomes.Cell{Coast: true, Altitude: 0.5, CoastSlope: 0.0101}, want: biomes.Cliff},
		{Cell: biomes.Cell{Coast: true, Altitude: 0.5, CoastSlope: 0.01}, want: biomes.RockyShore},
		{Cell: biomes.Cell{Coast: true, Altitude: 0.04, CoastSlope: 0.5}, want: biomes.Beach},
		{Cell: biomes.Cell{Altitude: 0.5, Moisture: 0.6, CoastSlope: 0.5}, want: biomes.TemperateDeciduousForest},
		{Cell: biomes.Cell{Altitude: 0.5, Moisture: 0.6, Border: true}, want: biomes.TemperateDeciduousForest}, // BorderCliffs is off

		{Cell: biomes.Cell{Altitude: 0.81, Moisture: 0.51}, want: biomes.Snow},
		{Cell: biomes.Cell{Altitude: 0.81, Moisture: 0.5}, want: biomes.Tundra},
		{Cell: biomes.Cell{Altitude: 0.81, Moisture: 0.33}, want: biomes.Bare},
		{Cell: biomes.Cell{Altitude: 0.81, Moisture: 0.16}, want: biomes.Scorched},

		{Cell: biomes.Cell{Altitude: 0.8, Moisture: 0.67}, want: biomes.Taiga},
		{Cell: biomes.Cell{Altitude: 0.61, Moisture: 0.66}, want: biomes.Shrubland},
		{Cell: biomes.Cell{Altitude: 0.61, Moisture: 0.33}, want: biomes.TemperateDesert},

		{Cell: biomes.Cell{Altitude: 0.6, Moisture: 0.84}, want: biomes.TemperateRainForest},
		{Cell: biomes.Cell{Altitude: 0.31, Moisture: 0.83}, want: biomes.TemperateDeciduousForest},
		{Cell: biomes.Cell{Altitude: 0.31, Moisture: 0.5}, want: biomes.Grassland},
		{Cell: biomes.Cell{Altitude: 0.31, Moisture: 0.16}, want: biomes.TemperateDesert},

		{Cell: biomes.Cell{Altitude: 0.3, Moisture: 0.67}, want: biomes.TropicalRainForest},
		{Cell: biomes.Cell{Altitude: 0, Moisture: 0.66}, want: biomes.TropicalSeasonalForest},
		{Cell: biomes.Cell{Altitude: 0, Moisture: 0.33}, want: biomes.Grassland},
		{Cell: biomes.Cell{Altitude: 0, Moisture: 0.16}, want: biomes.SubtropicalDesert},
	} {
		if got := biomes.DefaultOptions().Classify(tc.Cell); got != tc.want {
			t.Errorf("%+v: got %v", tc.Cell, got)
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
		cell := biomes.Cell{Ocean: c.Ocean, Lake: c.Lake >= 0, Coast: c.Coast, Altitude: a, Moisture: b.Moisture.Cells[i], CoastSlope: b.CoastSlope[i]}
		if (b.CoastSlope[i] > 0) && !c.Coast {
			t.Fatalf("cell %d: coast slope %g off the coast", i, b.CoastSlope[i])
		}
		if want := biomes.DefaultOptions().Classify(cell); b.Cells[i] != want {
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
	for _, biome := range []biomes.Biome{biomes.Beach, biomes.RockyShore, biomes.Cliff, biomes.Grassland, biomes.TropicalRainForest, biomes.TemperateDeciduousForest} {
		if count[biome] == 0 {
			t.Errorf("no %v cells", biome)
		}
	}
}

// TestBorderCliffs checks that BorderCliffs turns every land cell on the
// edge of the map into cliff and changes nothing else.
func TestBorderCliffs(t *testing.T) {
	off := generate(t)
	opts := biomes.DefaultOptions()
	opts.BorderCliffs = true
	on := generateWith(t, opts)
	mesh := on.Moisture.Rivers.Terrain.Mesh
	tr := on.Moisture.Rivers.Terrain

	var border int
	for i, c := range mesh.Cells {
		touches := slices.ContainsFunc(c.Corners, func(k int) bool { return mesh.Corners[k].Border })
		switch {
		case touches && !tr.Cells[i].Ocean:
			border++
			if on.Cells[i] != biomes.Cliff {
				t.Fatalf("cell %d: on the border but %v", i, on.Cells[i])
			}
		case on.Cells[i] != off.Cells[i]:
			t.Fatalf("cell %d: changed from %v to %v but is not land on the border", i, off.Cells[i], on.Cells[i])
		}
	}
	if border == 0 {
		t.Fatal("no land touches the border of the default map")
	}
	t.Logf("%d land cells on the border became cliff", border)

	// the rule also holds for a single cell
	for _, cell := range []biomes.Cell{
		{Border: true, Altitude: 0.9, Moisture: 0.9},
		{Border: true, Coast: true},
		{Border: true},
	} {
		if got := opts.Classify(cell); got != biomes.Cliff {
			t.Errorf("%+v: got %v, want CLIFF", cell, got)
		}
	}
	if got := opts.Classify(biomes.Cell{Border: true, Ocean: true}); got != biomes.Ocean {
		t.Errorf("ocean on the border: got %v", got)
	}
	if got := opts.Classify(biomes.Cell{Border: true, Lake: true, Altitude: 0.5}); got != biomes.Lake {
		t.Errorf("lake on the border: got %v", got)
	}
}

// TestRockyAltitude checks that raising the threshold turns rocky shore
// into beach and leaves every other cell alone.
func TestRockyAltitude(t *testing.T) {
	var prev *biomes.Map
	prevRocky := -1
	for _, th := range []float64{1, 0.08, 0.04, 0.02, 0} {
		opts := biomes.DefaultOptions()
		opts.RockyAltitude = th
		b := generateWith(t, opts)
		var rocky int
		for i, biome := range b.Cells {
			if biome == biomes.RockyShore || biome == biomes.Cliff {
				rocky++
				if b.Altitude[i] <= th {
					t.Fatalf("threshold %g: cell %d at altitude %g is rocky", th, i, b.Altitude[i])
				}
			}
			if prev != nil && biome != prev.Cells[i] && (prev.Cells[i] != biomes.Beach || (biome != biomes.RockyShore && biome != biomes.Cliff)) {
				t.Fatalf("threshold %g: cell %d changed from %v to %v", th, i, prev.Cells[i], biome)
			}
		}
		if th == 1 && rocky != 0 {
			t.Errorf("threshold 1: %d rocky cells, want none", rocky)
		}
		if rocky < prevRocky {
			t.Errorf("threshold %g: %d rocky cells, fewer than the %d at the higher threshold", th, rocky, prevRocky)
		}
		t.Logf("threshold %.2f: %d rocky shore or cliff cells", th, rocky)
		prev, prevRocky = b, rocky
	}
}

// TestCliffSlope checks that raising the slope threshold turns cliffs into
// rocky shore and leaves every other cell alone.
func TestCliffSlope(t *testing.T) {
	var prev *biomes.Map
	prevCliffs := -1
	for _, th := range []float64{0, 0.005, 0.01, 0.02, 1} {
		opts := biomes.DefaultOptions()
		opts.CliffSlope = th
		b := generateWith(t, opts)
		var cliffs int
		for i, biome := range b.Cells {
			if biome == biomes.Cliff {
				cliffs++
				if b.CoastSlope[i] <= th || b.Altitude[i] <= opts.RockyAltitude {
					t.Fatalf("threshold %g: cell %d with slope %g, altitude %g is a cliff", th, i, b.CoastSlope[i], b.Altitude[i])
				}
			}
			if prev != nil && biome != prev.Cells[i] && (prev.Cells[i] != biomes.Cliff || biome != biomes.RockyShore) {
				t.Fatalf("threshold %g: cell %d changed from %v to %v", th, i, prev.Cells[i], biome)
			}
		}
		if prevCliffs >= 0 && cliffs > prevCliffs {
			t.Errorf("threshold %g: %d cliffs, more than the %d at the lower threshold", th, cliffs, prevCliffs)
		}
		if th == 1 && cliffs != 0 {
			t.Errorf("threshold 1: %d cliffs, want none", cliffs)
		}
		t.Logf("threshold %.3f: %d cliff cells", th, cliffs)
		prev, prevCliffs = b, cliffs
	}
}

func TestGenerateRejectsBadOptions(t *testing.T) {
	m, err := defaultMoisture()
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range []float64{-0.1, math.NaN()} {
		if _, err := biomes.Generate(m, biomes.Options{RockyAltitude: th, CliffSlope: 0.01}); err == nil {
			t.Errorf("rocky altitude %g: want error, got nil", th)
		}
		if _, err := biomes.Generate(m, biomes.Options{RockyAltitude: 0.04, CliffSlope: th}); err == nil {
			t.Errorf("cliff slope %g: want error, got nil", th)
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
	for _, tc := range []struct {
		name           string
		borders, icons bool
	}{
		{"biomes.png", false, false},
		{"biomes-mesh.png", true, false},
		{"biomes-icons.png", false, true},
	} {
		opts := render.DefaultBiomesOptions()
		opts.Borders, opts.Icons = tc.borders, tc.icons
		name := tc.name
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

// TestRenderIcons checks that icons change the image, that the same seed
// draws the same icons, and that another seed picks other drawings.
func TestRenderIcons(t *testing.T) {
	b := generate(t)
	opts := render.DefaultBiomesOptions()
	plain := render.Biomes(b, opts)
	opts.Icons = true
	a, again := render.Biomes(b, opts), render.Biomes(b, opts)
	opts.Seed++
	other := render.Biomes(b, opts)
	if slices.Equal(plain.Pix, a.Pix) {
		t.Error("icons drew nothing")
	}
	if !slices.Equal(a.Pix, again.Pix) {
		t.Error("the same seed drew different icons")
	}
	if slices.Equal(a.Pix, other.Pix) {
		t.Error("another seed drew the same icons")
	}
}
