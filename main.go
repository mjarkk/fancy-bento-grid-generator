package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	out := flag.String("o", "", "output PNG `path` (default: the config path with a .png extension)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-o output.png] config.toml\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(configPath, outPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	img, err := render(cfg, filepath.Dir(configPath))
	if err != nil {
		return err
	}
	if outPath == "" {
		outPath = strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".png"
	}
	if err := writePNG(outPath, img); err != nil {
		return err
	}
	fmt.Println("wrote", outPath)
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
