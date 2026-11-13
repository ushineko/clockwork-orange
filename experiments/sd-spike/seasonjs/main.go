package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/seasonjs/stable-diffusion/v2/pkg/diffusion"
	"github.com/seasonjs/stable-diffusion/v2/pkg/types"
)

func main() {
	lib := flag.String("lib", "", "lib dir")
	model := flag.String("model", "", "model")
	out := flag.String("out", "out.png", "out")
	flag.Parse()
	t0 := time.Now()
	d, err := diffusion.New(*lib)
	if err != nil {
		fmt.Println("LOAD FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("load:", time.Since(t0), "version:", d.Version())
	ctx, err := d.NewContext(&types.ContextParams{ModelPath: *model, NThreads: 16})
	if err != nil {
		fmt.Println("NEW FAIL:", err)
		os.Exit(1)
	}
	defer ctx.Close()
	f, _ := os.Create(*out)
	defer f.Close()
	p := &types.ImgGenParams{Prompt: "an orange cat on a beach, photo", Width: 512, Height: 512, Seed: 42}
	p.SampleParams.SampleSteps = 20
	p.SampleParams.Guidance.TxtCfg = 7
	t1 := time.Now()
	if err := ctx.GenerateImage(p, types.PNG, []io.Writer{f}); err != nil {
		fmt.Println("GEN FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("generate:", time.Since(t1))
}
