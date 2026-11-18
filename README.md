# fancy-bento-grid-generator

Describe a bento grid in a TOML file, get a PNG, JPEG or WebP back. Each tile is a stack of layers: images (PNG, JPEG, GIF, WebP), SVGs, text and gradients.

![example](example/bento.png)

## Usage

```sh
go install github.com/mjarkk/fancy-bento-grid-generator@latest
fancy-bento-grid-generator example/bento.toml              # writes example/bento.png
fancy-bento-grid-generator -o out.webp my-grid.toml
fancy-bento-grid-generator -scale 2 -o out.jpg -quality 85 my-grid.toml
fancy-bento-grid-generator sizes my-grid.toml              # where every tile lands
```

Without `-o`, the PNG goes next to the config with the same name. Image and font paths in the config are relative to the config file.

| Flag       | Description |
| ---------- | ----------- |
| `-o`       | Output path. Its extension picks the format: `.png`, `.jpg`/`.jpeg` or `.webp` |
| `-scale`   | Overrides the config's [`scale`](#scale) |
| `-quality` | `1` to `100` for `.jpg` and `.webp`; default `90`. An error for `.png` |

JPEG has no transparency, so a transparent `margin` turns white. WebP keeps it.

`sizes` prints every tile's position and size in logical and output pixels, to make SVGs at exactly a tile's size. It takes `-scale` too, and `-json` for JSON:

```
$ fancy-bento-grid-generator sizes example/features.toml
canvas 1600×1000, scale 2: 3200×2000 output px

tile  cols  rows  x       y    w      h    out x  out y  out w  out h
1     1-3   1-3   40      40   750    545  80     80     1500   1090
2     4-4   1-2   810     40   236.5  357  1620   80     473    714
...
```

## Claude Code skill

[`.claude/skills/bento-grid`](.claude/skills/bento-grid/SKILL.md) teaches Claude Code how to use this tool. It is picked up automatically inside this repo. To use it everywhere:

```sh
mkdir -p ~/.claude/skills/bento-grid
curl -fsSL https://raw.githubusercontent.com/mjarkk/fancy-bento-grid-generator/main/.claude/skills/bento-grid/SKILL.md \
  -o ~/.claude/skills/bento-grid/SKILL.md
```

## Config

The canvas is split into `columns` × `rows` equal cells. Each tile claims a rectangle of cells; tiles may not overlap, and cells without a tile stay empty.

A tile has no layout of its own: its layers are drawn on top of each other, clipped to its rounded corners. Images and text are placed with a `position` keyword and `padding`, or at `x`/`y` pixels from the tile's top-left corner.

```toml
width = 1200
height = 810
columns = 4
rows = 3
background = "#0b0b0f"
tile_background = "#1c1c22"

[[tiles]]
cols = "1-2"
rows = "1-2"
layers = [
  { image = "photo.jpg" },
  { colors = ["#00000000", "#000000b3"] },
  { text = "Fast by default", font = "Inter-Bold.ttf", size = 40, color = "#fff", position = "bottom left", padding = 32 },
]

[[tiles]]
cols = "3-3"
rows = "1-1"
background = "#18122b"
layers = [{ image = "fast.svg" }]
```

See [`example/bento.toml`](example/bento.toml) for a complete grid, and [`example/features.toml`](example/features.toml) ([rendered](example/features.png)) for text, off-edge cards, shadows, borders and glows at scale 2.

### Canvas

| Key               | Default   | Description                                |
| ----------------- | --------- | ------------------------------------------ |
| `width`/`height`  | required  | Canvas size in logical pixels              |
| `scale`           | `1`       | Output pixels per logical pixel; see [Scale](#scale) |
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
| `require_full`    | `false`   | Make cells without a tile an error that lists them |

The sheet is the rectangle in `background` color that the tiles sit on. It fills the canvas unless `margin` or `sheet_radius` is set; the canvas outside it is transparent. Everything on the sheet, tile shadows included, is clipped to it.

Tile edges are rounded to whole output pixels, so cells don't need to come out as whole pixels. Gaps are rounded once and stay equal; cells absorb the rounding and may differ by a pixel.

Colors are `#rgb`, `#rrggbb` or `#rrggbbaa`.

#### Scale

All lengths in the config are logical pixels: the canvas size, gaps, padding, margins, radii, shadows, borders, layer `x`/`y`/`width`/`height`/`padding`, and text `size`/`letter_spacing`/`max_width`. The output is `scale` times as large, with every length multiplied, so a grid made for 1600×1000 renders at 3200×2000 with `scale = 2`.

SVGs are rasterized at their final size, so they stay sharp at any scale; with `fit = "none"` or `repeat`, one `viewBox` unit is one logical pixel. Raster images count one image pixel as one logical pixel too, so with `fit = "none"` they are upscaled. Give a high-resolution raster a `width` or `height` in logical pixels to draw it at its own resolution instead.

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

Like in CSS, a shadow is not drawn under the shape that casts it, so it doesn't darken a translucent tile or sheet. `shadow = false` turns a shadow off, for example to leave one tile without the canvas `tile_shadow`.

#### Border

A border along the inside of a tile's edge, drawn on top of its layers, written as an inline table:

```toml
tile_border = { color = "#ffffff1a" }   # a 1 px hairline on every tile
```

| Key     | Default  | Description |
| ------- | -------- | ----------- |
| `width` | `1`      | Width in pixels |
| `color` | required | Border color |

`border = false` leaves one tile without the canvas `tile_border`.

### Tile

| Key            | Default                  | Description                                  |
| -------------- | ------------------------ | -------------------------------------------- |
| `cols`/`rows`  | required                 | Cells covered as `"first-last"`, counting from 1: `"1-2"` for the first two, `"1-1"` for just the first |
| `background`   | canvas `tile_background` | Color under the layers                       |
| `radius`       | canvas `radius`          | Corner radius                                |
| `shadow`       | canvas `tile_shadow`     | [Shadow](#shadow), or `false` for none       |
| `border`       | canvas `tile_border`     | [Border](#border), or `false` for none       |
| `layers`       |                          | Layers, drawn first to last                  |

Run `fancy-bento-grid-generator sizes grid.toml` for each tile's size. Give a tile's SVG a `viewBox` of exactly its logical size (`viewBox="0 0 750 545"`) and the default `fit = "cover"` fills the tile edge to edge.

### Layer

A layer is an image when it has `image`, text when it has `text`, or a gradient when it has `colors`:

```toml
{ image = "logo.svg", fit = "contain", padding = 24, position = "bottom right" }
{ image = "dots.svg", repeat = "both", angle = 45, opacity = 0.4 }
{ image = "card.svg", x = 220, y = 22, width = 400, radius = 12, shadow = { y = 12, blur = 32 } }
{ text = "Release 4.2", font = "Inter-Bold.ttf", size = 48, color = "#fff", x = 32, y = 28 }
{ angle = 90, colors = ["#f472b6", "#c084fc"] }
{ gradient = "radial", position = "top left", colors = ["#7c3aed66", "#7c3aed00"] }
```

| Key        | Applies to | Default | Description |
| ---------- | ---------- | ------- | ----------- |
| `image`    | image      |         | Path to a PNG, JPEG, GIF, WebP or SVG |
| `fit`      | image      | `cover`, or `none` with `repeat` or `x`/`y` | `cover` scales to fill the tile and crops; `contain` fits it inside; `none` keeps its own size (see [Scale](#scale)). Not with `width`/`height` |
| `width`/`height` | image |        | Size in pixels. With only one of them the other keeps the aspect ratio |
| `repeat`   | image      |         | `x`, `y` or `both` to tile copies of the image outward from where it is placed. Not with `fit = "cover"`, where one copy already fills the tile |
| `radius`   | image, radial gradient | `0`, or the farthest corner | Rounds an image's corners. A radial gradient's radius |
| `position` | all but linear gradients | `center` | Where the image or text sits when it doesn't fill the tile exactly: `center`, an edge like `top`, or a corner like `bottom right`. A radial gradient's center |
| `padding`  | image, text | `0`    | Space in pixels between the tile's edge and the image or text, which is clipped to that inset |
| `x`/`y`    | all but linear gradients | | Pixels from the tile's top-left corner to the image's or text's top-left corner, or to a radial gradient's center. May be negative or run past the tile, which clips it. Setting one places by `x`/`y` instead of `position`, with the other `0`. Not with `position` or `padding` |
| `shadow`   | image, text |        | [Shadow](#shadow) that follows the layer's shape: a rounded card, an icon's outline, the letters of text |
| `opacity`  | image, text | `1`    | From `0` (invisible) to `1` (opaque) |
| `angle`    | all but radial gradients | `180` for a gradient, `0` otherwise | Degrees clockwise. A linear gradient runs in that direction, as in CSS: `0` bottom to top, `90` left to right, `180` top to bottom. Images and text are rotated by it |
| `text`     | text       |         | The text. `\n` starts a new line |
| `font`     | text       | required | Path to a TTF or OTF file. Pick the weight by picking the file |
| `size`     | text       | `16`    | Font size in pixels |
| `color`    | text       | `#000`  | Text color |
| `letter_spacing` | text |  `0`    | Extra pixels between letters; may be negative |
| `line_height` | text    | `1.2`   | Line height as a multiple of `size` |
| `max_width` | text      |         | Wrap lines between words to fit this many pixels |
| `align`    | text       | `left`  | `left`, `center` or `right`: how lines line up with each other |
| `colors`   | gradient   |         | Two or more colors, spread evenly |
| `gradient` | gradient   | `linear` | `linear`, or `radial` for circles spreading from a center |

A rotated image still follows its `fit`: `cover` leaves no gaps and `contain` shows the whole image. Repeats run along the image's rotated axes, so a stripe pattern with an `angle` gives diagonal stripes. An image placed by `x`/`y` turns about its own center.

Text is laid out like CSS: kerned, each line `line_height` × `size` tall with the letters centered in it, and the block as wide as its longest line. That block is what `position`, `padding` and `x`/`y` place.

## SVG support

SVGs are rendered with [oksvg](https://github.com/srwiley/oksvg), at the final size so they stay sharp. Paths, basic shapes, groups, transforms and gradients work. Watch out for:

- `<text>` is not rendered. Use a text layer on top, or convert text to outlines when exporting (e.g. Figma's "Outline text" SVG export option).
- Filters, masks and clip paths are skipped with a warning. Give the layer a `shadow` for drop shadows, and a `radius` for rounded corners.
- Rounded `<rect>`s need both `rx` and `ry`, and `scale()` needs both arguments: oksvg doesn't apply the SVG defaults for these.
