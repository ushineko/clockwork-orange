package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	sd "github.com/pendra-ai/stable-diffusion-go"
)

func main() {
	lib := flag.String("lib", "", "dir holding libstable-diffusion.so")
	model := flag.String("model", "", "model path")
	out := flag.String("out", "out.png", "output png")
	steps := flag.Int("steps", 20, "steps")
	seed := flag.Int64("seed", 42, "seed")
	prompt := flag.String("prompt", "an orange cat on a beach, photo", "prompt")
	flag.Parse()

	t0 := time.Now()
	if err := sd.Load(*lib); err != nil {
		fmt.Println("LOAD FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("load:", time.Since(t0))
	t1 := time.Now()
	ctx, err := sd.NewStableDiffusion(&sd.ContextParams{ModelPath: *model, NThreads: 16})
	if err != nil {
		fmt.Println("NEW FAIL:", err)
		os.Exit(1)
	}
	defer ctx.Free()
	fmt.Println("model load:", time.Since(t1))
	t2 := time.Now()
	err = ctx.GenerateImage(&sd.ImgGenParams{Prompt: *prompt, Width: 512, Height: 512,
		SampleSteps: int32(*steps), CfgScale: 7, Seed: *seed}, *out)
	if err != nil {
		fmt.Println("GEN FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("generate:", time.Since(t2), "->", *out)
}
