# fancy-bento-grid-generator

Describe a bento grid in a TOML file, get a PNG back. Each tile is a stack of layers: images (PNG, JPEG, GIF, WebP), SVGs and gradients.

![example](example/bento.png)

## Usage

```sh
go install github.com/mjarkk/fancy-bento-grid-generator@latest
fancy-bento-grid-generator example/bento.toml              # writes example/bento.png
fancy-bento-grid-generator -o out.png my-grid.toml
```

Without `-o`, the PNG goes next to the config with the same name. Image paths in the config are relative to the config file.

## Claude Code skill

[`.claude/skills/bento-grid`](.claude/skills/bento-grid/SKILL.md) teaches Claude Code how to use this tool. It is picked up automatically inside this repo. To use it everywhere:

```sh
mkdir -p ~/.claude/skills/bento-grid
curl -fsSL https://raw.githubusercontent.com/mjarkk/fancy-bento-grid-generator/main/.claude/skills/bento-grid/SKILL.md \
  -o ~/.claude/skills/bento-grid/SKILL.md
```

## Config

The canvas is split into `columns` × `rows` equal cells. Each tile claims a rectangle of cells; tiles may not overlap, and cells without a tile stay empty.

A tile has no layout of its own: its layers are drawn on top of each other, clipped to its rounded corners. An image can be inset, positioned, rotated or repeated, but to place text, icons or anything side by side, put them in one image or SVG made at the tile's size.

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
  { image = "hero.svg" },
]

[[tiles]]
cols = "3-3"
rows = "1-1"
background = "#18122b"
layers = [{ image = "fast.svg" }]
```

See [`example/bento.toml`](example/bento.toml) for a complete grid.

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

### Tile

| Key            | Default                  | Description                                  |
| -------------- | ------------------------ | -------------------------------------------- |
| `cols`/`rows`  | required                 | Cells covered as `"first-last"`, counting from 1: `"1-2"` for the first two, `"1-1"` for just the first |
| `background`   | canvas `tile_background` | Color under the layers                       |
| `layers`       |                          | Layers, drawn first to last                  |

A tile covering `n` columns is `n × cell + (n − 1) × gap` pixels wide, where `cell = (width − 2 × (margin + padding) − (columns − 1) × gap) / columns` (same for rows). Make SVGs with that aspect ratio so they line up exactly.

### Layer

A layer is an image when it has `image`, or a linear gradient when it has `colors`:

```toml
{ image = "logo.svg", fit = "contain", padding = 24, position = "bottom right" }
{ image = "dots.svg", repeat = "both", angle = 45, opacity = 0.4 }
{ angle = 90, colors = ["#f472b6", "#c084fc"] }
```

| Key        | Applies to | Default | Description |
| ---------- | ---------- | ------- | ----------- |
| `image`    | image      |         | Path to a PNG, JPEG, GIF, WebP or SVG |
| `fit`      | image      | `cover`, or `none` with `repeat` | `cover` scales to fill the tile and crops; `contain` fits it inside; `none` keeps its own size (pixels, or the SVG's `viewBox` units) |
| `position` | image      | `center` | Where the image sits when it doesn't fill the tile exactly: `center`, an edge like `top`, or a corner like `bottom right` |
| `padding`  | image      | `0`     | Space in pixels between the tile's edge and the image, which is clipped to that inset |
| `repeat`   | image      |         | `x`, `y` or `both` to tile copies of the image outward from `position`. Not with `fit = "cover"`, where one copy already fills the tile |
| `opacity`  | image      | `1`     | From `0` (invisible) to `1` (opaque) |
| `colors`   | gradient   |         | Two or more colors, spread evenly |
| `angle`    | both       | `180` for a gradient, `0` for an image | Degrees clockwise. A gradient runs in that direction, as in CSS: `0` bottom to top, `90` left to right, `180` top to bottom. An image is rotated by it |

A rotated image still follows its `fit`: `cover` leaves no gaps and `contain` shows the whole image. Repeats run along the image's rotated axes, so a stripe pattern with an `angle` gives diagonal stripes.

## SVG support

SVGs are rendered with [oksvg](https://github.com/srwiley/oksvg), at the final size so they stay sharp. Paths, basic shapes, groups, transforms and gradients work. Watch out for:

- `<text>` is not rendered. Convert text to outlines when exporting (e.g. Figma's "Outline text" SVG export option).
- Filters, masks and clip paths are skipped with a warning.
- Rounded `<rect>`s need both `rx` and `ry`, and `scale()` needs both arguments: oksvg doesn't apply the SVG defaults for these.
