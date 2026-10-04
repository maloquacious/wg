// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render_test

import (
	"testing"

	"github.com/maloquacious/wg/fracture"
	"github.com/maloquacious/wg/render"
	"github.com/maloquacious/wg/voronoi"
)

func TestMesh(t *testing.T) {
	hm, err := fracture.Generate(fracture.Options{Width: 320, Height: 180, Rounds: 200, Seed: 0x0123456789abcdef})
	if err != nil {
		t.Fatal(err)
	}
	opts := voronoi.DefaultOptions()
	opts.Cells = 200
	mesh, err := voronoi.Generate(hm, opts)
	if err != nil {
		t.Fatal(err)
	}
	img := render.Mesh(mesh, render.MeshOptions{})
	if b := img.Bounds(); b.Dx() != hm.Width || b.Dy() != hm.Height {
		t.Fatalf("size: want %dx%d, got %dx%d", hm.Width, hm.Height, b.Dx(), b.Dy())
	}
	// the pixel under each site shows whether the cell is ocean or land
	for i, c := range mesh.Cells {
		px := img.RGBAAt(int(c.Site.X), int(c.Site.Y))
		if water := px.B > px.R && px.B > px.G; water != c.Ocean {
			t.Errorf("cell %d: ocean %v, but the pixel at its site is %v", i, c.Ocean, px)
		}
	}
}
