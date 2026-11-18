package main

import (
	"cmp"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// textBlock is a laid out text layer. Its box is as wide as its longest line
// and one line height tall per line, with each line's glyphs centered on it
// vertically as in CSS. Glyphs may spill outside the box.
type textBlock struct {
	face    font.Face
	color   color.NRGBA
	spacing fixed.Int26_6
	lineH   float64
	align   string
	lines   []string
	widths  []fixed.Int26_6
}

func (r *renderer) font(path string) (*sfnt.Font, error) {
	if f, ok := r.fonts[path]; ok {
		return f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r.fonts[path] = f
	return f, nil
}

func (r *renderer) text(l Layer) (*textBlock, error) {
	f, err := r.font(r.path(l.Font))
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	size := orDefault(l.Size, 16*r.cfg.scale())
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	c, err := parseColor(cmp.Or(l.Color, "#000"))
	if err != nil {
		return nil, err
	}
	t := &textBlock{
		face:    face,
		color:   c,
		spacing: fixed.Int26_6(math.Round(l.LetterSpacing * 64)),
		lineH:   size * orDefault(l.LineHeight, 1.2),
		align:   l.Align,
	}
	for _, line := range strings.Split(l.Text, "\n") {
		if l.MaxWidth == nil {
			t.lines = append(t.lines, line)
		} else {
			t.lines = append(t.lines, t.wrap(line, fixed.Int26_6(math.Round(*l.MaxWidth*64)))...)
		}
	}
	for _, line := range t.lines {
		t.widths = append(t.widths, t.walk(line, nil))
	}
	return t, nil
}

// walk calls fn with every rune of line and the pen's x position for it, and
// returns the line's width. Letter spacing goes between runes, not after the
// last one, so it doesn't push aligned text off center.
func (t *textBlock) walk(line string, fn func(c rune, x fixed.Int26_6)) fixed.Int26_6 {
	var x fixed.Int26_6
	prev := rune(-1)
	for _, c := range line {
		if prev >= 0 {
			x += t.face.Kern(prev, c) + t.spacing
		}
		if fn != nil {
			fn(c, x)
		}
		advance, _ := t.face.GlyphAdvance(c)
		x += advance
		prev = c
	}
	return x
}

// wrap breaks line between words so each part fits in maxWidth, except a
// single word that doesn't fit on its own.
func (t *textBlock) wrap(line string, maxWidth fixed.Int26_6) []string {
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := &lines[len(lines)-1]
		if joined := *last + " " + word; t.walk(joined, nil) <= maxWidth {
			*last = joined
		} else {
			lines = append(lines, word)
		}
	}
	return lines
}

func (t *textBlock) size() (w, h float64) {
	var widest fixed.Int26_6
	for _, lw := range t.widths {
		widest = max(widest, lw)
	}
	return float64(widest) / 64, float64(len(t.lines)) * t.lineH
}

func (t *textBlock) render(int, int) *image.RGBA {
	type glyph struct {
		c   rune
		dot fixed.Point26_6
	}
	w, _ := t.size()
	m := t.face.Metrics()
	ascent, descent := float64(m.Ascent)/64, float64(m.Descent)/64
	var glyphs []glyph
	for i, line := range t.lines {
		var left float64
		switch t.align {
		case "center":
			left = (w - float64(t.widths[i])/64) / 2
		case "right":
			left = w - float64(t.widths[i])/64
		}
		baseline := float64(i)*t.lineH + (t.lineH-ascent-descent)/2 + ascent
		t.walk(line, func(c rune, x fixed.Int26_6) {
			glyphs = append(glyphs, glyph{c, fixed.Point26_6{X: fixed.Int26_6(math.Round(left*64)) + x, Y: fixed.Int26_6(math.Round(baseline * 64))}})
		})
	}

	var bounds image.Rectangle
	for _, g := range glyphs {
		if dr, _, _, _, ok := t.face.Glyph(g.dot, g.c); ok {
			bounds = bounds.Union(dr)
		}
	}
	img := image.NewRGBA(bounds)
	src := image.NewUniform(t.color)
	for _, g := range glyphs {
		if dr, mask, mp, _, ok := t.face.Glyph(g.dot, g.c); ok {
			draw.DrawMask(img, dr, src, image.Point{}, mask, mp, draw.Over)
		}
	}
	return img
}

func orDefault(v *float64, def float64) float64 {
	if v == nil {
		return def
	}
	return *v
}
