package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// SVGs are rasterized at the final size so they stay sharp.
type asset struct {
	raster image.Image
	svg    *oksvg.SvgIcon
	// Output pixels per pixel of the raster or unit of the SVG's viewBox.
	scale float64
}

func loadAsset(path string, scale float64) (*asset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if strings.EqualFold(filepath.Ext(path), ".svg") {
		icon, err := oksvg.ReadIconStream(f, oksvg.WarnErrorMode)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
			return nil, fmt.Errorf("%s: svg needs a viewBox or width and height", path)
		}
		return &asset{svg: icon, scale: scale}, nil
	}

	img, err := decodeRaster(bufio.NewReader(f))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if img.Bounds().Empty() {
		return nil, fmt.Errorf("%s: image is empty", path)
	}
	return &asset{raster: img, scale: scale}, nil
}

// decodeRaster decodes WebP with x/image: the WebP encoder registers its own
// decoder, which image.Decode would pick and which renders lossy colors
// differently.
func decodeRaster(r *bufio.Reader) (image.Image, error) {
	if head, err := r.Peek(12); err == nil && string(head[:4]) == "RIFF" && string(head[8:]) == "WEBP" {
		return webp.Decode(r)
	}
	img, _, err := image.Decode(r)
	return img, err
}

func (a *asset) size() (w, h float64) {
	if a.svg != nil {
		return a.svg.ViewBox.W * a.scale, a.svg.ViewBox.H * a.scale
	}
	b := a.raster.Bounds()
	return float64(b.Dx()) * a.scale, float64(b.Dy()) * a.scale
}

func (a *asset) render(w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if a.svg != nil {
		a.svg.SetTarget(0, 0, float64(w), float64(h))
		scanner := rasterx.NewScannerGV(w, h, dst, dst.Bounds())
		a.svg.Draw(rasterx.NewDasher(w, h, scanner), 1)
		return dst
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), a.raster, a.raster.Bounds(), draw.Src, nil)
	return dst
}

func parseColor(s string) (color.NRGBA, error) {
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) == 6 {
		hex += "ff"
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if len(hex) != 8 || err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid color %q, want #rgb, #rrggbb or #rrggbbaa", s)
	}
	return color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
}
