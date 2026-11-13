package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/malina/pkg/sd"
)

func main() {
	lib := flag.String("lib", "", "lib dir")
	model := flag.String("model", "", "model")
	out := flag.String("out", "out.png", "out")
	flag.Parse()
	t0 := time.Now()
	if err := sd.Load(*lib); err != nil {
		fmt.Println("LOAD FAIL:", err)
		os.Exit(1)
	}
	if err := sd.Init(*lib); err != nil {
		fmt.Println("INIT FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("load:", time.Since(t0))
	cp := sd.ContextParamsInit()
	cp.ModelPath = *model
	cp.NThreads = 16
	t1 := time.Now()
	ctx, err := sd.NewContext(cp)
	if err != nil {
		fmt.Println("NEW FAIL:", err)
		os.Exit(1)
	}
	defer sd.FreeContext(ctx)
	fmt.Println("model load:", time.Since(t1))
	p := sd.ImgGenParamsInit()
	p.Prompt = "an orange cat on a beach, photo"
	p.Width, p.Height, p.Seed = 512, 512, 42
	t2 := time.Now()
	img, err := sd.GenerateImage(ctx, p)
	if err != nil {
		fmt.Println("GEN FAIL:", err)
		os.Exit(1)
	}
	if err := img.SavePNG(*out); err != nil {
		fmt.Println("SAVE FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("generate:", time.Since(t2), "->", *out)
}
