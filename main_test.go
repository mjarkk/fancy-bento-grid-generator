package main

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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
	two := 2.0
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
		"image and colors":    {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Colors: []string{"#000", "#fff"}}}}}, "set either image, or colors"},
		"empty layer":         {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{}}}}, "set either image, or colors"},
		"unknown fit":         {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Fit: "fill"}}}}, "fit"},
		"unknown repeat":      {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Repeat: "yes"}}}}, "repeat"},
		"repeat with cover":   {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Fit: "cover", Repeat: "x"}}}}, "no effect"},
		"negative padding":    {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Padding: -1}}}}, "padding"},
		"opacity above 1":     {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Image: "a.png", Opacity: &two}}}}, "opacity"},
		"fit on gradient":     {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Colors: []string{"#000", "#fff"}, Fit: "cover"}}}}, "only apply to images"},
		"padding on gradient": {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Colors: []string{"#000", "#fff"}, Padding: 4}}}}, "only apply to images"},
		"one color gradient":  {[]Tile{{Cols: one, Rows: one, Layers: []Layer{{Colors: []string{"#000"}}}}}, "at least two colors"},
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
	if err := writePNG(filepath.Join(dir, "split.png"), split); err != nil {
		t.Fatal(err)
	}

	red, blue, black := color.NRGBA{0xff, 0, 0, 0xff}, color.NRGBA{0, 0, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}
	half, ninety := 0.5, 90.0
	type pixel struct {
		x, y int
		want color.NRGBA
	}
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
			for _, p := range tt.pixels {
				got := color.NRGBAModel.Convert(img.At(p.x, p.y)).(color.NRGBA)
				if !near(got, p.want) {
					t.Errorf("pixel %d,%d = %v, want %v", p.x, p.y, got, p.want)
				}
			}
		})
	}
}

func near(a, b color.NRGBA) bool {
	diff := func(x, y uint8) bool { return max(x, y)-min(x, y) <= 8 }
	return diff(a.R, b.R) && diff(a.G, b.G) && diff(a.B, b.B) && diff(a.A, b.A)
}

func TestSheetAndShadows(t *testing.T) {
	white, black := color.NRGBA{0xff, 0xff, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}
	var transparent color.NRGBA
	type pixel struct {
		x, y int
		want color.NRGBA
	}
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
			for _, p := range tt.pixels {
				got := color.NRGBAModel.Convert(img.At(p.x, p.y)).(color.NRGBA)
				if !near(got, p.want) {
					t.Errorf("pixel %d,%d = %v, want %v", p.x, p.y, got, p.want)
				}
			}
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
