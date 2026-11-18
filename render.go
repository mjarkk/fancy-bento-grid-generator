package main

import (
	"cmp"
	"fmt"
	"image"
	"image/draw"
	"math"
	"path/filepath"

	"github.com/fogleman/gg"
)

type renderer struct {
	cfg     *Config
	baseDir string
	dc      *gg.Context
	// Everything drawn on the sheet is clipped to it.
	sheet *image.Alpha
}

// render resolves relative image paths against baseDir.
func render(cfg *Config, baseDir string) (image.Image, error) {
	r := &renderer{cfg: cfg, baseDir: baseDir, dc: gg.NewContext(cfg.Width, cfg.Height)}
	bg, err := parseColor(cfg.Background)
	if err != nil {
		return nil, fmt.Errorf("background: %w", err)
	}
	m := cfg.Margin
	sheetW, sheetH := float64(cfg.Width)-2*m, float64(cfg.Height)-2*m
	if cfg.SheetShadow != nil {
		if err := r.shadow(*cfg.SheetShadow, m, m, sheetW, sheetH, cfg.SheetRadius); err != nil {
			return nil, fmt.Errorf("sheet_shadow: %w", err)
		}
	}
	r.sheet = roundedRectMask(cfg.Width, cfg.Height, m, m, sheetW, sheetH, cfg.SheetRadius)
	r.dc.SetColor(bg)
	r.dc.DrawRoundedRectangle(m, m, sheetW, sheetH, cfg.SheetRadius)
	r.dc.Fill()
	r.dc.SetMask(r.sheet)

	// All shadows go first so none falls on a neighbouring tile.
	if cfg.TileShadow != nil {
		for _, t := range cfg.Tiles {
			x, y, w, h := r.bounds(t)
			if err := r.shadow(*cfg.TileShadow, x, y, w, h, cfg.Radius); err != nil {
				return nil, fmt.Errorf("tile_shadow: %w", err)
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

func (r *renderer) tile(t Tile) error {
	x, y, w, h := r.bounds(t)
	bg, err := parseColor(cmp.Or(t.Background, r.cfg.TileBackground))
	if err != nil {
		return err
	}
	r.clip(x, y, w, h, r.cfg.Radius)
	r.dc.SetColor(bg)
	r.dc.DrawRectangle(x, y, w, h)
	r.dc.Fill()

	for j, l := range t.Layers {
		if l.Colors != nil {
			err = r.gradient(l, x, y, w, h)
		} else {
			err = r.image(l, x, y, w, h)
		}
		if err != nil {
			return fmt.Errorf("layer %d: %w", j+1, err)
		}
	}
	return nil
}

func (r *renderer) clip(x, y, w, h, radius float64) {
	r.dc.SetMask(r.sheet)
	r.dc.DrawRoundedRectangle(x, y, w, h, radius)
	r.dc.Clip()
}

func (r *renderer) bounds(t Tile) (x, y, w, h float64) {
	cellW, cellH := r.cfg.cellSize()
	x, w = r.place(t.Cols, cellW)
	y, h = r.place(t.Rows, cellH)
	return x, y, w, h
}

func (r *renderer) place(cells cellRange, cellSize float64) (start, length float64) {
	n := float64(cells.last - cells.first + 1)
	return r.cfg.Margin + r.cfg.Padding + float64(cells.first-1)*(cellSize+r.cfg.Gap), n*cellSize + (n-1)*r.cfg.Gap
}

func (r *renderer) shadow(s Shadow, x, y, w, h, radius float64) error {
	c, err := parseColor(cmp.Or(s.Color, "#00000066"))
	if err != nil {
		return err
	}
	radii := gaussianBoxRadii(s.Blur / 2)
	var reach float64
	for _, br := range radii {
		reach += float64(br)
	}
	ox, oy := math.Floor(x+s.X-reach), math.Floor(y+s.Y-reach)
	rw, rh := int(math.Ceil(x+s.X+w+reach)-ox), int(math.Ceil(y+s.Y+h+reach)-oy)

	cast := roundedRectMask(rw, rh, x+s.X-ox, y+s.Y-oy, w, h, radius)
	boxBlur(cast, radii)
	caster := roundedRectMask(rw, rh, x-ox, y-oy, w, h, radius)
	img := image.NewNRGBA(cast.Rect)
	for i, a := range cast.Pix {
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2] = c.R, c.G, c.B
		img.Pix[4*i+3] = uint8((uint32(c.A)*uint32(a)*uint32(255-caster.Pix[i]) + 255*255/2) / (255 * 255))
	}
	r.dc.DrawImage(img, int(ox), int(oy))
	return nil
}

func roundedRectMask(canvasW, canvasH int, x, y, w, h, radius float64) *image.Alpha {
	dc := gg.NewContext(canvasW, canvasH)
	dc.DrawRoundedRectangle(x, y, w, h, radius)
	dc.Fill()
	return dc.AsMask()
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
	grad := gg.NewLinearGradient(cx-dx, cy-dy, cx+dx, cy+dy)
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

func (r *renderer) image(l Layer, tileX, tileY, tileW, tileH float64) error {
	path := l.Image
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.baseDir, path)
	}
	a, err := loadAsset(path)
	if err != nil {
		return err
	}

	x, y, w, h := tileX+l.Padding, tileY+l.Padding, tileW-2*l.Padding, tileH-2*l.Padding
	if w < 1 || h < 1 {
		return fmt.Errorf("padding %v leaves no room in a %.0fx%.0f tile", l.Padding, tileW, tileH)
	}
	if l.Padding > 0 {
		// Concentric corners keep this clip inside the tile's, so it can replace it.
		r.clip(x, y, w, h, max(r.cfg.Radius-l.Padding, 0))
		defer r.clip(tileX, tileY, tileW, tileH, r.cfg.Radius)
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

	fit := cmp.Or(l.Fit, "cover")
	if l.Fit == "" && l.Repeat != "" {
		fit = "none"
	}
	iw, ih := a.size()
	scale := 1.0
	switch fit {
	case "cover":
		scale = max(frameW/iw, frameH/ih)
	case "contain":
		scale = min(w/(iw*absCos+ih*absSin), h/(iw*absSin+ih*absCos))
	}
	dw, dh := max(1, round(iw*scale)), max(1, round(ih*scale))

	var pos position
	if l.Position != nil {
		pos = *l.Position
	}
	// u, v is the image center's offset from the box center along the rotated axes.
	var u, v float64
	if fit == "cover" {
		u, v = (frameW-float64(dw))*pos.x, (frameH-float64(dh))*pos.y
	} else {
		// Aligning the rotated image's bounding box keeps a contained image whole.
		sx := (w - (float64(dw)*absCos + float64(dh)*absSin)) * pos.x
		sy := (h - (float64(dw)*absSin + float64(dh)*absCos)) * pos.y
		u, v = sx*cos+sy*sin, sy*cos-sx*sin
	}

	// Rotating all copies together on one canvas, not one by one, keeps seams
	// out of repeats.
	cx, cy := x+w/2, y+h/2
	ox, oy := int(math.Floor(cx-frameW/2))-1, int(math.Floor(cy-frameH/2))-1
	canvas := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(cx+frameW/2))+1-ox, int(math.Ceil(cy+frameH/2))+1-oy))
	img := a.render(dw, dh)
	ax, ay := round(cx+u-float64(dw)/2)-ox, round(cy+v-float64(dh)/2)-oy
	for _, py := range repeatStarts(ay, dh, canvas.Rect.Dy(), l.Repeat == "y" || l.Repeat == "both") {
		for _, px := range repeatStarts(ax, dw, canvas.Rect.Dx(), l.Repeat == "x" || l.Repeat == "both") {
			draw.Draw(canvas, image.Rect(px, py, px+dw, py+dh), img, image.Point{}, draw.Src)
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
