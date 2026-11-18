package main

import (
	"cmp"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func goFontDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.ttf"), goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func layoutText(t *testing.T, l Layer) *textBlock {
	t.Helper()
	l.Font = "go.ttf"
	r := &renderer{cfg: &Config{}, baseDir: goFontDir(t), fonts: map[string]*sfnt.Font{}}
	block, err := r.text(l)
	if err != nil {
		t.Fatal(err)
	}
	return block
}

func TestTextLayout(t *testing.T) {
	n := func(v float64) *float64 { return &v }
	one := layoutText(t, Layer{Text: "Hello world", Size: n(20)})
	oneW, oneH := one.size()
	if oneH != 24 {
		t.Errorf("height of one line = %v, want 24 from the default line_height of 1.2", oneH)
	}

	wrapped := layoutText(t, Layer{Text: "Hello world", Size: n(20), MaxWidth: n(oneW - 1)})
	if strings.Join(wrapped.lines, "|") != "Hello|world" {
		t.Errorf("lines with max_width = %q, want Hello and world", wrapped.lines)
	}
	if fits := layoutText(t, Layer{Text: "Hello world", Size: n(20), MaxWidth: n(oneW)}); len(fits.lines) != 1 {
		t.Errorf("lines with max_width of the whole text = %q, want one", fits.lines)
	}

	broken := layoutText(t, Layer{Text: "Hello\nworld", Size: n(20), LineHeight: n(1.5)})
	if _, h := broken.size(); len(broken.lines) != 2 || h != 60 {
		t.Errorf("explicit newline gives %q, %v tall, want two lines 60 tall", broken.lines, h)
	}

	spaced := layoutText(t, Layer{Text: "Hello world", Size: n(20), LetterSpacing: 2})
	if w, _ := spaced.size(); w != oneW+10*2 {
		t.Errorf("width with letter_spacing = %v, want %v plus 2 between each of 11 runes", w, oneW)
	}

	kerned := layoutText(t, Layer{Text: "AV", Size: n(20)})
	plain := kerned.walk("AV", nil)
	kerned.face = fixedKern{kerned.face, -fixed.I(3)}
	if got := kerned.walk("AV", nil); got != plain-fixed.I(3) {
		t.Errorf("kerned width = %v, want %v", got, plain-fixed.I(3))
	}
}

type fixedKern struct {
	font.Face
	kern fixed.Int26_6
}

func (f fixedKern) Kern(rune, rune) fixed.Int26_6 { return f.kern }

func TestTextLayer(t *testing.T) {
	white, black := color.NRGBA{0xff, 0xff, 0xff, 0xff}, color.NRGBA{0, 0, 0, 0xff}
	n := func(v float64) *float64 { return &v }
	ink := func(img image.Image, area image.Rectangle) bool {
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				if r, _, _, _ := img.At(x, y).RGBA(); r > 0x8000 {
					return true
				}
			}
		}
		return false
	}
	tests := map[string]struct {
		layer      Layer
		inked, not image.Rectangle
	}{
		"x and y place the box's top-left corner": {
			Layer{X: n(10), Y: n(10)},
			image.Rect(10, 10, 60, 40), image.Rect(0, 50, 200, 100),
		},
		"position and padding place the box like an image": {
			Layer{Position: &position{0.5, 0.5}, Padding: 10},
			image.Rect(100, 50, 190, 90), image.Rect(0, 0, 200, 40),
		},
		"align right lines up the lines' ends": {
			Layer{Text: "Hi\nHello", Align: "right", X: n(10), Y: n(10)},
			image.Rect(40, 10, 60, 30), image.Rect(10, 10, 30, 30),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			l := tt.layer
			l.Text = cmp.Or(l.Text, "Hello")
			l.Font, l.Size, l.Color = "go.ttf", n(24), "#fff"
			cfg := &Config{Width: 200, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000", Tiles: []Tile{{
				Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{l},
			}}}
			if err := cfg.validate(); err != nil {
				t.Fatal(err)
			}
			img, err := render(cfg, goFontDir(t))
			if err != nil {
				t.Fatal(err)
			}
			if !ink(img, tt.inked) {
				t.Errorf("no text in %v", tt.inked)
			}
			if ink(img, tt.not) {
				t.Errorf("text in %v", tt.not)
			}
		})
	}

	t.Run("color and shadow", func(t *testing.T) {
		cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000", Tiles: []Tile{{
			Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{
				{Text: "█", Font: "go.ttf", Size: n(40), Color: "#fff", X: n(10), Y: n(10), Shadow: &Shadow{X: 40, Color: "#f00"}},
			},
		}}}
		img, err := render(cfg, goFontDir(t))
		if err != nil {
			t.Fatal(err)
		}
		checkPixels(t, img, []pixel{{20, 30, white}, {70, 30, color.NRGBA{0xff, 0, 0, 0xff}}, {95, 95, black}})
	})

	t.Run("missing font", func(t *testing.T) {
		cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000", Tiles: []Tile{{
			Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{{Text: "Hi", Font: "missing.ttf"}},
		}}}
		_, err := render(cfg, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "tile 1: layer 1: font: open") || !strings.Contains(err.Error(), "missing.ttf") {
			t.Fatalf("err = %v, want it to name the missing font", err)
		}
	})

	t.Run("not a font", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "bad.ttf"), []byte("nope"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := &Config{Width: 100, Height: 100, Columns: 1, Rows: 1, Background: "#000", TileBackground: "#000", Tiles: []Tile{{
			Cols: cellRange{1, 1}, Rows: cellRange{1, 1}, Layers: []Layer{{Text: "Hi", Font: "bad.ttf"}},
		}}}
		if _, err := render(cfg, dir); err == nil || !strings.Contains(err.Error(), "bad.ttf") {
			t.Fatalf("err = %v, want it to name the bad font", err)
		}
	})
}
