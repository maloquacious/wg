// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"

	"github.com/maloquacious/wg/biomes"
	"github.com/maloquacious/wg/resources"
)

// The icon sheet, resources/map-icons.svg, is Red Blob Games' mapgen2 icons
// (CC BY 4.0). It is a grid of 100×100 cells: a label in column 0, then five
// drawings of the same thing in columns 1 to 5. mapgen2 draws from a PNG of
// the sheet with its grid at (9, 4), which puts the grid's origin at
// (iconLeft, iconTop) in the SVG's coordinates.
const (
	iconLeft    = 291.044312 + 9
	iconTop     = 71.611176 + 4
	iconSize    = 100
	iconRows    = 10
	iconColumns = 5
)

// The rows of the icon sheet.
const (
	rowWater = iota
	rowMountains
	rowShrub
	rowDesert
	rowJungle
	rowForest
	rowGrassland
	rowMarsh
	rowWinterForest
	rowNorthernForest
)

// iconRow is the row that mapgen2's draw.js gives each biome. It draws
// mountains wherever elevation is above 0.8, which is where the table puts
// snow, tundra, bare and scorched land. The biomes not listed get no icon.
// mapgen2 also turns forest in the northern 30% of the map into northern
// forest, and we don't, because the map has no climate by latitude.
var iconRow = map[biomes.Biome]int{
	biomes.Ocean:                    rowWater,
	biomes.Lake:                     rowWater,
	biomes.Snow:                     rowMountains,
	biomes.Tundra:                   rowMountains,
	biomes.Bare:                     rowMountains,
	biomes.Scorched:                 rowMountains,
	biomes.Shrubland:                rowShrub,
	biomes.TemperateDesert:          rowDesert,
	biomes.SubtropicalDesert:        rowDesert,
	biomes.TropicalRainForest:       rowJungle,
	biomes.TropicalSeasonalForest:   rowJungle,
	biomes.TemperateDeciduousForest: rowForest,
	biomes.TemperateRainForest:      rowForest,
	biomes.Grassland:                rowGrassland,
	biomes.Marsh:                    rowMarsh,
	biomes.Taiga:                    rowNorthernForest,
}

// inkColor is the color the icons are drawn in.
var inkColor = color.RGBA{R: 30, G: 26, B: 20, A: 255}

// minIconStroke is the narrowest line, in pixels, that an icon is drawn
// with, so that small icons stay visible.
const minIconStroke = 0.9

// segment is a straight piece of an icon stroke, in sheet units relative to
// the icon's top left corner. A dot is a segment of zero length.
type segment struct {
	x0, y0, x1, y1 float64
	// width is the line width, or the diameter of a dot.
	width float64
}

// icon is one drawing on the sheet.
type icon []segment

// loadIcons parses the sheet into icons[row][column-1].
var loadIcons = sync.OnceValues(func() (*[iconRows][iconColumns]icon, error) {
	return parseIcons(resources.MapIcons)
})

// parseIcons reads the paths and dots of the icon sheet and sorts them into
// grid cells by the centers of their bounding boxes. It flattens each
// quadratic Bézier into straight segments. The labels in column 0 are
// dropped.
func parseIcons(sheet string) (*[iconRows][iconColumns]icon, error) {
	var icons [iconRows][iconColumns]icon
	add := func(segs []segment) {
		if len(segs) == 0 {
			return
		}
		minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, s := range segs {
			minX, maxX = min(minX, s.x0, s.x1), max(maxX, s.x0, s.x1)
			minY, maxY = min(minY, s.y0, s.y1), max(maxY, s.y0, s.y1)
		}
		col := int(math.Floor(((minX+maxX)/2 - iconLeft) / iconSize))
		row := int(math.Floor(((minY+maxY)/2 - iconTop) / iconSize))
		if col < 1 || col > iconColumns || row < 0 || row >= iconRows {
			return
		}
		ox, oy := iconLeft+float64(col)*iconSize, iconTop+float64(row)*iconSize
		for _, s := range segs {
			s.x0, s.y0, s.x1, s.y1 = s.x0-ox, s.y0-oy, s.x1-ox, s.y1-oy
			icons[row][col-1] = append(icons[row][col-1], s)
		}
	}

	d := xml.NewDecoder(strings.NewReader(sheet))
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("icons: %w", err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := func(name string) string {
			for _, a := range el.Attr {
				if a.Name.Local == name {
					return a.Value
				}
			}
			return ""
		}
		switch el.Name.Local {
		case "path":
			width, err := strconv.ParseFloat(attr("stroke-width"), 64)
			if err != nil {
				return nil, fmt.Errorf("icons: path %s: stroke width: %w", attr("id"), err)
			}
			segs, err := flattenPath(attr("d"), width)
			if err != nil {
				return nil, fmt.Errorf("icons: path %s: %w", attr("id"), err)
			}
			add(segs)
		case "circle":
			var v [3]float64
			for i, name := range []string{"cx", "cy", "r"} {
				if v[i], err = strconv.ParseFloat(attr(name), 64); err != nil {
					return nil, fmt.Errorf("icons: circle %s: %s: %w", attr("id"), name, err)
				}
			}
			add([]segment{{x0: v[0], y0: v[1], x1: v[0], y1: v[1], width: 2 * v[2]}})
		}
	}
	for row := range icons {
		for col := range icons[row] {
			if len(icons[row][col]) == 0 {
				return nil, fmt.Errorf("icons: row %d column %d is empty", row, col+1)
			}
		}
	}
	return &icons, nil
}

// flattenPath turns path data made of absolute M, L and Q commands into
// segments. Each quadratic Bézier becomes bezierSteps segments.
func flattenPath(data string, width float64) ([]segment, error) {
	const bezierSteps = 6
	fields := strings.Fields(data)
	var segs []segment
	var x, y float64
	nums := func(i, n int) ([]float64, error) {
		if i+n > len(fields) {
			return nil, fmt.Errorf("%q: too few numbers", fields[i-1])
		}
		v := make([]float64, n)
		for k := range v {
			var err error
			if v[k], err = strconv.ParseFloat(fields[i+k], 64); err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	for i := 0; i < len(fields); {
		cmd := fields[i]
		i++
		switch cmd {
		case "M":
			v, err := nums(i, 2)
			if err != nil {
				return nil, err
			}
			x, y = v[0], v[1]
			i += 2
		case "L":
			v, err := nums(i, 2)
			if err != nil {
				return nil, err
			}
			segs = append(segs, segment{x, y, v[0], v[1], width})
			x, y = v[0], v[1]
			i += 2
		case "Q":
			v, err := nums(i, 4)
			if err != nil {
				return nil, err
			}
			cx, cy, ex, ey := v[0], v[1], v[2], v[3]
			px, py := x, y
			for k := 1; k <= bezierSteps; k++ {
				t := float64(k) / bezierSteps
				u := 1 - t
				qx := u*u*x + 2*u*t*cx + t*t*ex
				qy := u*u*y + 2*u*t*cy + t*t*ey
				segs = append(segs, segment{px, py, qx, qy, width})
				px, py = qx, qy
			}
			x, y = ex, ey
			i += 4
		default:
			return nil, fmt.Errorf("unsupported path command %q", cmd)
		}
	}
	return segs, nil
}

// drawIcons draws a biome icon in each cell that has one, as mapgen2 does:
// the icon fills a square centered on the cell's site whose half-width is
// the distance from the site to its nearest corner, and which of the five
// drawings it gets is chosen at random from seed. The sheet is embedded, so
// it panics only if the sheet is broken, which TestIconSheet checks.
func drawIcons(img *image.RGBA, b *biomes.Map, seed uint64) {
	icons, err := loadIcons()
	if err != nil {
		panic(err)
	}
	rnd := rand.New(rand.NewPCG(seed, seed))
	for i, c := range b.Moisture.Rivers.Terrain.Mesh.Cells {
		row, ok := iconRow[b.Cells[i]]
		if !ok {
			continue
		}
		col := rnd.IntN(iconColumns)
		radius := math.Inf(1)
		for _, p := range c.Polygon {
			radius = min(radius, math.Hypot(p.X-c.Site.X, p.Y-c.Site.Y))
		}
		drawIcon(img, icons[row][col], c.Site.X-radius, c.Site.Y-radius, 2*radius/iconSize)
	}
}

// drawIcon draws ic with its top left corner at (left, top), scaled by
// scale pixels per sheet unit. Each pixel takes the coverage of the stroke
// nearest its center, so the lines are antialiased and overlapping strokes
// do not darken one another.
func drawIcon(img *image.RGBA, ic icon, left, top, scale float64) {
	bounds := img.Bounds().Intersect(image.Rect(
		int(math.Floor(left))-2, int(math.Floor(top))-2,
		int(math.Ceil(left+iconSize*scale))+2, int(math.Ceil(top+iconSize*scale))+2))
	if bounds.Empty() {
		return
	}
	cover := make([]float64, bounds.Dx()*bounds.Dy())
	for _, s := range ic {
		x0, y0 := left+s.x0*scale, top+s.y0*scale
		x1, y1 := left+s.x1*scale, top+s.y1*scale
		r := max(s.width*scale, minIconStroke) / 2
		dx, dy := x1-x0, y1-y0
		length2 := dx*dx + dy*dy
		box := bounds.Intersect(image.Rect(
			int(math.Floor(min(x0, x1)-r-1)), int(math.Floor(min(y0, y1)-r-1)),
			int(math.Ceil(max(x0, x1)+r+1)), int(math.Ceil(max(y0, y1)+r+1))))
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				px, py := float64(x)+0.5, float64(y)+0.5
				t := 0.0
				if length2 > 0 {
					t = clamp01(((px-x0)*dx + (py-y0)*dy) / length2)
				}
				a := clamp01(r + 0.5 - math.Hypot(px-(x0+t*dx), py-(y0+t*dy)))
				n := (y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X
				cover[n] = max(cover[n], a)
			}
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if a := cover[(y-bounds.Min.Y)*bounds.Dx()+x-bounds.Min.X]; a > 0 {
				img.SetRGBA(x, y, blend(img.RGBAAt(x, y), inkColor, a))
			}
		}
	}
}
