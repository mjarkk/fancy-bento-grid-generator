---
name: bento-grid
description: Render bento grid images (PNG) from a TOML layout with the `fancy-bento-grid-generator` CLI, where every tile is a stack of images, SVGs and gradients. Use when the user wants a bento grid, feature grid, tiled showcase or collage image, or is writing or debugging a bento grid TOML config.
---

# Bento grids with fancy-bento-grid-generator

`fancy-bento-grid-generator` reads a TOML file that describes a grid of tiles and writes a PNG.

## Setup

Check that it is installed with `command -v fancy-bento-grid-generator`. If it is missing, install it (needs Go):

```sh
go install github.com/mjarkk/fancy-bento-grid-generator@latest
```

The binary lands in `$(go env GOPATH)/bin`, which must be on `PATH`.

## Running

```sh
fancy-bento-grid-generator grid.toml              # writes grid.png next to grid.toml
fancy-bento-grid-generator -o out.png grid.toml
```

- Image paths in the config are relative to the config file. Absolute paths work too.
- Unknown keys are errors, not ignored.
- Errors count tiles and layers from 1 in file order: `tile 3: layer 2: ...`.

## Workflow

1. Pick the canvas size and grid, and decide which cells each tile covers.
2. Work out each tile's pixel size (see [Tile sizes](#tile-sizes)) and make its content at that size: SVGs (writing them by hand is fine) or existing images.
3. Write the TOML and render it.
4. Look at the PNG (read the image file) and adjust. Repeat until it looks right.

## How a grid works

The canvas is split into `columns` × `rows` equal cells. Each tile covers a rectangle of cells. Tiles may not overlap, and cells without a tile show the sheet (`background`).

A tile has no layout of its own. Its layers are drawn on top of each other, first to last, and clipped to the tile's rounded corners. An image layer can be inset, positioned, rotated or repeated, but there is no way to place things side by side. To put a heading, some text and an icon in one tile, draw them all in one SVG made at the tile's size.

## Tile sizes

```
cell_w = (width  − 2 × (margin + padding) − (columns − 1) × gap) / columns
cell_h = (height − 2 × (margin + padding) − (rows − 1) × gap) / rows

tile_w = n × cell_w + (n − 1) × gap     # tile covering n columns
tile_h = m × cell_h + (m − 1) × gap     # tile covering m rows
```

For example, `width = 1200`, `height = 810`, `padding = 32`, `gap = 16`, 4 columns and 3 rows give 272 × 238 cells. A tile with `cols = "1-2"` and `rows = "1-2"` is then 560 × 492.

Choose `width` and `height` so the cells come out as whole pixels. Give each tile's SVG a `viewBox` of exactly its tile size (`viewBox="0 0 560 492"`) and the default `fit = "cover"` fills the tile edge to edge, with coordinates in the SVG matching pixels in the output.

## Config

```toml
width = 1200
height = 810
columns = 4
rows = 3
background = "#0b0b0f"
tile_background = "#1c1c22"
padding = 32

[[tiles]]
cols = "1-2"
rows = "1-2"
layers = [
  { image = "photo.jpg" },
  { colors = ["#00000000", "#000000b3"] },  # darken the bottom so text on top reads
  { image = "hero.svg" },
]

[[tiles]]
cols = "3-3"
rows = "1-1"
background = "#18122b"
layers = [{ image = "fast.svg" }]
```

### Canvas

| Key               | Default   | Description                                |
| ----------------- | --------- | ------------------------------------------ |
| `width`/`height`  | required  | Output size in pixels                      |
| `columns`/`rows`  | required  | Grid dimensions                            |
| `gap`             | `16`      | Space between tiles                        |
| `padding`         | `16`      | Space between the sheet edge and the grid  |
| `margin`          | `0`       | Transparent space between the canvas edge and the sheet |
| `background`      | required  | Sheet color                                |
| `tile_background` | required  | Default tile color                         |
| `radius`          | `24`      | Tile corner radius                         |
| `sheet_radius`    | `0`       | Sheet corner radius                        |
| `sheet_shadow`    |           | [Shadow](#shadow) under the sheet; needs a `margin` to show |
| `tile_shadow`     |           | [Shadow](#shadow) under every tile         |

The sheet is the rectangle in `background` color that the tiles sit on. It fills the canvas unless `margin` or `sheet_radius` is set; the canvas outside it is transparent. Everything on the sheet, tile shadows included, is clipped to it.

Colors are `#rgb`, `#rrggbb` or `#rrggbbaa`.

#### Shadow

A drop shadow like CSS `box-shadow`, written as an inline table:

```toml
margin = 48
sheet_radius = 40
sheet_shadow = { y = 16, blur = 40, color = "#00000080" }
tile_shadow = { y = 6, blur = 18 }
```

| Key     | Default     | Description |
| ------- | ----------- | ----------- |
| `x`/`y` | `0`         | Offset in pixels, right and down |
| `blur`  | `0`         | Blur radius in pixels, as in CSS |
| `color` | `#00000066` | Shadow color |

Like in CSS, a shadow is not drawn under the shape that casts it, so it doesn't darken a translucent tile or sheet.

### Tile (`[[tiles]]`)

| Key           | Default                  | Description |
| ------------- | ------------------------ | ----------- |
| `cols`/`rows` | required                 | Cells covered as `"first-last"`, counting from 1: `"1-2"` for the first two, `"3-3"` for just the third |
| `background`  | canvas `tile_background` | Color under the layers |
| `layers`      |                          | Layers, drawn first to last |

### Layer

A layer is an image when it has `image`, or a linear gradient when it has `colors`. Never both.

| Key        | Applies to | Default | Description |
| ---------- | ---------- | ------- | ----------- |
| `image`    | image      |         | Path to a PNG, JPEG, GIF, WebP or SVG |
| `fit`      | image      | `cover`, or `none` with `repeat` | `cover` scales to fill the tile and crops; `contain` fits it inside; `none` keeps its own size (pixels, or the SVG's `viewBox` units) |
| `position` | image      | `center` | Where the image sits when it doesn't fill the tile exactly: `center`, an edge like `top`, or a corner like `bottom right` |
| `padding`  | image      | `0`     | Inset in pixels from the tile's edge; the image is clipped to it |
| `repeat`   | image      |         | `x`, `y` or `both` to tile copies outward from `position`. Not allowed with `fit = "cover"` |
| `opacity`  | image      | `1`     | `0` (invisible) to `1` (opaque) |
| `colors`   | gradient   |         | Two or more colors, spread evenly |
| `angle`    | both       | `180` for a gradient, `0` for an image | Degrees clockwise. A gradient runs that way, as in CSS: `0` bottom to top, `90` left to right, `180` top to bottom. An image is rotated by it |

A rotated image still follows its `fit`: `cover` leaves no gaps and `contain` shows the whole image. Repeats run along the image's rotated axes, so a stripe pattern with an `angle` gives diagonal stripes.

### Layer recipes

```toml
{ image = "logo.svg", fit = "contain", padding = 24 }             # logo centered with breathing room
{ image = "badge.svg", fit = "none", position = "top right", padding = 16 }
{ image = "dots.svg", repeat = "both", opacity = 0.3 }            # background pattern
{ image = "stripes.svg", repeat = "both", angle = 45 }            # diagonal stripes
{ angle = 90, colors = ["#f472b6", "#c084fc"] }                   # left-to-right gradient
{ angle = 160, colors = ["#7c3aed66", "#7c3aed00"] }              # soft glow fading out
```

## SVG limitations

SVGs are rasterized with [oksvg](https://github.com/srwiley/oksvg) at their final size, so they stay sharp. Paths, basic shapes, groups, transforms, gradients, `style` attributes and `<style>` class rules work. Keep these in mind when writing or picking SVGs:

- **`<text>` is not rendered.** Text has to be paths. Export from a design tool with outlined text (Figma: "Outline text"), convert with `inkscape in.svg --export-text-to-path --export-plain-svg -o out.svg`, or put the text in a PNG made some other way.
- Filters (blur, drop shadow), masks and clip paths are skipped. Fake a shadow or glow with a gradient layer or semi-transparent shapes instead.
- Rounded `<rect>`s need both `rx` and `ry`, and `scale()` needs both arguments: oksvg doesn't apply the SVG defaults for these.
- The root `<svg>` needs a `viewBox`, or a `width` and `height`.
