package main

import (
	"cmp"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"path/filepath"

	"github.com/fogleman/gg"
	"golang.org/x/image/font/sfnt"
)

type renderer struct {
	// In output pixels.
	cfg     *Config
	baseDir string
	dc      *gg.Context
	// Everything drawn on the sheet is clipped to it.
	sheet *image.Alpha
	fonts map[string]*sfnt.Font
}

// render resolves relative image and font paths against baseDir.
func render(cfg *Config, baseDir string) (image.Image, error) {
	cfg = cfg.scaled()
	r := &renderer{cfg: cfg, baseDir: baseDir, dc: gg.NewContext(cfg.Width, cfg.Height), fonts: map[string]*sfnt.Font{}}
	bg, err := parseColor(cfg.Background)
	if err != nil {
		return nil, fmt.Errorf("background: %w", err)
	}
	m := cfg.Margin
	sheetW, sheetH := float64(cfg.Width)-2*m, float64(cfg.Height)-2*m
	if s := cfg.SheetShadow.or(nil); s != nil {
		if err := r.shadow(*s, m, m, sheetW, sheetH, cfg.SheetRadius); err != nil {
			return nil, fmt.Errorf("sheet_shadow: %w", err)
		}
	}
	r.sheet = roundedRectMask(cfg.Width, cfg.Height, m, m, sheetW, sheetH, cfg.SheetRadius)
	r.dc.SetColor(bg)
	r.dc.DrawRoundedRectangle(m, m, sheetW, sheetH, cfg.SheetRadius)
	r.dc.Fill()
	r.dc.SetMask(r.sheet)

	// All shadows go first so none falls on a neighbouring tile.
	for i, t := range cfg.Tiles {
		if s := t.Shadow.or(cfg.TileShadow); s != nil {
			x, y, w, h := cfg.tileRect(t)
			if err := r.shadow(*s, x, y, w, h, r.tileRadius(t)); err != nil {
				return nil, fmt.Errorf("tile %d: shadow: %w", i+1, err)
			}
		}
	}
	for i, t := range cfg.Tiles {
		if err := r.tile(t); err != nil {
			return nil, fmt.Errorf("tile %d: %w", i+1, err)
		}
	}
	return r.dc.Image(), nil
}

func (r *renderer) tileRadius(t Tile) float64 {
	return *cmp.Or(t.Radius, &r.cfg.Radius)
}

func (r *renderer) tile(t Tile) error {
	x, y, w, h := r.cfg.tileRect(t)
	radius := r.tileRadius(t)
	bg, err := parseColor(cmp.Or(t.Background, r.cfg.TileBackground))
	if err != nil {
		return err
	}
	r.clip(x, y, w, h, radius)
	r.dc.SetColor(bg)
	r.dc.DrawRectangle(x, y, w, h)
	r.dc.Fill()

	for j, l := range t.Layers {
		if l.Colors != nil {
			err = r.gradient(l, x, y, w, h)
		} else {
			err = r.place(l, x, y, w, h, radius)
		}
		if err != nil {
			return fmt.Errorf("layer %d: %w", j+1, err)
		}
	}
	if b := t.Border.or(r.cfg.TileBorder); b != nil {
		if err := r.border(*b, x, y, w, h, radius); err != nil {
			return fmt.Errorf("border: %w", err)
		}
	}
	return nil
}

func (r *renderer) clip(x, y, w, h, radius float64) {
	r.dc.SetMask(r.sheet)
	r.dc.DrawRoundedRectangle(x, y, w, h, radius)
	r.dc.Clip()
}

func (r *renderer) border(b Border, x, y, w, h, radius float64) error {
	c, err := parseColor(b.Color)
	if err != nil {
		return err
	}
	r.dc.SetColor(c)
	r.dc.DrawRoundedRectangle(x, y, w, h, radius)
	if iw, ih := w-2*b.Width, h-2*b.Width; iw > 0 && ih > 0 {
		r.dc.DrawRoundedRectangle(x+b.Width, y+b.Width, iw, ih, max(radius-b.Width, 0))
	}
	r.dc.SetFillRuleEvenOdd()
	r.dc.Fill()
	r.dc.SetFillRuleWinding()
	return nil
}

func (r *renderer) shadow(s Shadow, x, y, w, h, radius float64) error {
	c, err := s.color()
	if err != nil {
		return err
	}
	radii := gaussianBoxRadii(s.Blur / 2)
	reach := float64(sum(radii))
	ox, oy := math.Floor(x+s.X-reach), math.Floor(y+s.Y-reach)
	rw, rh := int(math.Ceil(x+s.X+w+reach)-ox), int(math.Ceil(y+s.Y+h+reach)-oy)

	cast := roundedRectMask(rw, rh, x+s.X-ox, y+s.Y-oy, w, h, radius)
	boxBlur(cast, radii)
	caster := roundedRectMask(rw, rh, x-ox, y-oy, w, h, radius)
	r.dc.DrawImage(tint(c, cast, caster), int(ox), int(oy))
	return nil
}

func (s Shadow) color() (color.NRGBA, error) {
	return parseColor(cmp.Or(s.Color, "#00000066"))
}

// castShadow returns the shadow that img casts dx, dy pixels away, over img's
// bounds, which start at 0, 0.
func castShadow(img *image.RGBA, s Shadow, dx, dy int, radii []int) (*image.NRGBA, error) {
	c, err := s.color()
	if err != nil {
		return nil, err
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	caster, cast := image.NewAlpha(img.Rect), image.NewAlpha(img.Rect)
	for y := range h {
		for x := range w {
			a := img.Pix[y*img.Stride+4*x+3]
			caster.Pix[y*w+x] = a
			if tx, ty := x+dx, y+dy; tx >= 0 && tx < w && ty >= 0 && ty < h {
				cast.Pix[ty*w+tx] = a
			}
		}
	}
	boxBlur(cast, radii)
	return tint(c, cast, caster), nil
}

// tint colors the alpha of cast with c, leaving out what caster covers. Both
// masks must share their bounds and stride.
func tint(c color.NRGBA, cast, caster *image.Alpha) *image.NRGBA {
	img := image.NewNRGBA(cast.Rect)
	for i, a := range cast.Pix {
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2] = c.R, c.G, c.B
		img.Pix[4*i+3] = uint8((uint32(c.A)*uint32(a)*uint32(255-caster.Pix[i]) + 255*255/2) / (255 * 255))
	}
	return img
}

func roundedRectMask(canvasW, canvasH int, x, y, w, h, radius float64) *image.Alpha {
	dc := gg.NewContext(canvasW, canvasH)
	dc.DrawRoundedRectangle(x, y, w, h, radius)
	dc.Fill()
	return dc.AsMask()
}

func roundCorners(img *image.RGBA, radius float64) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	mask := roundedRectMask(w, h, 0, 0, float64(w), float64(h), radius)
	for i, a := range mask.Pix {
		for k := 4 * i; k < 4*i+4; k++ {
			img.Pix[k] = uint8((uint32(img.Pix[k])*uint32(a) + 127) / 255)
		}
	}
}

// gaussianBoxRadii returns the radii of three box blurs that together
// approximate a Gaussian blur, after Kovesi, "Fast Almost-Gaussian Filtering".
func gaussianBoxRadii(sigma float64) []int {
	if sigma <= 0 {
		return nil
	}
	const n = 3
	lower := int(math.Sqrt(12*sigma*sigma/n + 1))
	if lower%2 == 0 {
		lower--
	}
	fl := float64(lower)
	useLower := round((12*sigma*sigma - n*fl*fl - 4*n*fl - 3*n) / (-4*fl - 4))
	radii := make([]int, n)
	for i := range radii {
		width := lower + 2
		if i < useLower {
			width = lower
		}
		radii[i] = (width - 1) / 2
	}
	return radii
}

// boxBlur treats pixels outside a as transparent.
func boxBlur(a *image.Alpha, radii []int) {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	buf := make([]float64, w*h)
	for y := range h {
		for x := range w {
			buf[y*w+x] = float64(a.Pix[y*a.Stride+x])
		}
	}
	tmp := make([]float64, max(w, h))
	for _, br := range radii {
		for y := range h {
			boxBlurLine(buf[y*w:], 1, w, br, tmp)
		}
		for x := range w {
			boxBlurLine(buf[x:], w, h, br, tmp)
		}
	}
	for y := range h {
		for x := range w {
			a.Pix[y*a.Stride+x] = uint8(math.Round(buf[y*w+x]))
		}
	}
}

// boxBlurLine blurs the n values in line that are step apart.
func boxBlurLine(line []float64, step, n, radius int, tmp []float64) {
	var sum float64
	for i := range min(radius+1, n) {
		sum += line[i*step]
	}
	for i := range n {
		tmp[i] = sum / float64(2*radius+1)
		if j := i + radius + 1; j < n {
			sum += line[j*step]
		}
		if j := i - radius; j >= 0 {
			sum -= line[j*step]
		}
	}
	for i := range n {
		line[i*step] = tmp[i]
	}
}

func (r *renderer) gradient(l Layer, x, y, w, h float64) error {
	var grad gg.Gradient
	if l.Gradient == "radial" {
		cx, cy := x+w/2, y+h/2
		switch {
		case l.placedByXY():
			cx, cy = x+orDefault(l.X, 0), y+orDefault(l.Y, 0)
		case l.Position != nil:
			cx, cy = x+w*(0.5+l.Position.x), y+h*(0.5+l.Position.y)
		}
		var radius float64
		if l.Radius != nil {
			radius = *l.Radius
		} else {
			for _, corner := range [][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}} {
				radius = max(radius, math.Hypot(corner[0]-cx, corner[1]-cy))
			}
		}
		grad = gg.NewRadialGradient(cx, cy, 0, cx, cy, radius)
	} else {
		angle := 180.0
		if l.Angle != nil {
			angle = *l.Angle
		}
		// Like CSS, the gradient line is just long enough for the first and last
		// colors to land exactly on opposite corners.
		sin, cos := math.Sincos(angle * math.Pi / 180)
		half := (math.Abs(w*sin) + math.Abs(h*cos)) / 2
		cx, cy := x+w/2, y+h/2
		dx, dy := sin*half, -cos*half
		grad = gg.NewLinearGradient(cx-dx, cy-dy, cx+dx, cy+dy)
	}
	for i, s := range l.Colors {
		c, err := parseColor(s)
		if err != nil {
			return err
		}
		grad.AddColorStop(float64(i)/float64(len(l.Colors)-1), c)
	}
	r.dc.SetFillStyle(grad)
	r.dc.DrawRectangle(x, y, w, h)
	r.dc.Fill()
	return nil
}

func (r *renderer) path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(r.baseDir, p)
}

// drawable is what an image or text layer places.
type drawable interface {
	// size is the natural size in output pixels.
	size() (w, h float64)
	// render draws at w×h pixels, from 0, 0. Text may spill outside that box.
	render(w, h int) *image.RGBA
}

// place draws an image or text layer.
func (r *renderer) place(l Layer, tileX, tileY, tileW, tileH, tileRadius float64) error {
	var d drawable
	var err error
	if l.Text != "" {
		d, err = r.text(l)
	} else {
		d, err = loadAsset(r.path(l.Image), r.cfg.scale())
	}
	if err != nil {
		return err
	}

	x, y, w, h := tileX+l.Padding, tileY+l.Padding, tileW-2*l.Padding, tileH-2*l.Padding
	if w < 1 || h < 1 {
		s := r.cfg.scale()
		return fmt.Errorf("padding %v leaves no room in a %.0fx%.0f tile", l.Padding/s, tileW/s, tileH/s)
	}
	if l.Padding > 0 {
		// Concentric corners keep this clip inside the tile's, so it can replace it.
		r.clip(x, y, w, h, max(tileRadius-l.Padding, 0))
		defer r.clip(tileX, tileY, tileW, tileH, tileRadius)
	}

	var angle float64
	if l.Angle != nil {
		angle = *l.Angle * math.Pi / 180
	}
	sin, cos := math.Sincos(angle)
	absSin, absCos := math.Abs(sin), math.Abs(cos)
	// The box's bounding box along the image's rotated axes: whatever covers
	// it covers the box.
	frameW, frameH := w*absCos+h*absSin, w*absSin+h*absCos

	sized := l.Width != nil || l.Height != nil
	fit := cmp.Or(l.Fit, "cover")
	if l.Fit == "" && (l.Repeat != "" || sized || l.placedByXY() || l.Text != "") {
		fit = "none"
	}
	iw, ih := d.size()
	switch {
	case l.Width != nil && l.Height != nil:
		iw, ih = *l.Width, *l.Height
	case l.Width != nil:
		iw, ih = *l.Width, *l.Width*ih/iw
	case l.Height != nil:
		iw, ih = *l.Height*iw/ih, *l.Height
	}
	zoom := 1.0
	switch fit {
	case "cover":
		zoom = max(frameW/iw, frameH/ih)
	case "contain":
		zoom = min(w/(iw*absCos+ih*absSin), h/(iw*absSin+ih*absCos))
	}
	dw, dh := max(1, round(iw*zoom)), max(1, round(ih*zoom))

	var pos position
	if l.Position != nil {
		pos = *l.Position
	}
	cx, cy := x+w/2, y+h/2
	// u, v is the image center's offset from the box center along the rotated axes.
	var u, v float64
	if fit == "cover" && !l.placedByXY() {
		u, v = (frameW-float64(dw))*pos.x, (frameH-float64(dh))*pos.y
	} else {
		// Aligning the rotated image's bounding box keeps a contained image whole.
		sx := (w - (float64(dw)*absCos + float64(dh)*absSin)) * pos.x
		sy := (h - (float64(dw)*absSin + float64(dh)*absCos)) * pos.y
		if l.placedByXY() {
			// x and y place the unrotated image, which then turns about its center.
			sx, sy = tileX+orDefault(l.X, 0)+float64(dw)/2-cx, tileY+orDefault(l.Y, 0)+float64(dh)/2-cy
		}
		u, v = sx*cos+sy*sin, sy*cos-sx*sin
	}

	shadow := l.Shadow.or(nil)
	var radii []int
	var reach int
	if shadow != nil {
		radii = gaussianBoxRadii(shadow.Blur / 2)
		// Whatever casts a shadow onto the box must be on the canvas.
		reach = int(math.Ceil(math.Hypot(shadow.X, shadow.Y))) + sum(radii)
	}

	// Rotating all copies together on one canvas, not one by one, keeps seams
	// out of repeats.
	ox, oy := int(math.Floor(cx-frameW/2))-1-reach, int(math.Floor(cy-frameH/2))-1-reach
	canvas := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(cx+frameW/2))+1+reach-ox, int(math.Ceil(cy+frameH/2))+1+reach-oy))
	img := d.render(dw, dh)
	if l.Radius != nil && *l.Radius > 0 {
		roundCorners(img, *l.Radius)
	}
	ax, ay := round(cx+u-float64(dw)/2)-ox, round(cy+v-float64(dh)/2)-oy
	b := img.Bounds()
	for _, py := range repeatStarts(ay, dh, canvas.Rect.Dy(), l.Repeat == "y" || l.Repeat == "both") {
		for _, px := range repeatStarts(ax, dw, canvas.Rect.Dx(), l.Repeat == "x" || l.Repeat == "both") {
			draw.Draw(canvas, b.Add(image.Pt(px, py)), img, b.Min, draw.Over)
		}
	}
	if l.Opacity != nil {
		for i, c := range canvas.Pix {
			canvas.Pix[i] = uint8(math.Round(float64(c) * *l.Opacity))
		}
	}

	r.dc.Push()
	defer r.dc.Pop()
	r.dc.RotateAbout(angle, cx, cy)
	if shadow != nil {
		// The shadow falls along the tile's axes, not the rotated canvas's.
		dx, dy := shadow.X*cos+shadow.Y*sin, shadow.Y*cos-shadow.X*sin
		cast, err := castShadow(canvas, *shadow, round(dx), round(dy), radii)
		if err != nil {
			return fmt.Errorf("shadow: %w", err)
		}
		r.dc.DrawImage(cast, ox, oy)
	}
	r.dc.DrawImage(canvas, ox, oy)
	return nil
}

// repeatStarts lists where copies of length size begin along one axis: at
// alone, or when repeating, every size pixels through at until [0, total) is
// filled.
func repeatStarts(at, size, total int, repeat bool) []int {
	if !repeat {
		return []int{at}
	}
	first := at % size
	if first > 0 {
		first -= size
	}
	var starts []int
	for s := first; s < total; s += size {
		starts = append(starts, s)
	}
	return starts
}

func round(v float64) int {
	return int(math.Round(v))
}

func sum(values []int) int {
	var total int
	for _, v := range values {
		total += v
	}
	return total
}
