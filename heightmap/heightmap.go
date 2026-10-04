// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package heightmap implements a store for elevation data.
package heightmap

import (
	"fmt"
	"image"
)

// Map is a height map with elevations normalized to the range 0...1.
type Map struct {
	Width, Height int
	// Data holds the elevations in row-major order, indexed as y*Width + x.
	Data []float64
}

// New returns a flat map of the given size.
func New(width, height int) (*Map, error) {
	if width < 1 || height < 1 {
		return nil, fmt.Errorf("heightmap: invalid size %dx%d", width, height)
	}
	return &Map{Width: width, Height: height, Data: make([]float64, width*height)}, nil
}

// At returns the elevation at (x, y).
func (m *Map) At(x, y int) float64 {
	return m.Data[y*m.Width+x]
}

// Gray returns the map as a grayscale image, with black at elevation 0 and
// white at elevation 1.
func (m *Map) Gray() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, m.Width, m.Height))
	for n, e := range m.Data {
		img.Pix[n] = uint8(e * 255)
	}
	return img
}
