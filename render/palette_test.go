// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/maloquacious/wg/biomes"
)

// TestPaletteMatchesResource checks the Go palette against the discrete
// colors in resources/biome-colors.js, entry for entry.
func TestPaletteMatchesResource(t *testing.T) {
	src, err := os.ReadFile("../resources/biome-colors.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)discreteColors\s*=\s*\{(.*?)\}`).FindSubmatch(src)
	if block == nil {
		t.Fatal("discreteColors not found")
	}
	entries := regexp.MustCompile(`(\w+):\s*"#([0-9a-fA-F]{6})"`).FindAllSubmatch(block[1], -1)
	if len(entries) != len(palette) {
		t.Errorf("biome-colors.js has %d colors, palette has %d", len(entries), len(palette))
	}
	for _, e := range entries {
		name := string(e[1])
		hex, _ := strconv.ParseUint(string(e[2]), 16, 32)
		got, ok := palette[name]
		if !ok {
			t.Errorf("%s: missing from palette", name)
		} else if got != rgb(uint32(hex)) {
			t.Errorf("%s: palette %v, biome-colors.js #%s", name, got, e[2])
		}
	}
	for _, b := range biomes.All() {
		_, mapgen2 := palette[b.String()]
		_, own := ownColors[b.String()]
		if mapgen2 == own {
			t.Errorf("biome %v: in mapgen2's palette %v, in ours %v; want exactly one", b, mapgen2, own)
		}
	}
}
