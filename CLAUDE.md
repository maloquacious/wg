# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## Commands

```sh
go build ./...
go vet ./...
go test ./...
go test ./voronoi -run TestNeighborsAreSymmetric   # one test

# write PNGs for visual inspection into var/ (gitignored; paths are relative to the package dir)
go test ./fracture -run 'TestGenerate$' -render=../var   # var/fracture-topo.png
go test ./voronoi -run TestRender -render=../var         # var/voronoi-mesh.png
```

The `-render` flag is defined in the `fracture` and `voronoi` test files. `voronoi`'s `TestRender` skips without it.

## Architecture

A fantasy map generator built as a pipeline of stages. Each stage is a package with an `Options` struct, a `DefaultOptions()` (the values the tests use), and a `Generate` that is deterministic for the same inputs.

1. **`fracture`**: makes the height map. Each round raises or lowers every pixel inside a random circle by 1, and the result is normalized to 0…1. It is adapted from `pkg/generators/flat` in github.com/mdhender/mapgen; `TestMatchesReference` checks it pixel for pixel against the original's per-pixel circle test. The defaults are 1920×1080 pixels, 1,000 rounds and seed `0x0123456789abcdef`. Elevations come in whole steps (about 58 distinct levels at the defaults), and later stages and renderers have to allow for that.
2. **`voronoi`**: lays a mesh over a `*heightmap.Map`. It triangulates random sites with `github.com/fogleman/delaunay`, builds each cell by clipping the map rectangle against the bisectors of its Delaunay neighbors (edges are labeled, so neighbors come from the clipped polygon), and runs a fixed number of Lloyd relaxation passes. A cell's elevation is the mean of the pixels whose centers lie inside it. Ocean is the lowest `OceanPercent` of cells *by count*, and `Mesh.SeaLevel` is −1 when there is no ocean. The only user-facing options are `Cells` and `OceanPercent`, plus the seed that every random stage must take.

`heightmap.Map` stores elevations row-major (`Data[y*Width+x]`), whereas mapgen used `[x][y]`.

`render` is for tests and visual checks only:
- `Topo` follows the recipe in `valle-escondido.sh` from github.com/mdhender/vetopo: a six-stop color ramp, multidirectional hill shading rescaled to 25–100% and multiplied into the colors, minor and index contours, water tinted by depth, and a coastline. Its sea level comes from the share of *pixels*, and it blurs a copy of the heights (`Smooth`) to hide the fracture steps.
- `Mesh` colors whole cells with the same ramps.

The two renders flood different things (pixels versus cells), so their coastlines differ, and that is expected.

The pixel-center fill rule (a pixel belongs to a polygon when `left <= x+0.5 < right`) appears in both `voronoi.elevation` and `render.fillPolygon`. Keep the two in sync.
