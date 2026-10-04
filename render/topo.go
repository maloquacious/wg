// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package render draws pipeline output as images for testing.
package render

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"slices"

	"github.com/maloquacious/wg/heightmap"
)

// TopoOptions configures Topo.
type TopoOptions struct {
	// Relief is the height, in pixels, of elevation 1. Larger values give
	// deeper hill shading.
	Relief float64
	// Contours is the number of contour intervals between elevation 0 and 1.
	// Zero disables contour lines.
	Contours int
	// IndexEvery draws every n-th contour as a heavier index line.
	// Zero disables index lines.
	IndexEvery int
	// OceanPercent is the percentage of pixels, 0...100, below sea level.
	OceanPercent int
	// Smooth is the radius, in pixels, of the blur applied to a copy of the
	// elevations before drawing. It softens the steps left by the fracture
	// stage. Zero draws the elevations as they are.
	Smooth int
}

// DefaultTopoOptions returns the options used by the tests.
func DefaultTopoOptions() TopoOptions {
	return TopoOptions{
		Relief:       200,
		Contours:     20,
		IndexEvery:   5,
		OceanPercent: 70,
		Smooth:       4,
	}
}

// Topo renders hm as a shaded topographic map. Land is tinted by elevation
// and multiplied by a multidirectional hillshade, with minor and index
// contour lines on top. Water is tinted by depth and outlined by a coastline.
//
// The land recipe follows https://github.com/mdhender/vetopo.
func Topo(hm *heightmap.Map, opts TopoOptions) *image.RGBA {
	if opts.Smooth > 0 {
		hm = smooth(hm, opts.Smooth)
	}
	sea := seaLevel(hm.Data, opts.OceanPercent)
	img := image.NewRGBA(image.Rect(0, 0, hm.Width, hm.Height))
	for y := range hm.Height {
		for x := range hm.Width {
			e := hm.At(x, y)
			if e < sea {
				img.SetRGBA(x, y, waterAt((sea-e)/sea))
				continue
			}
			tint := tintAt(landElevation(e, sea))
			// stretch the shade from 25%...100% to 0...1, as `-level 25%,100%` does
			shade := clamp01((hillshade(hm, x, y, opts.Relief) - 0.25) / 0.75)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(tint[0] * shade),
				G: uint8(tint[1] * shade),
				B: uint8(tint[2] * shade),
				A: 255,
			})
		}
	}
	if opts.Contours > 0 {
		drawContours(img, hm, sea, opts)
	}
	drawCoast(img, hm, sea)
	return img
}

// smooth returns a copy of hm blurred by three passes of a box filter with
// the given radius, which approximates a Gaussian blur. The map edges are
// repeated.
func smooth(hm *heightmap.Map, radius int) *heightmap.Map {
	out := &heightmap.Map{Width: hm.Width, Height: hm.Height, Data: slices.Clone(hm.Data)}
	tmp := make([]float64, len(out.Data))
	for range 3 {
		boxBlur(tmp, out.Data, hm.Width, hm.Height, radius, 1, hm.Width)
		boxBlur(out.Data, tmp, hm.Height, hm.Width, radius, hm.Width, 1)
	}
	return out
}

// boxBlur averages src into dst along lines of length n. Element i of line
// j is at j*lineStride + i*step.
func boxBlur(dst, src []float64, n, lines, radius, step, lineStride int) {
	at := func(base, i int) float64 {
		return src[base+min(max(i, 0), n-1)*step]
	}
	scale := 1 / float64(2*radius+1)
	for j := range lines {
		base := j * lineStride
		var sum float64
		for i := -radius; i <= radius; i++ {
			sum += at(base, i)
		}
		for i := range n {
			dst[base+i*step] = sum * scale
			sum += at(base, i+radius+1) - at(base, i-radius)
		}
	}
}

// seaLevel returns the elevation below which pixels are water. Elevations
// repeat, so it picks the level whose share of water pixels is closest to
// percent. Zero means no water.
func seaLevel(data []float64, percent int) float64 {
	sorted := slices.Clone(data)
	slices.Sort(sorted)
	n := len(sorted) * percent / 100
	if n <= 0 {
		return 0
	}
	if n >= len(sorted) {
		// just above the highest point, so that everything is water
		return math.Nextafter(sorted[len(sorted)-1], math.Inf(1))
	}
	// water is either everything below sorted[n] or everything up to and
	// including it
	below, _ := slices.BinarySearch(sorted, sorted[n])
	through := below
	for through < len(sorted) && sorted[through] == sorted[n] {
		through++
	}
	if n-below <= through-n || through == len(sorted) {
		return sorted[n]
	}
	return sorted[through]
}

// landElevation rescales e so that the coast is 0 and the highest point is 1.
func landElevation(e, sea float64) float64 {
	if sea >= 1 {
		return 0
	}
	return (e - sea) / (1 - sea)
}

// waterAt returns the water tint for a depth from 0 (the coast) to 1 (the
// deepest point).
func waterAt(depth float64) color.RGBA {
	shallow, deep := [3]float64{150, 200, 225}, [3]float64{25, 65, 125}
	t := clamp01(depth)
	return color.RGBA{
		R: uint8(shallow[0] + (deep[0]-shallow[0])*t),
		G: uint8(shallow[1] + (deep[1]-shallow[1])*t),
		B: uint8(shallow[2] + (deep[2]-shallow[2])*t),
		A: 255,
	}
}

// drawCoast outlines the land on every water pixel that touches it.
func drawCoast(img *image.RGBA, hm *heightmap.Map, sea float64) {
	coast := color.RGBA{R: 40, G: 60, B: 90, A: 255}
	for y := range hm.Height {
		for x := range hm.Width {
			if hm.At(x, y) >= sea {
				continue
			}
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if px, py := x+d[0], y+d[1]; px >= 0 && px < hm.Width && py >= 0 && py < hm.Height && hm.At(px, py) >= sea {
					img.SetRGBA(x, y, coast)
					break
				}
			}
		}
	}
}

// WritePNG saves img to path.
func WritePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ramp is the elevation tint, from valley green through tan and brown to
// snow. Stop positions are scaled from vetopo's 1000...1800 m ramp.
var ramp = []struct {
	at      float64
	r, g, b float64
}{
	{0.0000, 46, 110, 60},
	{0.1875, 112, 160, 84},
	{0.3750, 196, 196, 120},
	{0.5625, 214, 170, 110},
	{0.7500, 170, 120, 80},
	{1.0000, 240, 236, 230},
}

func tintAt(e float64) [3]float64 {
	for n := 1; n < len(ramp); n++ {
		if a, b := ramp[n-1], ramp[n]; e <= b.at {
			t := clamp01((e - a.at) / (b.at - a.at))
			return [3]float64{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
		}
	}
	last := ramp[len(ramp)-1]
	return [3]float64{last.r, last.g, last.b}
}

// hillshade returns the illumination, 0...1, at (x, y). Like GDAL's
// -multidirectional mode, it blends light from four azimuths at 45 degrees
// altitude, weighting each by the direction the slope faces.
func hillshade(hm *heightmap.Map, x, y int, relief float64) float64 {
	// Horn's method: a 3x3 window, with the map edge repeated
	z := func(dx, dy int) float64 {
		return hm.At(min(max(x+dx, 0), hm.Width-1), min(max(y+dy, 0), hm.Height-1))
	}
	dzdx := ((z(1, -1) + 2*z(1, 0) + z(1, 1)) - (z(-1, -1) + 2*z(-1, 0) + z(-1, 1))) / 8 * relief
	dzdy := ((z(-1, 1) + 2*z(0, 1) + z(1, 1)) - (z(-1, -1) + 2*z(0, -1) + z(1, -1))) / 8 * relief

	// the surface normal, with y pointing down the image
	length := math.Sqrt(dzdx*dzdx + dzdy*dzdy + 1)
	nx, ny, nz := -dzdx/length, -dzdy/length, 1/length
	// aspect is the compass direction the slope faces, clockwise from north
	aspect := math.Atan2(nx, -ny)

	const altitude = math.Pi / 4
	var sum, weights float64
	for _, deg := range []float64{225, 270, 315, 360} {
		az := deg * math.Pi / 180
		// the direction toward the light
		lx, ly, lz := math.Sin(az)*math.Cos(altitude), -math.Cos(az)*math.Cos(altitude), math.Sin(altitude)
		w := 0.5 * (1 - math.Cos(aspect-az))
		sum += w * max(nx*lx+ny*ly+nz*lz, 0)
		weights += w
	}
	if weights == 0 {
		return math.Sin(altitude)
	}
	return sum / weights
}

// drawContours draws a line wherever the land elevation crosses a contour
// level between a pixel and its right or lower neighbor. Minor lines are
// blended at 55%; index lines are opaque and one pixel thicker on each side.
func drawContours(img *image.RGBA, hm *heightmap.Map, sea float64, opts TopoOptions) {
	minor := color.RGBA{R: 90, G: 60, B: 40, A: 255}
	index := color.RGBA{R: 70, G: 40, B: 25, A: 255}
	// level returns the contour level at (x, y), or -1 for water
	level := func(x, y int) int {
		e := hm.At(x, y)
		if e < sea {
			return -1
		}
		return int(math.Floor(landElevation(e, sea) * float64(opts.Contours)))
	}

	// crossing returns the highest contour crossed at (x, y), or -1
	crossing := func(x, y int) int {
		l, crossed := level(x, y), -1
		if l < 0 {
			return -1
		}
		if x+1 < hm.Width {
			if r := level(x+1, y); r >= 0 && r != l {
				crossed = max(crossed, l, r)
			}
		}
		if y+1 < hm.Height {
			if d := level(x, y+1); d >= 0 && d != l {
				crossed = max(crossed, l, d)
			}
		}
		return crossed
	}

	isIndex := make([]bool, hm.Width*hm.Height)
	for y := range hm.Height {
		for x := range hm.Width {
			c := crossing(x, y)
			if c < 0 {
				continue
			}
			if opts.IndexEvery > 0 && c%opts.IndexEvery == 0 {
				isIndex[y*hm.Width+x] = true
			} else {
				img.SetRGBA(x, y, blend(img.RGBAAt(x, y), minor, 0.55))
			}
		}
	}

	// dilate the index lines by a disk of radius 1
	for y := range hm.Height {
		for x := range hm.Width {
			if !isIndex[y*hm.Width+x] {
				continue
			}
			for _, d := range [][2]int{{0, 0}, {-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if px, py := x+d[0], y+d[1]; px >= 0 && px < hm.Width && py >= 0 && py < hm.Height {
					img.SetRGBA(px, py, index)
				}
			}
		}
	}
}

func blend(dst, src color.RGBA, alpha float64) color.RGBA {
	mix := func(d, s uint8) uint8 {
		return uint8(float64(d)*(1-alpha) + float64(s)*alpha)
	}
	return color.RGBA{R: mix(dst.R, src.R), G: mix(dst.G, src.G), B: mix(dst.B, src.B), A: 255}
}

func clamp01(v float64) float64 {
	return min(max(v, 0), 1)
}
