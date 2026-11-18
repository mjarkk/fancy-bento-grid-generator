package main

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Width          int     `toml:"width"`
	Height         int     `toml:"height"`
	Columns        int     `toml:"columns"`
	Rows           int     `toml:"rows"`
	Gap            float64 `toml:"gap"`
	Padding        float64 `toml:"padding"`
	Margin         float64 `toml:"margin"`
	Background     string  `toml:"background"`
	TileBackground string  `toml:"tile_background"`
	Radius         float64 `toml:"radius"`
	SheetRadius    float64 `toml:"sheet_radius"`
	SheetShadow    *Shadow `toml:"sheet_shadow"`
	TileShadow     *Shadow `toml:"tile_shadow"`
	Tiles          []Tile  `toml:"tiles"`
}

// Shadow works like CSS box-shadow: a Gaussian blur with a standard deviation
// of Blur/2, never drawn under the shape that casts it.
type Shadow struct {
	X     float64 `toml:"x"`
	Y     float64 `toml:"y"`
	Blur  float64 `toml:"blur"`
	Color string  `toml:"color"`
}

type Tile struct {
	Cols       cellRange `toml:"cols"`
	Rows       cellRange `toml:"rows"`
	Background string    `toml:"background"`
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

// Layer is an image (raster or SVG) when Image is set, or a linear gradient
// when Colors is set; never both.
type Layer struct {
	Image    string    `toml:"image"`
	Fit      string    `toml:"fit"`
	Position *position `toml:"position"`
	Padding  float64   `toml:"padding"`
	Repeat   string    `toml:"repeat"`
	Opacity  *float64  `toml:"opacity"`

	Colors []string `toml:"colors"`

	// Degrees clockwise. A gradient runs in that direction as in CSS, where nil
	// means 180 (top to bottom); an image is rotated by it, where nil means 0.
	Angle *float64 `toml:"angle"`
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
	return nil
}

func (t Tile) validate(columns, rows int) error {
	if err := t.Cols.validate("cols", columns); err != nil {
		return err
	}
	if err := t.Rows.validate("rows", rows); err != nil {
		return err
	}
	for j, l := range t.Layers {
		if err := l.validate(); err != nil {
			return fmt.Errorf("layer %d: %w", j+1, err)
		}
	}
	return nil
}

func (l Layer) validate() error {
	if (l.Image == "") == (l.Colors == nil) {
		return errors.New("set either image, or colors for a gradient")
	}
	if l.Image != "" {
		switch {
		case !slices.Contains([]string{"", "cover", "contain", "none"}, l.Fit):
			return fmt.Errorf("fit %q must be cover, contain or none", l.Fit)
		case !slices.Contains([]string{"", "x", "y", "both"}, l.Repeat):
			return fmt.Errorf("repeat %q must be x, y or both", l.Repeat)
		case l.Repeat != "" && l.Fit == "cover":
			return errors.New(`repeat has no effect with fit = "cover", as one copy already fills the tile`)
		case l.Padding < 0:
			return errors.New("padding can't be negative")
		case l.Opacity != nil && (*l.Opacity < 0 || *l.Opacity > 1):
			return fmt.Errorf("opacity %v must be between 0 and 1", *l.Opacity)
		}
		return nil
	}
	if l.Fit != "" || l.Position != nil || l.Padding != 0 || l.Repeat != "" || l.Opacity != nil {
		return errors.New("fit, position, padding, repeat and opacity only apply to images")
	}
	if len(l.Colors) < 2 {
		return errors.New("a gradient needs at least two colors")
	}
	return nil
}

func (s *Shadow) validate() error {
	if s != nil && s.Blur < 0 {
		return errors.New("blur can't be negative")
	}
	return nil
}

func (c *Config) cellSize() (w, h float64) {
	inset := 2 * (c.Margin + c.Padding)
	w = (float64(c.Width) - inset - float64(c.Columns-1)*c.Gap) / float64(c.Columns)
	h = (float64(c.Height) - inset - float64(c.Rows-1)*c.Gap) / float64(c.Rows)
	return w, h
}
