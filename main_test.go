package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	deepwebp "github.com/deepteams/webp"
	xwebp "golang.org/x/image/webp"
)

func TestRenderExample(t *testing.T) {
	cfg, err := loadConfig("example/bento.toml")
	if err != nil {
		t.Fatal(err)
	}
	img, err := render(cfg, "example")
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1296 || b.Dy() != 906 {
		t.Fatalf("size = %dx%d, want 1296x906", b.Dx(), b.Dy())
	}
	if got := color.NRGBAModel.Convert(img.At(4, 4)); got != (color.NRGBA{}) {
		t.Errorf("margin = %v, want transparent", got)
	}
	if got := color.NRGBAModel.Convert(img.At(60, 453)); got != (color.NRGBA{0x0b, 0x0b, 0x0f, 0xff}) {
		t.Errorf("sheet background = %v, want #0b0b0f", got)
	}
}

func TestCellRangeFromTOML(t *testing.T) {
	valid := map[string]cellRange{
		`"1-1"`:     {1, 1},
		`"1-2"`:     {1, 2},
		`" 2 - 3 "`: {2, 3},
	}
	for value, want := range valid {
		var got struct{ Cols cellRange }
		if _, err := toml.Decode("cols = "+value, &got); err != nil {
			t.Errorf("cols = %s: %v", value, err)
		} else if got.Cols != want {
			t.Errorf("cols = %s: got %v, want %v", value, got.Cols, want)
		}
	}

	for _, value := range []string{`1`, `"1"`, `"1-"`, `"a-b"`, `"1-2-3"`, `1.5`, `[1, 2]`} {
		var got struct{ Cols cellRange }
		if _, err := toml.Decode("cols = "+value, &got); err == nil {
			t.Errorf("cols = %s: got %v, want an error", value, got.Cols)
		}
	}
}

func TestPositionFromTOML(t *testing.T) {
	valid := map[string]position{
		`"center"`:        {0, 0},
		`"top"`:           {0, -0.5},
		`"bottom right"`:  {0.5, 0.5},
		`"left  top"`:     {-0.5, -0.5},
		`"center bottom"`: {0, 0.5},
	}
	for value, want := range valid {
		var got struct{ Position *position }
		if _, err := toml.Decode("position = "+value, &got); err != nil {
			t.Errorf("position = %s: %v", value, err)
		} else if *got.Position != want {
			t.Errorf("position = %s: got %v, want %v", value, *got.Position, want)
		}
	}

	for _, value := range []string{`""`, `"middle"`, `"top bottom"`, `"left left"`, `"top left right"`, `1`} {
		var got struct{ Position *position }
		if _, err := toml.Decode("position = "+value, &got); err == nil {
			t.Errorf("position = %s: got %v, want an error", value, *got.Position)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	one := cellRange{1, 1}
	two, minusOne := 2.0, -1.0
	tests := map[string]struct {
		tiles []Tile
		want  string
	}{
		"missing position": {[]Tile{{Rows: one}}, "cols is required"},
		"outside grid":     {[]Tile{{Cols: cellRange{2, 3}, Rows: one}}, "cols 2-3 does not fit in a grid of 2"},
		"backwards range":  {[]Tile{{Cols: one, Rows: cellRange{2, 1}}}, "rows 2-1 ends before it starts"},
		"overlap": {
			[]Tile{{Cols: cellRange{1, 2}, Rows: one}, {Cols: cellRange{2, 2}, Rows: one}},
			"tile 2 overlaps tile 1 at col 2, row 1",
		},
		"image and colors":     {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Colors: []string{"#000", "#fff"}}}}}, "set one of image, text, or colors"},
		"empty layer":          {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{}}}}, "set one of image, text, or colors"},
		"unknown fit":          {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Fit: "fill"}}}}, "fit"},
		"unknown repeat":       {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Repeat: "yes"}}}}, "repeat"},
		"repeat with cover":    {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Fit: "cover", Repeat: "x"}}}}, "no effect"},
		"negative padding":     {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Padding: -1}}}}, "padding"},
		"opacity above 1":      {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Opacity: &two}}}}, "opacity"},
		"fit on gradient":      {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Colors: []string{"#000", "#fff"}, Fit: "cover"}}}}, "fit doesn't apply to gradients"},
		"padding on gradient":  {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Colors: []string{"#000", "#fff"}, Padding: 4}}}}, "padding doesn't apply to gradients"},
		"negative tile radius": {[]Tile{{Cols: one, Rows: one, Radius: &minusOne}}, "tile 1: radius can't be negative"},
		"tile border width":    {[]Tile{{Cols: one, Rows: one, Border: &Border{Color: "#fff"}}}, "tile 1: border: width must be positive"},
		"one color gradient":   {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Colors: []string{"#000"}}}}}, "at least two colors"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Width: 400, Height: 400, Columns: 2, Rows: 2, Background: "#000", TileBackground: "#000", Tiles: tt.tiles}
			err := cfg.validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestGradientDirection(t *testing.T) {
	ninety := 90.0
	tests := map[string]struct {
		angle          *float64
		darkX, darkY   int
		lightX, lightY int
	}{
		"default runs top to bottom": {nil, 50, 1, 50, 98},
		"90 runs left to right":      {&ninety, 1, 50, 98, 50},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000", Tiles: []Tile{{
				Cols: cellRange{1, 1}, Rows: cellRange{1, 1},
				Layers: []Layer{{Angle: tt.angle, Colors: []string{"#000", "#fff"}}},
			}}}
			img, err := render(cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			dark, _, _, _ := img.At(tt.darkX, tt.darkY).RGBA()
			light, _, _, _ := img.At(tt.lightX, tt.lightY).RGBA()
			if dark > 0x1000 || light < 0xf000 {
				t.Errorf("start = %#x, end = %#x, want near black and near white", dark, light)
			}
		})
	}
}

func TestImageLayer(t *testing.T) {
	split := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		split.Set(0, y, color.NRGBA{0xff, 0, 0, 0xff})
		split.Set(1, y, color.NRGBA{0, 0, 0xff, 0xff})
	}
	dir := t.TempDir()
	if err := writeImage(filepath.Join(dir, "split.png"), split, png.Encode); err != nil {
		t.Fatal(err)
	}

	red, blue, black := color.NRGBA{0xff, 0, 0, 0xff}, color.NRGBA{0, 0, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}
	white := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	half, ninety := 0.5, 90.0
	n := func(v float64) *float64 { return &v }
	tests := map[string]struct {
		layer  Layer
		pixels []pixel
	}{
		"none at top left keeps the image's size": {
			Layer{Fit: "none", Position: &position{-0.5, -0.5}},
			[]pixel{{0, 0, red}, {1, 1, blue}, {2, 0, black}, {50, 50, black}},
		},
		"repeat x defaults to the image's size": {
			Layer{Repeat: "x", Position: &position{-0.5, -0.5}},
			[]pixel{{0, 0, red}, {1, 0, blue}, {98, 1, red}, {99, 1, blue}, {50, 2, black}},
		},
		"padding insets the image": {
			Layer{Padding: 10},
			[]pixel{{5, 50, black}, {50, 5, black}, {20, 50, red}, {80, 50, blue}},
		},
		"angle rotates clockwise": {
			Layer{Angle: &ninety},
			[]pixel{{50, 10, red}, {50, 90, blue}},
		},
		"opacity blends with what is below": {
			Layer{Opacity: &half},
			[]pixel{{10, 50, color.NRGBA{0x80, 0, 0, 0xff}}, {90, 50, color.NRGBA{0, 0, 0x80, 0xff}}},
		},
		"x and y place the top-left corner and width and height set the size": {
			Layer{X: n(10), Y: n(20), Width: n(20), Height: n(10)},
			[]pixel{{11, 21, red}, {28, 28, blue}, {9, 25, black}, {20, 31, black}, {31, 25, black}},
		},
		"width alone keeps the aspect ratio": {
			Layer{X: n(0), Width: n(40)},
			[]pixel{{10, 35, red}, {30, 35, blue}, {10, 45, black}},
		},
		"negative x runs off the tile's edge": {
			Layer{X: n(-10), Width: n(40)},
			[]pixel{{0, 5, red}, {20, 5, blue}, {35, 5, black}},
		},
		"x and y with an angle turn the image about its own center": {
			Layer{X: n(20), Y: n(20), Width: n(40), Angle: &ninety},
			[]pixel{{40, 25, red}, {40, 55, blue}, {15, 40, black}},
		},
		"radius rounds the image's corners": {
			Layer{X: n(0), Width: n(100), Radius: n(30)},
			[]pixel{{2, 2, black}, {97, 97, black}, {2, 50, red}, {25, 2, red}},
		},
		"shadow falls outside the image and follows its alpha": {
			Layer{X: n(20), Y: n(20), Width: n(40), Radius: n(20), Shadow: &Shadow{Y: 30, Color: "#fff"}},
			[]pixel{{40, 70, white}, {40, 95, black}, {25, 40, red}, {22, 88, black}},
		},
		"shadow of an image running off the tile is cast from the part outside it too": {
			Layer{X: n(60), Y: n(10), Width: n(80), Height: n(40), Shadow: &Shadow{Y: 50, Blur: 8, Color: "#fff"}},
			[]pixel{{98, 80, white}, {80, 80, white}, {40, 80, black}},
		},
		"shadow = false draws none": {
			Layer{X: n(20), Y: n(20), Width: n(40), Shadow: &Shadow{off: true}},
			[]pixel{{40, 70, black}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.layer.Image = "split.png"
			cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000", Tiles: []Tile{{
				Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{tt.layer},
			}}}
			if err := cfg.validate(); err != nil {
				t.Fatal(err)
			}
			img, err := render(cfg, dir)
			if err != nil {
				t.Fatal(err)
			}
			checkPixels(t, img, tt.pixels)
		})
	}
}

type pixel struct {
	x, y int
	want color.NRGBA
}

func checkPixels(t *testing.T, img image.Image, pixels []pixel) {
	t.Helper()
	for _, p := range pixels {
		got := color.NRGBAModel.Convert(img.At(p.x, p.y)).(color.NRGBA)
		if !near(got, p.want) {
			t.Errorf("pixel %d,%d = %v, want %v", p.x, p.y, got, p.want)
		}
	}
}

func near(a, b color.NRGBA) bool {
	diff := func(x, y uint8) bool { return max(x, y)-min(x, y) <= 8 }
	return diff(a.R, b.R) && diff(a.G, b.G) && diff(a.B, b.B) && diff(a.A, b.A)
}

func TestSheetAndShadows(t *testing.T) {
	white, black := color.NRGBA{0xff, 0xff, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}
	var transparent color.NRGBA
	tests := map[string]struct {
		cfg    Config
		pixels []pixel
	}{
		"margin and sheet_radius leave the outside transparent": {
			Config{Margin: 10, SheetRadius: 20, Background: "#fff", TileBackground: "#000"},
			[]pixel{{5, 50, transparent}, {11, 11, transparent}, {50, 50, white}, {12, 50, white}},
		},
		"sheet_shadow is not drawn under a translucent sheet": {
			Config{Margin: 10, Background: "#ffffff80", TileBackground: "#000", SheetShadow: &Shadow{Y: 6, Color: "#000"}},
			[]pixel{{50, 5, transparent}, {50, 50, color.NRGBA{0xff, 0xff, 0xff, 0x80}}, {50, 93, black}},
		},
		"tile_shadow is not drawn under a translucent tile and stays on the sheet": {
			Config{Margin: 10, Padding: 5, Background: "#fff", TileBackground: "#00000000", TileShadow: &Shadow{Y: 10, Color: "#000"},
				Tiles: []Tile{{Cols: cellRange{1, 1}, Rows: cellRange{1, 1}}}},
			[]pixel{{50, 50, white}, {50, 88, black}, {50, 93, transparent}},
		},
		"shadow color defaults to translucent black": {
			Config{Margin: 10, Background: "#fff", TileBackground: "#000", SheetShadow: &Shadow{Y: 6}},
			[]pixel{{50, 93, color.NRGBA{0, 0, 0, 0x66}}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.Width, cfg.Height, cfg.Columns, cfg.Rows = 100, 100, 1, 1
			if err := cfg.validate(); err != nil {
				t.Fatal(err)
			}
			img, err := render(&cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			checkPixels(t, img, tt.pixels)
		})
	}
}

func TestShadowBlur(t *testing.T) {
	cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Margin: 30, Background: "#fff",
		SheetShadow: &Shadow{Y: 20, Blur: 8, Color: "#000"}}
	img, err := render(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	// The cast sheet's bottom edge runs along y = 90.
	alpha := func(y int) uint8 { return color.NRGBAModel.Convert(img.At(50, y)).(color.NRGBA).A }
	if a := alpha(82); a < 0xf0 {
		t.Errorf("alpha 8px inside the edge = %#x, want nearly opaque", a)
	}
	if a := alpha(90); a < 0x40 || a > 0xc0 {
		t.Errorf("alpha at the edge = %#x, want about half", a)
	}
	if a := alpha(98); a > 0x10 {
		t.Errorf("alpha 8px outside the edge = %#x, want nearly transparent", a)
	}
}

func TestValidateRejectsCanvas(t *testing.T) {
	tests := map[string]struct {
		edit func(*Config)
		want string
	}{
		"missing background":      {func(c *Config) { c.Background = "" }, "background is required"},
		"missing tile_background": {func(c *Config) { c.TileBackground = "" }, "tile_background is required"},
		"negative shadow blur":    {func(c *Config) { c.TileShadow = &Shadow{Blur: -1} }, "tile_shadow: blur can't be negative"},
		"zero scale":              {func(c *Config) { zero := 0.0; c.Scale = &zero }, "scale must be positive"},
		"border without color":    {func(c *Config) { c.TileBorder = &Border{Width: 1} }, "tile_border: color is required"},
		"zero border width":       {func(c *Config) { c.TileBorder = &Border{Color: "#fff"} }, "tile_border: width must be positive; use false for no border"},
		"require_full lists the empty cells": {
			func(c *Config) {
				c.Columns, c.Rows, c.RequireFull = 2, 2, true
				c.Tiles = []Tile{{Cols: cellRange{1, 2}, Rows: cellRange{1, 1}}}
			},
			"require_full: no tile covers col 1 row 2, col 2 row 2",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000"}
			tt.edit(&cfg)
			if err := cfg.validate(); err == nil || err.Error() != tt.want {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsLayers(t *testing.T) {
	n := func(v float64) *float64 { return &v }
	gradient := []string{"#000", "#fff"}
	tests := map[string]struct {
		layer Layer
		want  string
	}{
		"text without font":      {Layer{Text: "Hi"}, "text needs a font, the path to a TTF or OTF file"},
		"zero text size":         {Layer{Text: "Hi", Font: "a.ttf", Size: n(0)}, "size must be positive"},
		"zero line_height":       {Layer{Text: "Hi", Font: "a.ttf", LineHeight: n(0)}, "line_height must be positive"},
		"zero max_width":         {Layer{Text: "Hi", Font: "a.ttf", MaxWidth: n(0)}, "max_width must be positive"},
		"unknown align":          {Layer{Text: "Hi", Font: "a.ttf", Align: "justify"}, `align "justify" must be left, center or right`},
		"image keys on text":     {Layer{Text: "Hi", Font: "a.ttf", Fit: "cover", Radius: n(4)}, "fit and radius don't apply to text"},
		"text keys on an image":  {Layer{Image: "a.png", Font: "a.ttf", Size: n(4), Color: "#fff"}, "font, size and color don't apply to images"},
		"fit with width":         {Layer{Image: "a.png", Fit: "contain", Width: n(10)}, "fit doesn't apply when width or height sets the size"},
		"zero width":             {Layer{Image: "a.png", Width: n(0)}, "width and height must be positive"},
		"negative image radius":  {Layer{Image: "a.png", Radius: n(-1)}, "radius can't be negative"},
		"position with x":        {Layer{Image: "a.png", X: n(1), Position: &position{}}, "position doesn't apply to a layer placed by x/y, which count from the tile's top-left corner"},
		"padding with y on text": {Layer{Text: "Hi", Font: "a.ttf", Y: n(1), Padding: 4}, "padding doesn't apply to a layer placed by x/y, which count from the tile's top-left corner"},
		"negative layer blur":    {Layer{Image: "a.png", Shadow: &Shadow{Blur: -1}}, "shadow: blur can't be negative"},
		"shadow on gradient":     {Layer{Colors: gradient, Shadow: &Shadow{}}, "shadow doesn't apply to gradients"},
		"unknown gradient":       {Layer{Colors: gradient, Gradient: "conic"}, `gradient "conic" must be linear or radial`},
		"gradient on an image":   {Layer{Image: "a.png", Gradient: "radial"}, "gradient doesn't apply to images"},
		"center on linear":       {Layer{Colors: gradient, Position: &position{}, Radius: n(4)}, "position and radius don't apply to linear gradients"},
		"angle on radial":        {Layer{Colors: gradient, Gradient: "radial", Angle: n(90)}, "angle doesn't apply to radial gradients"},
		"position and x radial":  {Layer{Colors: gradient, Gradient: "radial", X: n(1), Position: &position{}}, "position doesn't apply to a gradient centered by x/y"},
		"zero radial radius":     {Layer{Colors: gradient, Gradient: "radial", Radius: n(0)}, "radius must be positive"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000",
				Tiles: []Tile{{Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{tt.layer}}}}
			if err := cfg.validate(); err == nil || err.Error() != "tile 1: layer 1: "+tt.want {
				t.Fatalf("err = %v, want %q", err, "tile 1: layer 1: "+tt.want)
			}
		})
	}
}

func TestShadowAndBorderFromTOML(t *testing.T) {
	var cfg struct {
		Tiles []Tile `toml:"tiles"`
	}
	md, err := toml.Decode(`
[[tiles]]
shadow = false
border = { color = "#fff" }

[[tiles]]
shadow = { y = 4, blur = 8 }
border = false
`, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		t.Fatalf("undecoded keys %v", undecoded)
	}
	canvasShadow := &Shadow{Y: 1}
	if s := cfg.Tiles[0].Shadow.or(canvasShadow); s != nil {
		t.Errorf("shadow = false gives %+v, want none", s)
	}
	if b := cfg.Tiles[0].Border; b == nil || b.Width != 1 || b.Color != "#fff" {
		t.Errorf("border = %+v, want width 1 by default", b)
	}
	if s := cfg.Tiles[1].Shadow.or(canvasShadow); s == nil || s.Y != 4 || s.Blur != 8 {
		t.Errorf("shadow = %+v, want the tile's own", s)
	}
	if s := (*Shadow)(nil).or(canvasShadow); s != canvasShadow {
		t.Errorf("unset shadow = %+v, want the canvas tile_shadow", s)
	}
	if b := cfg.Tiles[1].Border.or(&Border{Width: 1, Color: "#fff"}); b != nil {
		t.Errorf("border = false gives %+v, want none", b)
	}

	for _, value := range []string{`true`, `"none"`, `{ y = 1, spread = 2 }`, `{ y = "1" }`} {
		var got struct{ Shadow *Shadow }
		if _, err := toml.Decode("shadow = "+value, &got); err == nil {
			t.Errorf("shadow = %s: got %+v, want an error", value, got.Shadow)
		}
	}
}

func TestTileOverrides(t *testing.T) {
	white, black, gray := color.NRGBA{0xff, 0xff, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}, color.NRGBA{0x80, 0x80, 0x80, 0xff}
	n := func(v float64) *float64 { return &v }
	tests := map[string]struct {
		cfg    Config
		pixels []pixel
	}{
		"border is drawn inside the edge, on top of the layers": {
			Config{Radius: 0, Tiles: []Tile{{Border: &Border{Width: 4, Color: "#fff"}, Layers: []Layer{{Colors: []string{"#808080", "#808080"}}}}}},
			[]pixel{{1, 50, white}, {50, 98, white}, {6, 50, gray}},
		},
		"tile_border is every tile's default": {
			Config{Radius: 0, TileBorder: &Border{Width: 4, Color: "#fff"}, Tiles: []Tile{{}}},
			[]pixel{{1, 50, white}, {6, 50, black}},
		},
		"border = false drops tile_border": {
			Config{Radius: 0, TileBorder: &Border{Width: 4, Color: "#fff"}, Tiles: []Tile{{Border: &Border{off: true}}}},
			[]pixel{{1, 50, black}},
		},
		"radius overrides the canvas radius": {
			Config{Radius: 0, Tiles: []Tile{{Radius: n(30)}}},
			[]pixel{{2, 2, white}, {50, 2, black}},
		},
		"shadow = false drops tile_shadow": {
			Config{Padding: 20, TileShadow: &Shadow{Y: 15, Color: "#f00"}, Tiles: []Tile{{Shadow: &Shadow{off: true}}}},
			[]pixel{{50, 85, white}},
		},
		"shadow overrides tile_shadow and follows the tile's radius": {
			Config{Padding: 20, TileShadow: &Shadow{Y: 15, Color: "#f00"}, Tiles: []Tile{{Shadow: &Shadow{Y: 15, Color: "#00f"}}}},
			[]pixel{{50, 85, color.NRGBA{0, 0, 0xff, 0xff}}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.Width, cfg.Height, cfg.Columns, cfg.Rows = 100, 100, 1, 1
			cfg.Background, cfg.TileBackground = "#fff", "#000"
			for i := range cfg.Tiles {
				cfg.Tiles[i].Cols, cfg.Tiles[i].Rows = cellRange{1, 1}, cellRange{1, 1}
			}
			if err := cfg.validate(); err != nil {
				t.Fatal(err)
			}
			img, err := render(&cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			checkPixels(t, img, tt.pixels)
		})
	}
}

func TestRadialGradient(t *testing.T) {
	white, black := color.NRGBA{0xff, 0xff, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}
	n := func(v float64) *float64 { return &v }
	tests := map[string]struct {
		layer  Layer
		pixels []pixel
	}{
		"centered and reaching the farthest corner by default": {
			Layer{},
			[]pixel{{50, 50, white}, {0, 0, black}, {99, 99, black}, {0, 50, color.NRGBA{0x4b, 0x4b, 0x4b, 0xff}}},
		},
		"position moves the center": {
			Layer{Position: &position{-0.5, -0.5}},
			[]pixel{{0, 0, white}, {99, 99, black}},
		},
		"x and y move the center and radius sets its size": {
			Layer{X: n(20), Y: n(80), Radius: n(40)},
			[]pixel{{20, 80, white}, {20, 38, black}, {62, 80, black}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			l := tt.layer
			l.Gradient, l.Colors = "radial", []string{"#fff", "#000"}
			cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000",
				Tiles: []Tile{{Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{l}}}}
			if err := cfg.validate(); err != nil {
				t.Fatal(err)
			}
			img, err := render(cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			checkPixels(t, img, tt.pixels)
		})
	}
}

func TestGridSnapsToPixels(t *testing.T) {
	gapX := 0.0
	tests := map[string]struct {
		cfg        Config
		xs, ws     []float64
		gapBetween float64
	}{
		"fractional cells round, gaps stay equal": {
			Config{Width: 100, Columns: 3, Gap: 10},
			[]float64{0, 37, 73}, []float64{27, 26, 27}, 10,
		},
		"gap_x overrides gap": {
			Config{Width: 90, Columns: 3, Gap: 10, GapX: &gapX},
			[]float64{0, 30, 60}, []float64{30, 30, 30}, 0,
		},
		"scale rounds the scaled gap": {
			Config{Width: 100, Columns: 3, Gap: 5, Scale: ptr(1.5)},
			[]float64{0, 53, 105}, []float64{45, 44, 45}, 8,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.Height, cfg.Rows = 100, 1
			out := cfg.scaled()
			var prevEnd float64
			for i := range tt.xs {
				x, _, w, _ := out.tileRect(Tile{Cols: cellRange{i + 1, i + 1}, Rows: cellRange{1, 1}})
				if x != tt.xs[i] || w != tt.ws[i] {
					t.Errorf("tile %d: x, w = %v, %v, want %v, %v", i+1, x, w, tt.xs[i], tt.ws[i])
				}
				if i > 0 && x-prevEnd != tt.gapBetween {
					t.Errorf("gap before tile %d = %v, want %v", i+1, x-prevEnd, tt.gapBetween)
				}
				prevEnd = x + w
			}
			if x, _, w, _ := out.tileRect(Tile{Cols: cellRange{1, cfg.Columns}, Rows: cellRange{1, 1}}); x != 0 || w != float64(out.Width) {
				t.Errorf("tile spanning all columns: x, w = %v, %v, want 0, %d", x, w, out.Width)
			}
		})
	}

	gapY := 4.0
	cfg := Config{Width: 100, Height: 100, Columns: 2, Rows: 2, Gap: 10, GapY: &gapY}
	if _, y, _, _ := cfg.tileRect(Tile{Cols: cellRange{1, 1}, Rows: cellRange{2, 2}}); y != 52 {
		t.Errorf("second row with gap_y 4 starts at %v, want 52", y)
	}
	if x, _, _, _ := cfg.tileRect(Tile{Cols: cellRange{2, 2}, Rows: cellRange{1, 1}}); x != 55 {
		t.Errorf("second column with gap 10 starts at %v, want 55", x)
	}
}

func ptr(v float64) *float64 { return &v }

func TestScale(t *testing.T) {
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#f00"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "red.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	red, black, white := color.NRGBA{0xff, 0, 0, 0xff}, color.NRGBA{0, 0, 0, 0xff}, color.NRGBA{0xff, 0xff, 0xff, 0xff}
	cfg := &Config{Width: 50, Height: 50, Scale: ptr(2), Columns: 1, Rows: 1, Padding: 5, Radius: 0, Background: "#fff", TileBackground: "#000",
		Tiles: []Tile{{Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{
			{Image: "red.svg", Fit: "none", Position: &position{-0.5, -0.5}},
			{Image: "red.svg", X: ptr(25), Y: ptr(25), Width: ptr(5)},
		}}}}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	img, err := render(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 100 || b.Dy() != 100 {
		t.Fatalf("size = %dx%d, want 100x100", b.Dx(), b.Dy())
	}
	checkPixels(t, img, []pixel{
		{9, 50, white}, {10, 10, red}, {29, 29, red}, {31, 31, black}, // padding and the viewBox are doubled
		{61, 61, red}, {69, 69, red}, {71, 71, black}, // so are x, y and width, which count from the tile's corner
	})
}

func TestSizes(t *testing.T) {
	cfg := &Config{Width: 100, Height: 50, Scale: ptr(1.5), Columns: 3, Rows: 1, Gap: 5, Padding: 0,
		Tiles: []Tile{{Cols: cellRange{1, 2}, Rows: cellRange{1, 1}}, {Cols: cellRange{3, 3}, Rows: cellRange{1, 1}}}}
	var table strings.Builder
	if err := printSizes(&table, cfg, false); err != nil {
		t.Fatal(err)
	}
	want := `canvas 100×50, scale 1.5: 150×75 output px

tile  cols  rows  x   y  w      h   out x  out y  out w  out h
1     1-2   1-1   0   0  64.67  50  0      0      97     75
2     3-3   1-1   70  0  30     50  105    0      45     75
`
	if table.String() != want {
		t.Errorf("table =\n%s\nwant\n%s", table.String(), want)
	}

	var out bytes.Buffer
	if err := printSizes(&out, cfg, true); err != nil {
		t.Fatal(err)
	}
	var got struct {
		OutputWidth int `json:"output_width"`
		Tiles       []tileSize
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OutputWidth != 150 || len(got.Tiles) != 2 || got.Tiles[0].Output != (rect{0, 0, 97, 75}) || got.Tiles[0].Logical.W != 64.67 {
		t.Errorf("json = %s", out.String())
	}
}

func TestParseFlagsAfterArguments(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	scale := scaleFlag(fs)
	asJSON := fs.Bool("json", false, "")
	args := parseFlags(fs, []string{"grid.toml", "--scale", "2", "-json"})
	if len(args) != 1 || args[0] != "grid.toml" || *scale == nil || **scale != 2 || !*asJSON {
		t.Errorf("args = %v, scale = %v, json = %v", args, *scale, *asJSON)
	}
}

func TestOutputFormats(t *testing.T) {
	cfg := &Config{Width: 40, Height: 40, Columns: 1, Rows: 1, Margin: 10, Background: "#808080", TileBackground: "#808080"}
	img, err := render(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	gray := color.NRGBA{0x80, 0x80, 0x80, 0xff}
	dir := t.TempDir()
	tests := map[string]struct {
		quality     int
		decode      func(io.Reader) (image.Image, error)
		margin, mid color.NRGBA
	}{
		"out.png":  {0, png.Decode, color.NRGBA{}, gray},
		"out.JPG":  {80, jpeg.Decode, color.NRGBA{0xff, 0xff, 0xff, 0xff}, gray},
		"out.jpeg": {0, jpeg.Decode, color.NRGBA{0xff, 0xff, 0xff, 0xff}, gray},
		"out.webp": {0, xwebp.Decode, color.NRGBA{}, gray},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			encode, err := encoder(name, tt.quality)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, name)
			if err := writeImage(path, img, encode); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := tt.decode(f)
			if err != nil {
				t.Fatal(err)
			}
			checkPixels(t, got, []pixel{{2, 2, tt.margin}, {20, 20, tt.mid}})
		})
	}

	for path, want := range map[string]string{
		"out.avif": "can't write out.avif: the output extension must be .png, .jpg, .jpeg or .webp",
		"out":      "can't write out: the output extension must be .png, .jpg, .jpeg or .webp",
	} {
		if _, err := encoder(path, 0); err == nil || err.Error() != want {
			t.Errorf("encoder(%q) err = %v, want %q", path, err, want)
		}
	}
	if _, err := encoder("out.png", 90); err == nil || !strings.Contains(err.Error(), "-quality only applies to .jpg, .jpeg and .webp") {
		t.Errorf("quality with png: err = %v", err)
	}
}

func TestRenderFeaturesExample(t *testing.T) {
	cfg, err := loadConfig("example/features.toml")
	if err != nil {
		t.Fatal(err)
	}
	img, err := render(cfg, "example")
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 3200 || b.Dy() != 2000 {
		t.Fatalf("size = %dx%d, want 3200x2000 at scale 2", b.Dx(), b.Dy())
	}
}

func TestWebPInputsDecodeAsBefore(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := range src.Pix {
		src.Pix[i] = uint8(i * 7)
	}
	var buf bytes.Buffer
	if err := deepwebp.Encode(&buf, src, &deepwebp.EncoderOptions{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	got, err := decodeRaster(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	want, err := xwebp.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for y := range 32 {
		for x := range 32 {
			if got.At(x, y) != want.At(x, y) {
				t.Fatalf("pixel %d,%d = %v, want %v from x/image/webp", x, y, got.At(x, y), want.At(x, y))
			}
		}
	}
}
