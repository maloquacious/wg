// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Command wg generates a fantasy map: a height map, a Voronoi mesh over it,
// drainage and lakes, rivers, moisture and biomes. It writes the renders it
// is asked for as PNG files and prints a summary.
//
// Usage:
//
//	wg [flags]
//
// For example, a map with about 10,000 land cells in the default 16:9 shape:
//
//	wg -land-cells 10000 -render biomes,rivers -out var
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/maloquacious/wg"
	"github.com/maloquacious/wg/biomes"
	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/moisture"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/rivers"
	"github.com/maloquacious/wg/terrain"
	"github.com/maloquacious/wg/voronoi"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "wg:", err)
		os.Exit(1)
	}
}

// renders lists the images wg can write, in the order it writes them.
var renders = []string{"topo", "mesh", "terrain", "rivers", "moisture", "biomes"}

// config holds the parsed flags.
type config struct {
	fracture  fracture.Options
	voronoi   voronoi.Options
	terrain   terrain.Options
	rivers    rivers.Options
	moisture  moisture.Options
	biomes    biomes.Options
	landCells int
	render    []string
	borders   bool
	out       string
	name      string
	stats     bool
	version   bool
}

// parse reads the flags in args. Every stage option defaults to the value in
// that stage's DefaultOptions.
func parse(args []string, stderr io.Writer) (*config, error) {
	c := &config{
		fracture: fracture.DefaultOptions(),
		voronoi:  voronoi.DefaultOptions(),
		terrain:  terrain.DefaultOptions(),
		rivers:   rivers.DefaultOptions(),
		moisture: moisture.DefaultOptions(),
		biomes:   biomes.DefaultOptions(),
	}
	fs := flag.NewFlagSet("wg", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: wg [flags]\n\nGenerates a fantasy map and writes the requested renders as PNG files.\n\nFlags:\n")
		fs.PrintDefaults()
	}

	fs.Uint64Var(&c.fracture.Seed, "seed", c.fracture.Seed, "seed for every random stage")
	fs.IntVar(&c.fracture.Width, "width", c.fracture.Width, "map width in pixels")
	fs.IntVar(&c.fracture.Height, "height", c.fracture.Height, "map height in pixels")
	fs.IntVar(&c.fracture.Rounds, "rounds", c.fracture.Rounds, "fracture rounds; more rounds give rougher terrain")
	fs.IntVar(&c.landCells, "land-cells", 0, "target number of land cells; when set, the map size is worked out from it, keeping the width:height ratio")
	fs.Float64Var(&c.voronoi.CellSize, "cell-size", c.voronoi.CellSize, "mean cell width in pixels")
	fs.IntVar(&c.voronoi.OceanPercent, "ocean", c.voronoi.OceanPercent, "percentage of cells, 0-100, that are ocean")
	fs.Float64Var(&c.terrain.LakeDepth, "lake-depth", c.terrain.LakeDepth, "how deep a pit must be to become a lake, in elevation")
	fs.Float64Var(&c.rivers.MinFlow, "min-flow", c.rivers.MinFlow, "land area in square pixels that an edge must drain to be a river")
	fs.Float64Var(&c.moisture.Spread, "spread", c.moisture.Spread, "distance in pixels over which wetness from water falls by a factor of e")
	fs.Float64Var(&c.moisture.SeaStrength, "sea", c.moisture.SeaStrength, "how much the sea wets the coast, where a lake is 1; 0 turns it off")
	fs.Float64Var(&c.moisture.Skew, "skew", c.moisture.Skew, "moisture skew: above 1 makes an arid world, below 1 a wet one")
	fs.Float64Var(&c.biomes.RockyAltitude, "rocky", c.biomes.RockyAltitude, "altitude above which coast is rocky shore rather than beach")
	fs.Float64Var(&c.biomes.CliffSlope, "cliff", c.biomes.CliffSlope, "coast slope, in altitude per pixel, above which rocky shore is a cliff")
	fs.BoolVar(&c.biomes.BorderCliffs, "border-cliffs", c.biomes.BorderCliffs, "make all land that touches the edge of the map cliff")
	renderList := fs.String("render", "biomes", "comma-separated images to write: "+strings.Join(renders, ", ")+", all or none")
	fs.BoolVar(&c.borders, "borders", false, "outline every cell in the mesh, terrain, rivers and biomes images")
	fs.StringVar(&c.out, "out", ".", "directory to write the images to")
	fs.StringVar(&c.name, "name", "map", "file name prefix, as in map-biomes.png")
	fs.BoolVar(&c.stats, "stats", false, "print the number of cells in each biome")
	fs.BoolVar(&c.version, "version", false, "print the version and exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	switch *renderList {
	case "all":
		c.render = slices.Clone(renders)
	case "none", "":
	default:
		for _, r := range strings.Split(*renderList, ",") {
			r = strings.TrimSpace(r)
			if !slices.Contains(renders, r) {
				return nil, fmt.Errorf("unknown render %q: want %s, all or none", r, strings.Join(renders, ", "))
			}
			if !slices.Contains(c.render, r) {
				c.render = append(c.render, r)
			}
		}
	}

	if c.landCells != 0 {
		if err := c.sizeForLandCells(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// sizeForLandCells sets the map size so that the mesh has about landCells
// land cells, keeping the width:height ratio of the width and height flags.
func (c *config) sizeForLandCells() error {
	switch {
	case c.landCells < 0:
		return fmt.Errorf("invalid land cells %d", c.landCells)
	case c.voronoi.OceanPercent >= 100:
		return fmt.Errorf("land cells %d: the map is all ocean", c.landCells)
	case c.fracture.Width < 1 || c.fracture.Height < 1:
		return fmt.Errorf("land cells %d: invalid size %dx%d", c.landCells, c.fracture.Width, c.fracture.Height)
	case !(c.voronoi.CellSize > 0):
		return fmt.Errorf("land cells %d: invalid cell size %g", c.landCells, c.voronoi.CellSize)
	}
	cells := math.Ceil(float64(c.landCells) * 100 / float64(100-c.voronoi.OceanPercent))
	area := cells * c.voronoi.CellSize * c.voronoi.CellSize
	aspect := float64(c.fracture.Width) / float64(c.fracture.Height)
	c.fracture.Height = max(1, int(math.Round(math.Sqrt(area/aspect))))
	c.fracture.Width = max(1, int(math.Round(area/float64(c.fracture.Height))))
	return nil
}

// run generates the map that args describe, writes its renders, and prints
// a summary to stdout.
func run(args []string, stdout, stderr io.Writer) error {
	c, err := parse(args, stderr)
	if err != nil {
		return err
	}
	if c.version {
		fmt.Fprintln(stdout, wg.Version().String())
		return nil
	}

	start := time.Now()
	hm, err := fracture.Generate(c.fracture)
	if err != nil {
		return err
	}
	mesh, err := voronoi.Generate(hm, c.voronoi)
	if err != nil {
		return err
	}
	tr, err := terrain.Generate(mesh, c.terrain)
	if err != nil {
		return err
	}
	rv, err := rivers.Generate(tr, c.rivers)
	if err != nil {
		return err
	}
	mo, err := moisture.Generate(rv, c.moisture)
	if err != nil {
		return err
	}
	bm, err := biomes.Generate(mo, c.biomes)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)

	count := make(map[biomes.Biome]int)
	for _, b := range bm.Cells {
		count[b]++
	}
	land := len(bm.Cells) - count[biomes.Ocean]
	var riverEdges int
	for _, f := range rv.River {
		if f > 0 {
			riverEdges++
		}
	}
	fmt.Fprintf(stdout, "wg %s: seed %#x, %dx%d pixels, %d rounds, generated in %v\n",
		wg.Version(), c.fracture.Seed, hm.Width, hm.Height, c.fracture.Rounds, elapsed.Round(time.Millisecond))
	fmt.Fprintf(stdout, "%d cells (%d land, %d ocean), %d lakes, %d river edges\n",
		len(mesh.Cells), land, count[biomes.Ocean], len(tr.Lakes), riverEdges)
	if c.stats {
		for _, b := range biomes.All()[1:] {
			fmt.Fprintf(stdout, "  %-26v %6d cells %5.1f%% of land\n", b, count[b], 100*float64(count[b])/float64(max(land, 1)))
		}
	}

	if len(c.render) > 0 {
		if err := os.MkdirAll(c.out, 0o755); err != nil {
			return err
		}
	}
	for _, r := range c.render {
		var img image.Image
		switch r {
		case "topo":
			opts := render.DefaultTopoOptions()
			opts.OceanPercent = c.voronoi.OceanPercent
			img = render.Topo(hm, opts)
		case "mesh":
			img = render.Mesh(mesh, render.MeshOptions{Borders: c.borders})
		case "terrain":
			opts := render.DefaultTerrainOptions()
			opts.Borders = c.borders
			img = render.Terrain(tr, opts)
		case "rivers":
			opts := render.DefaultRiversOptions()
			opts.Borders = c.borders
			img = render.Rivers(rv, opts)
		case "moisture":
			img = render.Moisture(mo, render.DefaultMoistureOptions())
		case "biomes":
			opts := render.DefaultBiomesOptions()
			opts.Borders = c.borders
			img = render.Biomes(bm, opts)
		}
		path := filepath.Join(c.out, c.name+"-"+r+".png")
		if err := render.WritePNG(path, img); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "wrote", path)
	}
	return nil
}
