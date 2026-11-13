package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/malina/pkg/sd"
)

//go:embed lib
var libFS embed.FS

func extract() (string, error) {
	h := sha256.New()
	_ = fs.WalkDir(libFS, "lib", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			i, _ := d.Info()
			fmt.Fprintf(h, "%s:%d;", p, i.Size())
		}
		return nil
	})
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sd-spike-malina", hex.EncodeToString(h.Sum(nil)[:8]))
	if _, err := os.Stat(filepath.Join(dir, ".complete")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	err = fs.WalkDir(libFS, "lib", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := libFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, filepath.Base(p)), b, 0o700)
	})
	if err != nil {
		return "", err
	}
	return dir, os.WriteFile(filepath.Join(dir, ".complete"), nil, 0o600)
}

func main() {
	model := flag.String("model", "", "model")
	out := flag.String("out", "out.png", "out")
	flag.Parse()
	t0 := time.Now()
	dir, err := extract()
	if err != nil {
		fmt.Println("EXTRACT FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("extracted to", dir, "in", time.Since(t0))
	if err := sd.Load(dir); err != nil {
		fmt.Println("LOAD FAIL:", err)
		os.Exit(1)
	}
	if err := sd.Init(dir); err != nil {
		fmt.Println("INIT FAIL:", err)
		os.Exit(1)
	}
	cp := sd.ContextParamsInit()
	cp.ModelPath = *model
	ctx, err := sd.NewContext(cp)
	if err != nil {
		fmt.Println("NEW FAIL:", err)
		os.Exit(1)
	}
	defer sd.FreeContext(ctx)
	p := sd.ImgGenParamsInit()
	p.Prompt = "an orange cat on a beach, photo"
	p.Width, p.Height, p.Seed = 512, 512, 42
	t1 := time.Now()
	img, err := sd.GenerateImage(ctx, p)
	if err != nil {
		fmt.Println("GEN FAIL:", err)
		os.Exit(1)
	}
	_ = img.SavePNG(*out)
	fmt.Println("generate:", time.Since(t1), "->", *out)
}
