package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	sd "github.com/l8bloom/gosd/pkg/gosd"
)

func main() {
	lib := flag.String("lib", "", "dir with libstable-diffusion.so")
	model := flag.String("model", "", "model path")
	out := flag.String("out", "out.png", "output")
	steps := flag.Int("steps", 20, "steps")
	flag.Parse()
	os.Setenv("GOSD_DYN_LIB", *lib)
	t0 := time.Now()
	if err := sd.Load(); err != nil {
		fmt.Println("LOAD FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("load:", time.Since(t0))
	p := sd.ContextParamsInit()
	p.ModelPath = *model
	t1 := time.Now()
	ctx := sd.NewContext(p)
	defer sd.FreeCtx(ctx)
	fmt.Println("model load:", time.Since(t1))
	ip := sd.ImageGenParamsInit()
	ip.Prompt = "an orange cat on a beach, photo"
	ip.Width, ip.Height = 512, 512
	ip.Seed = 42
	ip.SampleParams.SampleSteps = int32(*steps)
	ip.SampleParams.Guidance.TextCfg = 7
	t2 := time.Now()
	img := sd.GenerateImage(ctx, ip)
	img.SavePNG(*out)
	fmt.Println("generate:", time.Since(t2), "->", *out)
}
