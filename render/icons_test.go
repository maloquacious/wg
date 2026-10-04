// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/maloquacious/wg/biomes"
)

// TestIconSheet checks that every drawing on the sheet parses and stays
// inside its grid cell, give or take a stroke that strays over the line.
func TestIconSheet(t *testing.T) {
	icons, err := loadIcons()
	if err != nil {
		t.Fatal(err)
	}
	for row := range icons {
		for col, ic := range icons[row] {
			for _, s := range ic {
				for _, v := range []float64{s.x0, s.y0, s.x1, s.y1} {
					if v < -15 || v > iconSize+15 {
						t.Fatalf("row %d column %d: point %g is far outside the icon", row, col+1, v)
					}
				}
			}
		}
	}
}

// TestIconRows checks that the biomes with icons are real biomes and that
// beaches, rocky shores, cliffs and ice are left bare.
func TestIconRows(t *testing.T) {
	for b, row := range iconRow {
		if b >= biomes.Biome(len(biomes.All())) || row < 0 || row >= iconRows {
			t.Errorf("%v: row %d", b, row)
		}
	}
	for _, b := range []biomes.Biome{biomes.Beach, biomes.RockyShore, biomes.Cliff, biomes.Ice} {
		if _, ok := iconRow[b]; ok {
			t.Errorf("%v has an icon", b)
		}
	}
}

func TestDrawIcon(t *testing.T) {
	icons, err := loadIcons()
	if err != nil {
		t.Fatal(err)
	}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 255, 255, 255
	}
	// a 20-pixel tree, partly off the left edge
	drawIcon(img, icons[rowForest][0], -5, 10, 0.2, 1)
	inked := 0
	for y := range 40 {
		for x := range 40 {
			if img.RGBAAt(x, y) != white {
				inked++
				if x >= 15+2 || y < 10-2 || y >= 30+2 {
					t.Fatalf("ink at (%d, %d), outside the icon", x, y)
				}
			}
		}
	}
	if inked < 20 {
		t.Errorf("only %d pixels inked", inked)
	}
}
