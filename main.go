package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/deepteams/webp"
)

func main() {
	args := os.Args[1:]
	var err error
	if len(args) > 0 && args[0] == "sizes" {
		err = sizesCommand(args[1:])
	} else {
		err = renderCommand(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func renderCommand(args []string) error {
	name := filepath.Base(os.Args[0])
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	out := fs.String("o", "", "output `path`; its extension picks the format: .png, .jpg, .jpeg or .webp (default: the config path with a .png extension)")
	scale := scaleFlag(fs)
	var quality int
	fs.Func("quality", "`quality` of .jpg and .webp output, from 1 to 100 (default 90)", func(s string) error {
		q, err := strconv.Atoi(s)
		if err != nil || q < 1 || q > 100 {
			return errors.New("want a whole number from 1 to 100")
		}
		quality = q
		return nil
	})
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s [-o output.png] [-scale n] [-quality q] config.toml\n       %s sizes [-scale n] [-json] config.toml\n", name, name)
		fs.PrintDefaults()
	}
	positional := parseFlags(fs, args)
	if len(positional) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	return run(positional[0], *out, *scale, quality)
}

func sizesCommand(args []string) error {
	fs := flag.NewFlagSet("sizes", flag.ExitOnError)
	scale := scaleFlag(fs)
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s sizes [-scale n] [-json] config.toml\n\nPrints where every tile lands, in logical and in output pixels.\n\n", filepath.Base(os.Args[0]))
		fs.PrintDefaults()
	}
	positional := parseFlags(fs, args)
	if len(positional) != 1 {
		fs.Usage()
		os.Exit(2)
	}
	cfg, err := loadConfig(positional[0])
	if err != nil {
		return err
	}
	if *scale != nil {
		cfg.Scale = *scale
	}
	return printSizes(os.Stdout, cfg, *asJSON)
}

func scaleFlag(fs *flag.FlagSet) **float64 {
	var scale *float64
	fs.Func("scale", "multiply the canvas and every length by `n`, overriding the config's scale", func(s string) error {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || !(v > 0) || math.IsInf(v, 0) {
			return errors.New("want a positive number")
		}
		scale = &v
		return nil
	})
	return &scale
}

// parseFlags parses fs from args, allowing flags after the positional
// arguments, which it returns.
func parseFlags(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		fs.Parse(args) // ExitOnError handles errors.
		args = fs.Args()
		if len(args) == 0 {
			return positional
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func run(configPath, outPath string, scale *float64, quality int) error {
	if outPath == "" {
		outPath = strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".png"
	}
	encode, err := encoder(outPath, quality)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if scale != nil {
		cfg.Scale = scale
	}
	img, err := render(cfg, filepath.Dir(configPath))
	if err != nil {
		return err
	}
	if err := writeImage(outPath, img, encode); err != nil {
		return err
	}
	fmt.Println("wrote", outPath)
	return nil
}

const defaultQuality = 90

// encoder picks the format from path's extension. A quality of 0 means the
// default.
func encoder(path string, quality int) (func(io.Writer, image.Image) error, error) {
	ext := strings.ToLower(filepath.Ext(path))
	lossy := ext == ".jpg" || ext == ".jpeg" || ext == ".webp"
	if quality != 0 && !lossy {
		return nil, fmt.Errorf("-quality only applies to .jpg, .jpeg and .webp output, not %s", path)
	}
	if quality == 0 {
		quality = defaultQuality
	}
	switch ext {
	case ".png":
		return png.Encode, nil
	case ".jpg", ".jpeg":
		return func(w io.Writer, img image.Image) error {
			// JPEG has no alpha, so transparent margins become white, like the
			// page most viewers show images on.
			flat := image.NewRGBA(img.Bounds())
			draw.Draw(flat, flat.Rect, image.White, image.Point{}, draw.Src)
			draw.Draw(flat, flat.Rect, img, img.Bounds().Min, draw.Over)
			return jpeg.Encode(w, flat, &jpeg.Options{Quality: quality})
		}, nil
	case ".webp":
		return func(w io.Writer, img image.Image) error {
			return webp.Encode(w, img, &webp.EncoderOptions{Quality: float32(quality), Method: 4})
		}, nil
	}
	return nil, fmt.Errorf("can't write %s: the output extension must be .png, .jpg, .jpeg or .webp", path)
}

func writeImage(path string, img image.Image, encode func(io.Writer, image.Image) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

type rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type tileSize struct {
	Tile    int    `json:"tile"`
	Cols    string `json:"cols"`
	Rows    string `json:"rows"`
	Logical rect   `json:"logical"`
	Output  rect   `json:"output"`
}

// printSizes lists every tile's rectangle in output pixels, and in logical
// pixels as those divided by the scale, so an SVG with that viewBox maps its
// units onto whole output pixels.
func printSizes(w io.Writer, cfg *Config, asJSON bool) error {
	s := cfg.scale()
	out := cfg.scaled()
	logical := func(v float64) float64 { return math.Round(v/s*100) / 100 }
	tiles := []tileSize{}
	for i, t := range out.Tiles {
		x, y, tw, th := out.tileRect(t)
		tiles = append(tiles, tileSize{
			Tile: i + 1, Cols: t.Cols.String(), Rows: t.Rows.String(),
			Logical: rect{logical(x), logical(y), logical(tw), logical(th)},
			Output:  rect{x, y, tw, th},
		})
	}

	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Scale        float64    `json:"scale"`
			Width        int        `json:"width"`
			Height       int        `json:"height"`
			OutputWidth  int        `json:"output_width"`
			OutputHeight int        `json:"output_height"`
			Tiles        []tileSize `json:"tiles"`
		}{s, cfg.Width, cfg.Height, out.Width, out.Height, tiles})
	}

	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	fmt.Fprintf(w, "canvas %d×%d, scale %s: %d×%d output px\n\n", cfg.Width, cfg.Height, num(s), out.Width, out.Height)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "tile\tcols\trows\tx\ty\tw\th\tout x\tout y\tout w\tout h")
	for _, t := range tiles {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", t.Tile, t.Cols, t.Rows,
			num(t.Logical.X), num(t.Logical.Y), num(t.Logical.W), num(t.Logical.H),
			num(t.Output.X), num(t.Output.Y), num(t.Output.W), num(t.Output.H))
	}
	return tw.Flush()
}
