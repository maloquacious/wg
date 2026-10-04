// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package resources embeds the Red Blob Games mapgen2 files that the
// renderer draws with. See README.md for their licenses.
package resources

import _ "embed"

// MapIcons is map-icons.svg, Red Blob Games' sheet of hand-drawn map icons,
// under the Creative Commons Attribution 4.0 International License. Credit
// Red Blob Games wherever the icons appear.
//
//go:embed map-icons.svg
var MapIcons string
