package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	sd "github.com/pendra-ai/stable-diffusion-go"
)

//go:embed lib/libstable-diffusion.so
var libBytes []byte

// extract writes the embedded lib to <cache>/<sha256>/ once and returns the dir.
func extract() (string, error) {
	sum := sha256.Sum256(libBytes)
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sd-spike", hex.EncodeToString(sum[:8]))
	dst := filepath.Join(dir, "libstable-diffusion.so")
	if st, err := os.Stat(dst); err == nil && st.Size() == int64(len(libBytes)) {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, libBytes, 0o700); err != nil {
		return "", err
	}
	return dir, os.Rename(tmp, dst)
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
	ctx, err := sd.NewStableDiffusion(&sd.ContextParams{ModelPath: *model, NThreads: 16})
	if err != nil {
		fmt.Println("NEW FAIL:", err)
		os.Exit(1)
	}
	defer ctx.Free()
	t1 := time.Now()
	if err := ctx.GenerateImage(&sd.ImgGenParams{Prompt: "an orange cat on a beach, photo", Width: 512, Height: 512,
		SampleSteps: 20, CfgScale: 7, Seed: 42}, *out); err != nil {
		fmt.Println("GEN FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("generate:", time.Since(t1), "->", *out)
}
