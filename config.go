package main

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is in logical pixels; scaled converts it to output pixels.
type Config struct {
	Width          int      `toml:"width"`
	Height         int      `toml:"height"`
	Scale          *float64 `toml:"scale"`
	Columns        int      `toml:"columns"`
	Rows           int      `toml:"rows"`
	Gap            float64  `toml:"gap"`
	GapX           *float64 `toml:"gap_x"`
	GapY           *float64 `toml:"gap_y"`
	Padding        float64  `toml:"padding"`
	Margin         float64  `toml:"margin"`
	Background     string   `toml:"background"`
	TileBackground string   `toml:"tile_background"`
	Radius         float64  `toml:"radius"`
	SheetRadius    float64  `toml:"sheet_radius"`
	SheetShadow    *Shadow  `toml:"sheet_shadow"`
	TileShadow     *Shadow  `toml:"tile_shadow"`
	TileBorder     *Border  `toml:"tile_border"`
	RequireFull    bool     `toml:"require_full"`
	Tiles          []Tile   `toml:"tiles"`
}

// Shadow works like CSS box-shadow: a Gaussian blur with a standard deviation
// of Blur/2, never drawn under the shape that casts it. In TOML it is an inline
// table, or false for no shadow.
type Shadow struct {
	X, Y, Blur float64
	Color      string
	off        bool
}

func (s *Shadow) UnmarshalTOML(v any) error {
	return decodeTable(v, &s.off, map[string]any{"x": &s.X, "y": &s.Y, "blur": &s.Blur, "color": &s.Color})
}

// or returns s, or def when s is unset, and nil when that one is false.
func (s *Shadow) or(def *Shadow) *Shadow {
	if s = cmp.Or(s, def); s != nil && s.off {
		return nil
	}
	return s
}

func (s *Shadow) validate() error {
	if s != nil && s.Blur < 0 {
		return errors.New("blur can't be negative")
	}
	return nil
}

// Border is drawn on top of a tile's layers, along the inside of its edge. In
// TOML it is an inline table, or false for no border.
type Border struct {
	Width float64
	Color string
	off   bool
}

func (b *Border) UnmarshalTOML(v any) error {
	*b = Border{Width: 1}
	return decodeTable(v, &b.off, map[string]any{"width": &b.Width, "color": &b.Color})
}

// or returns b, or def when b is unset, and nil when that one is false.
func (b *Border) or(def *Border) *Border {
	if b = cmp.Or(b, def); b != nil && b.off {
		return nil
	}
	return b
}

func (b *Border) validate() error {
	switch {
	case b == nil || b.off:
		return nil
	case b.Width <= 0:
		return errors.New("width must be positive; use false for no border")
	case b.Color == "":
		return errors.New("color is required")
	}
	return nil
}

// decodeTable decodes an inline table into fields, which maps each key to a
// *float64 or *string, or sets *off for false.
func decodeTable(v any, off *bool, fields map[string]any) error {
	switch v := v.(type) {
	case bool:
		if !v {
			*off = true
			return nil
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(v)) {
			switch dst := fields[k].(type) {
			case nil:
				return fmt.Errorf("unknown key %q", k)
			case *float64:
				switch n := v[k].(type) {
				case int64:
					*dst = float64(n)
				case float64:
					*dst = n
				default:
					return fmt.Errorf("%s must be a number", k)
				}
			case *string:
				s, ok := v[k].(string)
				if !ok {
					return fmt.Errorf("%s must be a string", k)
				}
				*dst = s
			}
		}
		return nil
	}
	return fmt.Errorf("invalid value %#v, want an inline table or false", v)
}

type Tile struct {
	Cols       cellRange `toml:"cols"`
	Rows       cellRange `toml:"rows"`
	Background string    `toml:"background"`
	Radius     *float64  `toml:"radius"`
	Shadow     *Shadow   `toml:"shadow"`
	Border     *Border   `toml:"border"`
	Layers     []Layer   `toml:"layers"`
}

// cellRange is an inclusive range of 1-based grid cells, written in TOML as
// "first-last"; a single cell is "3-3".
type cellRange struct{ first, last int }

func (r *cellRange) UnmarshalTOML(v any) error {
	if s, ok := v.(string); ok {
		if first, last, ok := strings.Cut(s, "-"); ok {
			var errFirst, errLast error
			r.first, errFirst = strconv.Atoi(strings.TrimSpace(first))
			r.last, errLast = strconv.Atoi(strings.TrimSpace(last))
			if errFirst == nil && errLast == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("invalid cell range %#v, want \"first-last\" like \"1-2\", or \"3-3\" for a single cell", v)
}

func (r cellRange) String() string {
	return fmt.Sprintf("%d-%d", r.first, r.last)
}

func (r cellRange) validate(name string, gridSize int) error {
	switch {
	case r.first < 1:
		return fmt.Errorf("%s is required and starts at 1", name)
	case r.last < r.first:
		return fmt.Errorf("%s %s ends before it starts", name, r)
	case r.last > gridSize:
		return fmt.Errorf("%s %s does not fit in a grid of %d", name, r, gridSize)
	}
	return nil
}

// Layer is an image (raster or SVG) when Image is set, text when Text is set,
// or a gradient when Colors is set; exactly one of them.
type Layer struct {
	Image  string   `toml:"image"`
	Fit    string   `toml:"fit"`
	Repeat string   `toml:"repeat"`
	Width  *float64 `toml:"width"`
	Height *float64 `toml:"height"`

	Text          string   `toml:"text"`
	Font          string   `toml:"font"`
	Size          *float64 `toml:"size"`
	Color         string   `toml:"color"`
	LetterSpacing float64  `toml:"letter_spacing"`
	LineHeight    *float64 `toml:"line_height"`
	MaxWidth      *float64 `toml:"max_width"`
	Align         string   `toml:"align"`

	Colors   []string `toml:"colors"`
	Gradient string   `toml:"gradient"`

	// An image or text is placed from the tile's top-left corner when X or Y
	// is set, the other one defaulting to 0, and by Position inside Padding
	// otherwise. A radial gradient is centered the same way.
	Position *position `toml:"position"`
	Padding  float64   `toml:"padding"`
	X        *float64  `toml:"x"`
	Y        *float64  `toml:"y"`
	Opacity  *float64  `toml:"opacity"`
	Shadow   *Shadow   `toml:"shadow"`

	// An image's corner radius, or a radial gradient's radius, where nil
	// reaches the farthest corner of the tile.
	Radius *float64 `toml:"radius"`

	// Degrees clockwise. A gradient runs in that direction as in CSS, where nil
	// means 180 (top to bottom); an image or text is rotated by it, where nil
	// means 0.
	Angle *float64 `toml:"angle"`
}

func (l Layer) placedByXY() bool {
	return l.X != nil || l.Y != nil
}

// position is an offset from the center, as a fraction of the room the image
// leaves free: -0.5 is flush with the left or top edge, 0.5 with the right or
// bottom.
type position struct{ x, y float64 }

func (p *position) UnmarshalTOML(v any) error {
	s, _ := v.(string)
	words := strings.Fields(s)
	ok := len(words) == 1 || len(words) == 2
	var horizontal, vertical int
	for _, w := range words {
		switch w {
		case "left":
			p.x, horizontal = -0.5, horizontal+1
		case "right":
			p.x, horizontal = 0.5, horizontal+1
		case "top":
			p.y, vertical = -0.5, vertical+1
		case "bottom":
			p.y, vertical = 0.5, vertical+1
		case "center":
		default:
			ok = false
		}
	}
	if !ok || horizontal > 1 || vertical > 1 {
		return fmt.Errorf(`invalid position %#v, want "center", an edge like "top", or a corner like "bottom right"`, v)
	}
	return nil
}

func loadConfig(path string) (*Config, error) {
	// Defaults are set before decoding so an explicit 0 in the file still overrides them.
	cfg := &Config{
		Gap:     16,
		Padding: 16,
		Radius:  24,
	}
	md, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if unknown := md.Undecoded(); len(unknown) > 0 {
		return nil, fmt.Errorf("parse %s: unknown key %q", path, unknown[0].String())
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Width <= 0 || c.Height <= 0 {
		return errors.New("width and height must be positive")
	}
	if c.Scale != nil && *c.Scale <= 0 {
		return errors.New("scale must be positive")
	}
	if c.Columns <= 0 || c.Rows <= 0 {
		return errors.New("columns and rows must be positive")
	}
	if c.Background == "" {
		return errors.New("background is required")
	}
	if c.TileBackground == "" {
		return errors.New("tile_background is required")
	}
	if w, h := c.cellSize(); w <= 0 || h <= 0 {
		return errors.New("margin, padding and gap leave no room for cells")
	}
	if err := c.SheetShadow.validate(); err != nil {
		return fmt.Errorf("sheet_shadow: %w", err)
	}
	if err := c.TileShadow.validate(); err != nil {
		return fmt.Errorf("tile_shadow: %w", err)
	}
	if err := c.TileBorder.validate(); err != nil {
		return fmt.Errorf("tile_border: %w", err)
	}

	occupied := make([][]int, c.Rows)
	for r := range occupied {
		occupied[r] = make([]int, c.Columns)
	}
	for i, t := range c.Tiles {
		if err := t.validate(c.Columns, c.Rows); err != nil {
			return fmt.Errorf("tile %d: %w", i+1, err)
		}
		for r := t.Rows.first; r <= t.Rows.last; r++ {
			for col := t.Cols.first; col <= t.Cols.last; col++ {
				if other := occupied[r-1][col-1]; other != 0 {
					return fmt.Errorf("tile %d overlaps tile %d at col %d, row %d", i+1, other, col, r)
				}
				occupied[r-1][col-1] = i + 1
			}
		}
	}
	if c.RequireFull {
		var empty []string
		for r, row := range occupied {
			for col, tile := range row {
				if tile == 0 {
					empty = append(empty, fmt.Sprintf("col %d row %d", col+1, r+1))
				}
			}
		}
		if len(empty) > 0 {
			return fmt.Errorf("require_full: no tile covers %s", strings.Join(empty, ", "))
		}
	}
	return nil
}

func (t Tile) validate(columns, rows int) error {
	if err := t.Cols.validate("cols", columns); err != nil {
		return err
	}
	if err := t.Rows.validate("rows", rows); err != nil {
		return err
	}
	if t.Radius != nil && *t.Radius < 0 {
		return errors.New("radius can't be negative")
	}
	if err := t.Shadow.validate(); err != nil {
		return fmt.Errorf("shadow: %w", err)
	}
	if err := t.Border.validate(); err != nil {
		return fmt.Errorf("border: %w", err)
	}
	for j, l := range t.Layers {
		if err := l.validate(); err != nil {
			return fmt.Errorf("layer %d: %w", j+1, err)
		}
	}
	return nil
}

func (l Layer) validate() error {
	kinds := 0
	for _, set := range []bool{l.Image != "", l.Text != "", l.Colors != nil} {
		if set {
			kinds++
		}
	}
	if kinds != 1 {
		return errors.New("set one of image, text, or colors for a gradient")
	}

	imageKeys := []key{{"fit", l.Fit != ""}, {"repeat", l.Repeat != ""}, {"width", l.Width != nil}, {"height", l.Height != nil}}
	textKeys := []key{{"font", l.Font != ""}, {"size", l.Size != nil}, {"color", l.Color != ""}, {"letter_spacing", l.LetterSpacing != 0},
		{"line_height", l.LineHeight != nil}, {"max_width", l.MaxWidth != nil}, {"align", l.Align != ""}}
	gradientKeys := []key{{"gradient", l.Gradient != ""}}

	switch {
	case l.Colors != nil:
		return l.validateGradient(slices.Concat(imageKeys, textKeys))
	case l.Text != "":
		if err := rejectKeys("text", slices.Concat(imageKeys, gradientKeys, []key{{"radius", l.Radius != nil}})...); err != nil {
			return err
		}
		if err := l.validatePlacement(); err != nil {
			return err
		}
		return l.validateText()
	}
	if err := rejectKeys("images", slices.Concat(textKeys, gradientKeys)...); err != nil {
		return err
	}
	if err := l.validatePlacement(); err != nil {
		return err
	}
	sized := l.Width != nil || l.Height != nil
	switch {
	case !slices.Contains([]string{"", "cover", "contain", "none"}, l.Fit):
		return fmt.Errorf("fit %q must be cover, contain or none", l.Fit)
	case !slices.Contains([]string{"", "x", "y", "both"}, l.Repeat):
		return fmt.Errorf("repeat %q must be x, y or both", l.Repeat)
	case l.Repeat != "" && l.Fit == "cover":
		return errors.New(`repeat has no effect with fit = "cover", as one copy already fills the tile`)
	case sized && l.Fit != "":
		return errors.New("fit doesn't apply when width or height sets the size")
	case l.Width != nil && *l.Width <= 0, l.Height != nil && *l.Height <= 0:
		return errors.New("width and height must be positive")
	case l.Radius != nil && *l.Radius < 0:
		return errors.New("radius can't be negative")
	}
	return nil
}

func (l Layer) validatePlacement() error {
	if l.placedByXY() {
		if err := rejectKeys("a layer placed by x/y, which count from the tile's top-left corner",
			key{"position", l.Position != nil}, key{"padding", l.Padding != 0}); err != nil {
			return err
		}
	}
	switch {
	case l.Padding < 0:
		return errors.New("padding can't be negative")
	case l.Opacity != nil && (*l.Opacity < 0 || *l.Opacity > 1):
		return fmt.Errorf("opacity %v must be between 0 and 1", *l.Opacity)
	}
	if err := l.Shadow.validate(); err != nil {
		return fmt.Errorf("shadow: %w", err)
	}
	return nil
}

func (l Layer) validateText() error {
	switch {
	case l.Font == "":
		return errors.New("text needs a font, the path to a TTF or OTF file")
	case l.Size != nil && *l.Size <= 0:
		return errors.New("size must be positive")
	case l.LineHeight != nil && *l.LineHeight <= 0:
		return errors.New("line_height must be positive")
	case l.MaxWidth != nil && *l.MaxWidth <= 0:
		return errors.New("max_width must be positive")
	case !slices.Contains([]string{"", "left", "center", "right"}, l.Align):
		return fmt.Errorf("align %q must be left, center or right", l.Align)
	}
	return nil
}

func (l Layer) validateGradient(notForGradients []key) error {
	notForGradients = append(notForGradients, key{"padding", l.Padding != 0}, key{"opacity", l.Opacity != nil}, key{"shadow", l.Shadow != nil})
	if err := rejectKeys("gradients", notForGradients...); err != nil {
		return err
	}
	if len(l.Colors) < 2 {
		return errors.New("a gradient needs at least two colors")
	}
	switch l.Gradient {
	case "", "linear":
		return rejectKeys("linear gradients", key{"position", l.Position != nil}, key{"x", l.X != nil}, key{"y", l.Y != nil}, key{"radius", l.Radius != nil})
	case "radial":
		if err := rejectKeys("radial gradients", key{"angle", l.Angle != nil}); err != nil {
			return err
		}
		if err := rejectKeys("a gradient centered by x/y", key{"position", l.placedByXY() && l.Position != nil}); err != nil {
			return err
		}
		if l.Radius != nil && *l.Radius <= 0 {
			return errors.New("radius must be positive")
		}
		return nil
	}
	return fmt.Errorf("gradient %q must be linear or radial", l.Gradient)
}

type key struct {
	name string
	set  bool
}

// rejectKeys names the keys that are set and don't apply to what.
func rejectKeys(what string, keys ...key) error {
	var names []string
	for _, k := range keys {
		if k.set {
			names = append(names, k.name)
		}
	}
	switch len(names) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("%s doesn't apply to %s", names[0], what)
	}
	return fmt.Errorf("%s and %s don't apply to %s", strings.Join(names[:len(names)-1], ", "), names[len(names)-1], what)
}

func (c *Config) scale() float64 {
	if c.Scale == nil {
		return 1
	}
	return *c.Scale
}

func (c *Config) gaps() (x, y float64) {
	return *cmp.Or(c.GapX, &c.Gap), *cmp.Or(c.GapY, &c.Gap)
}

func (c *Config) cellSize() (w, h float64) {
	gapX, gapY := c.gaps()
	inset := 2 * (c.Margin + c.Padding)
	w = (float64(c.Width) - inset - float64(c.Columns-1)*gapX) / float64(c.Columns)
	h = (float64(c.Height) - inset - float64(c.Rows-1)*gapY) / float64(c.Rows)
	return w, h
}

// tileRect is t's rectangle, with its edges on whole pixels.
func (c *Config) tileRect(t Tile) (x, y, w, h float64) {
	gapX, gapY := c.gaps()
	x, w = c.span(t.Cols, c.Width, c.Columns, gapX)
	y, h = c.span(t.Rows, c.Height, c.Rows, gapY)
	return x, y, w, h
}

// span places cells along one axis. The gap is rounded on its own, so all gaps
// stay equal and the cells absorb the rounding, differing by at most a pixel.
func (c *Config) span(cells cellRange, size, count int, gap float64) (start, length float64) {
	inset := c.Margin + c.Padding
	lo, hi := math.Round(inset), math.Round(float64(size)-inset)
	gap = math.Round(gap)
	cell := (hi - lo - float64(count-1)*gap) / float64(count)
	edge := func(i int) float64 { return lo + math.Round(float64(i)*cell) + float64(i)*gap }
	start = edge(cells.first - 1)
	return start, edge(cells.last) - gap - start
}

// scaled returns a copy of c with the canvas and every length multiplied by
// its scale, so rendering works in output pixels.
func (c *Config) scaled() *Config {
	out := *c
	s := c.scale()
	out.Width, out.Height = round(float64(out.Width)*s), round(float64(out.Height)*s)
	out.Gap *= s
	out.GapX, out.GapY = scaleLength(out.GapX, s), scaleLength(out.GapY, s)
	out.Padding *= s
	out.Margin *= s
	out.Radius *= s
	out.SheetRadius *= s
	out.SheetShadow, out.TileShadow = out.SheetShadow.scaled(s), out.TileShadow.scaled(s)
	out.TileBorder = out.TileBorder.scaled(s)
	out.Tiles = slices.Clone(out.Tiles)
	for i := range out.Tiles {
		t := &out.Tiles[i]
		t.Radius = scaleLength(t.Radius, s)
		t.Shadow, t.Border = t.Shadow.scaled(s), t.Border.scaled(s)
		t.Layers = slices.Clone(t.Layers)
		for j := range t.Layers {
			l := &t.Layers[j]
			l.Width, l.Height = scaleLength(l.Width, s), scaleLength(l.Height, s)
			l.Size, l.MaxWidth = scaleLength(l.Size, s), scaleLength(l.MaxWidth, s)
			l.LetterSpacing *= s
			l.Padding *= s
			l.X, l.Y = scaleLength(l.X, s), scaleLength(l.Y, s)
			l.Shadow = l.Shadow.scaled(s)
			l.Radius = scaleLength(l.Radius, s)
		}
	}
	return &out
}

func scaleLength(v *float64, s float64) *float64 {
	if v == nil {
		return nil
	}
	scaled := *v * s
	return &scaled
}

func (s *Shadow) scaled(by float64) *Shadow {
	if s == nil {
		return nil
	}
	scaled := *s
	scaled.X, scaled.Y, scaled.Blur = s.X*by, s.Y*by, s.Blur*by
	return &scaled
}

func (b *Border) scaled(by float64) *Border {
	if b == nil {
		return nil
	}
	scaled := *b
	scaled.Width *= by
	return &scaled
}
