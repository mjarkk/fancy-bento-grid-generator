---
name: bento-grid
description: Render bento grid images (PNG, JPEG, WebP) from a TOML layout with the `fancy-bento-grid-generator` CLI, where every tile is a stack of images, SVGs, text and gradients. Use when the user wants a bento grid, feature grid, release image, tiled showcase or collage image, or is writing or debugging a bento grid TOML config.
---

# Bento grids with fancy-bento-grid-generator

`fancy-bento-grid-generator` reads a TOML file that describes a grid of tiles and writes a PNG, JPEG or WebP.

## Setup

Check that it is installed with `command -v fancy-bento-grid-generator`. If it is missing, install it (needs Go):

```sh
go install github.com/mjarkk/fancy-bento-grid-generator@latest
```

The binary lands in `$(go env GOPATH)/bin`, which must be on `PATH`.

## Running

```sh
fancy-bento-grid-generator grid.toml                      # writes grid.png next to grid.toml
fancy-bento-grid-generator -o out.webp grid.toml          # format from the extension: .png, .jpg/.jpeg, .webp
fancy-bento-grid-generator -scale 2 -quality 85 -o out.jpg grid.toml
fancy-bento-grid-generator sizes grid.toml                # every tile's x, y, w, h; add -json for JSON
```

- `-scale` overrides the config's `scale`. `-quality` (1–100, default 90) is for `.jpg` and `.webp` only.
- JPEG has no alpha: a transparent `margin` turns white. Use PNG or WebP to keep it. WebP is far smaller than PNG; good for Slack.
- Image and font paths in the config are relative to the config file. Absolute paths work too.
- Unknown keys are errors, not ignored. So are keys that don't apply to a layer.
- Errors count tiles and layers from 1 in file order: `tile 3: layer 2: ...`.

## Workflow

1. Pick the canvas size in logical pixels (e.g. 1600×1000), a `scale` (2 for crisp images), and the grid. Decide which cells each tile covers.
2. Run `sizes` on the config to get each tile's size in logical pixels, and make SVGs at exactly that size if needed.
3. Build tiles from layers: backgrounds and glows (gradients), images and UI cards (SVGs placed with `x`/`y`/`width`, with `radius` and `shadow`), and headings and labels (text layers).
4. Render, look at the image (read the file) and adjust. Repeat until it looks right.

## How a grid works

The canvas is split into `columns` × `rows` equal cells. Each tile covers a rectangle of cells. Tiles may not overlap, and cells without a tile show the sheet (`background`); `require_full = true` turns those into an error.

A tile has no layout of its own. Its layers are drawn on top of each other, first to last, and clipped to the tile's rounded corners. Each image or text layer is placed either by `position` (a keyword) inset by `padding`, or by `x`/`y` pixels from the tile's top-left corner. To put a heading, a line of text and an icon side by side, use three layers with their own `x`/`y`.

## Scale and tile sizes

Everything in the config is in logical pixels. `scale = 2` doubles the output and every length (gaps, padding, radii, shadows, borders, `x`/`y`/`width`/`height`, text sizes), so author at 1600×1000 and get 3200×2000. SVGs are rasterized at their final size and stay sharp; with `fit = "none"` or `repeat` one viewBox unit is one logical pixel. Raster images count one image pixel as one logical pixel, so `fit = "none"` upscales them; give a high-resolution raster a `width` in logical pixels instead.

Tile edges are rounded to whole output pixels, with equal gaps, so any `width`, `padding` and `gap` give crisp tiles. Read tile sizes from `sizes` rather than computing them:

```
$ fancy-bento-grid-generator sizes grid.toml
canvas 1600×1000, scale 2: 3200×2000 output px

tile  cols  rows  x       y    w      h    out x  out y  out w  out h
1     1-3   1-3   40      40   751    545  80     80     1502   1090
2     4-4   1-2   809     40   238.5  357  1618   80     477    714
```

Give a tile's SVG `viewBox="0 0 w h"` with the logical `w` and `h` and the default `fit = "cover"` fills it edge to edge, with SVG units matching logical pixels.

## Config

```toml
width = 1600
height = 1000
scale = 2
columns = 6
rows = 5
gap = 18
padding = 40
background = "#0a0a0f"
tile_background = "#16161d"
tile_border = { color = "#ffffff1a" }                        # 1 px hairline on every tile
tile_shadow = { y = 8, blur = 24, color = "#00000080" }
require_full = true

[[tiles]]
cols = "1-3"
rows = "1-3"
layers = [
  { gradient = "radial", position = "top left", radius = 520, colors = ["#7c3aed66", "#7c3aed00"] },
  { text = "Release 4.2", font = "fonts/Inter-Bold.ttf", size = 56, color = "#fff", x = 40, y = 84 },
  { text = "Shared checklists and offline sync.", font = "fonts/Inter-Regular.ttf", size = 20, color = "#a1a1aa", max_width = 340, x = 40, y = 164 },
  { image = "card.svg", x = 360, y = 300, width = 460, radius = 18, shadow = { y = 16, blur = 40 } },  # runs off the edge
]

[[tiles]]
cols = "4-4"
rows = "1-2"
background = "#f4f4f5"
radius = 12
border = false
shadow = false
layers = [{ image = "photo.jpg" }]
```

### Canvas

| Key               | Default   | Description                                |
| ----------------- | --------- | ------------------------------------------ |
| `width`/`height`  | required  | Canvas size in logical pixels              |
| `scale`           | `1`       | Output pixels per logical pixel            |
| `columns`/`rows`  | required  | Grid dimensions                            |
| `gap`             | `16`      | Space between tiles                        |
| `gap_x`/`gap_y`   | `gap`     | Space between columns, and between rows    |
| `padding`         | `16`      | Space between the sheet edge and the grid  |
| `margin`          | `0`       | Transparent space between the canvas edge and the sheet |
| `background`      | required  | Sheet color                                |
| `tile_background` | required  | Default tile color                         |
| `radius`          | `24`      | Default tile corner radius                 |
| `sheet_radius`    | `0`       | Sheet corner radius                        |
| `sheet_shadow`    |           | [Shadow](#shadow) under the sheet; needs a `margin` to show |
| `tile_shadow`     |           | Default [shadow](#shadow) under every tile |
| `tile_border`     |           | Default [border](#border) of every tile    |
| `require_full`    | `false`   | Error on cells without a tile, listing them |

The sheet is the rectangle in `background` color that the tiles sit on. It fills the canvas unless `margin` or `sheet_radius` is set; the canvas outside it is transparent. Everything on the sheet, tile shadows included, is clipped to it.

Colors are `#rgb`, `#rrggbb` or `#rrggbbaa`.

#### Shadow

A drop shadow like CSS `box-shadow`, written as an inline table, or `false` for none:

```toml
sheet_shadow = { y = 16, blur = 40, color = "#00000080" }
tile_shadow = { y = 6, blur = 18 }
```

| Key     | Default     | Description |
| ------- | ----------- | ----------- |
| `x`/`y` | `0`         | Offset in pixels, right and down |
| `blur`  | `0`         | Blur radius in pixels, as in CSS |
| `color` | `#00000066` | Shadow color |

Like in CSS, a shadow is not drawn under the shape that casts it, so it doesn't darken a translucent tile or sheet.

#### Border

Drawn along the inside of a tile's edge, on top of its layers. An inline table, or `false` for none.

| Key     | Default  | Description |
| ------- | -------- | ----------- |
| `width` | `1`      | Width in pixels |
| `color` | required | Border color |

### Tile (`[[tiles]]`)

| Key           | Default                  | Description |
| ------------- | ------------------------ | ----------- |
| `cols`/`rows` | required                 | Cells covered as `"first-last"`, counting from 1: `"1-2"` for the first two, `"3-3"` for just the third |
| `background`  | canvas `tile_background` | Color under the layers |
| `radius`      | canvas `radius`          | Corner radius |
| `shadow`      | canvas `tile_shadow`     | Shadow, or `false` for none |
| `border`      | canvas `tile_border`     | Border, or `false` for none |
| `layers`      |                          | Layers, drawn first to last |

### Layer

A layer is an image when it has `image`, text when it has `text`, or a gradient when it has `colors`. Exactly one of them.

| Key        | Applies to | Default | Description |
| ---------- | ---------- | ------- | ----------- |
| `image`    | image      |         | Path to a PNG, JPEG, GIF, WebP or SVG |
| `fit`      | image      | `cover`, or `none` with `repeat` or `x`/`y` | `cover` scales to fill the tile and crops; `contain` fits it inside; `none` keeps its own size. Not with `width`/`height` |
| `width`/`height` | image |        | Size in pixels; with only one, the other keeps the aspect ratio |
| `repeat`   | image      |         | `x`, `y` or `both` to tile copies outward from where the image is placed. Not with `fit = "cover"` |
| `radius`   | image, radial gradient | `0`, or the farthest corner | An image's corner radius. A radial gradient's radius |
| `position` | all but linear gradients | `center` | `center`, an edge like `top`, or a corner like `bottom right`. A radial gradient's center |
| `padding`  | image, text | `0`    | Inset in pixels from the tile's edge; the layer is clipped to it |
| `x`/`y`    | all but linear gradients | | Pixels from the tile's top-left corner to the layer's top-left corner (a radial gradient's center). Negative or past the edge is fine: the tile clips it. Setting one replaces `position`, the other is `0`. Not with `position` or `padding` |
| `shadow`   | image, text |        | Shadow following the layer's shape: a rounded card, an icon's outline, letters |
| `opacity`  | image, text | `1`    | `0` (invisible) to `1` (opaque) |
| `angle`    | all but radial gradients | `180` for a gradient, `0` otherwise | Degrees clockwise. A linear gradient runs that way, as in CSS: `0` bottom to top, `90` left to right, `180` top to bottom. Images and text are rotated by it |
| `text`     | text       |         | The text; `\n` breaks lines |
| `font`     | text       | required | Path to a TTF or OTF; pick the weight by picking the file |
| `size`     | text       | `16`    | Font size in pixels |
| `color`    | text       | `#000`  | Text color |
| `letter_spacing` | text | `0`     | Extra pixels between letters; may be negative |
| `line_height` | text    | `1.2`   | Multiple of `size` |
| `max_width` | text      |         | Wrap between words to fit this many pixels |
| `align`    | text       | `left`  | `left`, `center` or `right`, lining up lines with each other |
| `colors`   | gradient   |         | Two or more colors, spread evenly |
| `gradient` | gradient   | `linear` | `linear` or `radial` |

A text block is as wide as its longest line and `line_height` × `size` tall per line, letters centered in each line as in CSS; `position`/`padding`/`x`/`y` place that block. Text is kerned.

A rotated image still follows its `fit`: `cover` leaves no gaps and `contain` shows the whole image. Repeats run along the image's rotated axes, so a stripe pattern with an `angle` gives diagonal stripes. An image placed by `x`/`y` turns about its own center.

### Layer recipes

```toml
{ image = "logo.svg", fit = "contain", padding = 24 }             # logo centered with breathing room
{ image = "badge.svg", fit = "none", position = "top right", padding = 16 }
{ image = "dots.svg", repeat = "both", opacity = 0.3 }            # background pattern
{ image = "stripes.svg", repeat = "both", angle = 45 }            # diagonal stripes
{ image = "card.svg", x = 220, y = 22, width = 400, radius = 12, shadow = { y = 12, blur = 32, color = "#00000080" } }  # UI card running off the tile
{ image = "icon.svg", fit = "none", position = "top left", padding = 32, shadow = { y = 6, blur = 14 } }  # icon with a shadow of its outline
{ text = "Heading", font = "Inter-Bold.ttf", size = 40, color = "#fff", position = "bottom left", padding = 32 }
{ text = "2.4×", font = "Inter-Bold.ttf", size = 64, letter_spacing = -2, color = "#fff", position = "center" }
{ text = "A longer line that wraps", font = "Inter-Regular.ttf", size = 18, max_width = 300, line_height = 1.5, color = "#a1a1aa", x = 32, y = 96 }
{ angle = 90, colors = ["#f472b6", "#c084fc"] }                   # left-to-right gradient
{ gradient = "radial", position = "top left", colors = ["#7c3aed66", "#7c3aed00"] }  # soft glow from a corner
{ gradient = "radial", x = 380, y = 40, radius = 300, colors = ["#f472b633", "#f472b600"] }
```

## SVG limitations

SVGs are rasterized with [oksvg](https://github.com/srwiley/oksvg) at their final size, so they stay sharp. Paths, basic shapes, groups, transforms, gradients, `style` attributes and `<style>` class rules work. Keep these in mind when writing or picking SVGs:

- **`<text>` is not rendered.** Put text in a text layer on top of the SVG instead.
- Filters (blur, drop shadow), masks and clip paths are skipped. Give the image layer a `shadow` for a drop shadow and a `radius` for rounded corners; fake glows with a radial gradient layer.
- Rounded `<rect>`s need both `rx` and `ry`, and `scale()` needs both arguments: oksvg doesn't apply the SVG defaults for these.
- The root `<svg>` needs a `viewBox`, or a `width` and `height`.
