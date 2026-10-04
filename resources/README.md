# Resources

* map-icons.svg - [Red Blob Games Mapgen 2 SVG icons](https://www.redblobgames.com/maps/mapgen2/map-icons.svg)
* map-icons.png - the same sheet as a PNG, from mapgen2's repository, for looking up which row is which (not used by the code)
* biome-colors.js - [Red Blob Games Mapgen 2 Biome pallete](https://www.redblobgames.com/maps/mapgen2/map-icons.html)

`biome-colors.js` is Copyright 2017 Red Blob Games and licensed under the
[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html), as its
header states. `map-icons.svg` is by Red Blob Games and licensed under the
[Creative Commons Attribution 4.0 International License](https://creativecommons.org/licenses/by/4.0/),
per its [source page](https://www.redblobgames.com/maps/mapgen2/map-icons.html);
credit Red Blob Games wherever the icons appear.

The `resources` Go package embeds `map-icons.svg` for the biomes render's
`-icons` option. The sheet is a grid of 100-unit cells: a label in column 0
and five drawings in columns 1 to 5, one row per kind of icon.
