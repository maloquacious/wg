// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"errors"
	"flag"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/maloquacious/wg"
)

// small makes the maps in these tests quick to generate.
var small = []string{"-width", "320", "-height", "180", "-rounds", "200"}

func TestRunWritesRenders(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := append(slices.Clone(small), "-render", "all", "-out", dir, "-name", "test", "-stats", "-borders", "-border-cliffs", "-icons")
	if err := run(args, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	for _, r := range renders {
		path := filepath.Join(dir, "test-"+r+".png")
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Errorf("%s: not written (%v)", path, err)
		}
		if !strings.Contains(stdout.String(), "wrote "+path) {
			t.Errorf("summary does not mention %s", path)
		}
	}
	for _, want := range []string{"320x180 pixels", "land", "lakes", "river edges", "GRASSLAND"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("summary is missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunWithoutRenders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never")
	var stdout, stderr bytes.Buffer
	if err := run(append(slices.Clone(small), "-render", "none", "-out", dir), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("output directory created with nothing to write")
	}
	if strings.Contains(stdout.String(), "wrote") {
		t.Errorf("wrote images with -render none:\n%s", stdout.String())
	}
}

func TestLandCells(t *testing.T) {
	for _, tc := range []struct {
		land, ocean   int
		width, height int
		size          float64
	}{
		{10_000, 52, 1920, 1080, 14},
		{10_000, 70, 2000, 2000, 14},
		{500, 30, 400, 300, 10},
	} {
		c, err := parse([]string{
			"-land-cells", itoa(tc.land), "-ocean", itoa(tc.ocean),
			"-width", itoa(tc.width), "-height", itoa(tc.height), "-cell-size", ftoa(tc.size),
		}, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		w, h := c.fracture.Width, c.fracture.Height
		cells := c.voronoi.CellCount(w, h)
		land := cells - cells*tc.ocean/100
		if math.Abs(float64(land-tc.land)) > 0.01*float64(tc.land) {
			t.Errorf("%+v: %dx%d gives %d land cells", tc, w, h, land)
		}
		if got, want := float64(w)/float64(h), float64(tc.width)/float64(tc.height); math.Abs(got-want) > 0.01*want {
			t.Errorf("%+v: aspect %g, want %g", tc, got, want)
		}
	}

	// the land cells of a generated map
	var stdout bytes.Buffer
	if err := run([]string{"-land-cells", "300", "-rounds", "200", "-render", "none"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "(300 land,") && !strings.Contains(stdout.String(), "(301 land,") {
		t.Errorf("want about 300 land cells:\n%s", stdout.String())
	}
}

func TestBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-render", "biomes,maps"},
		{"-land-cells", "-5"},
		{"-land-cells", "100", "-ocean", "100"},
		{"-ocean", "101", "-render", "none"},
		{"-skew", "0", "-render", "none"},
		{"stray"},
		{"-no-such-flag"},
	} {
		if err := run(append(slices.Clone(small), args...), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: want error, got nil", args)
		}
	}
}

func TestHelpAndVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: want flag.ErrHelp, got %v", err)
	}
	if !strings.Contains(stderr.String(), "-land-cells") {
		t.Errorf("-h: usage does not list -land-cells:\n%s", stderr.String())
	}
	stdout.Reset()
	if err := run([]string{"-version"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != wg.Version().String() {
		t.Errorf("-version: got %q, want %q", got, wg.Version().String())
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }
