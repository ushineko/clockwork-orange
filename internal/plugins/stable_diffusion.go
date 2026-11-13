package plugins

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
	"github.com/ushineko/clockwork-orange/internal/models"
	"github.com/ushineko/clockwork-orange/internal/sdgen"
	"github.com/ushineko/clockwork-orange/internal/sdlib"
)

const (
	sdLog           = "[StableDiffusion]"
	sdDefaultPrompt = "breathtaking landscape, mountains, lake, sunset, 4k, photorealistic, serene"
	sdUpscaleWidth  = 2560 // the v2.9.5 plugin's "QHD target"
	sdUpscaleHeight = 1440
)

// stableDiffusion generates wallpapers locally with stable-diffusion.cpp
// (spec 031). Its config keys and run flow follow plugins/stable_diffusion.py
// from v2.9.5; the generation itself moved from PyTorch to the native library.
type stableDiffusion struct {
	deps Deps
}

func newStableDiffusion(d Deps) *stableDiffusion { return &stableDiffusion{deps: d} }

func (*stableDiffusion) Name() string { return "stable_diffusion" }

func (*stableDiffusion) Description() string {
	return "Generate wallpapers locally with Stable Diffusion"
}

func sdDefaultDir() string {
	return filepath.Join(config.HomeDir(), "Pictures", "Wallpapers", "StableDiffusion")
}

// Schema is the v2.9.5 get_config_schema in declaration order, followed by
// the keys spec 031 adds.
func (*stableDiffusion) Schema() []Field {
	return []Field{
		{
			Key: "prompt", Type: TypeStringList,
			Default:     []Term{{Term: sdDefaultPrompt, Enabled: true}},
			Description: "Text Prompts (Randomly Selected)",
			Suggestions: []string{
				sdDefaultPrompt,
				"abstract 3d geometric shapes, vibrant colors, 4k, raytracing",
				"cyberpunk city street, rain, neon lights, 4k, detailed",
				"deep space nebula, stars, galaxy, 8k, hubble style",
			},
		},
		{
			Key: "negative_prompt", Type: TypeString,
			Default:     "blurry, low quality, distortion, ugly, text, watermark",
			Description: "Negative Prompt",
		},
		{
			Key: "model_id", Type: TypeString, Default: "sd-1.5",
			Description: "Model (a catalog name, or a path to a checkpoint)",
			Suggestions: modelNames(),
		},
		{Key: "steps", Type: TypeInteger, Default: 30, Description: "Inference Steps"},
		{
			Key: "width", Type: TypeInteger, Default: 768, Description: "Generation Width (Base)",
			Suggestions: []string{"512", "768", "960"},
		},
		{
			Key: "height", Type: TypeInteger, Default: 512, Description: "Generation Height (Base)",
			Suggestions: []string{"512", "768"},
		},
		{Key: "upscale", Type: TypeBoolean, Default: true, Description: "Upscale to Wallpaper (1080p+)"},
		{
			Key: "safety_checker", Type: TypeBoolean, Default: true,
			Description: "Enable NSFW Safety Filter (not available in this build)",
		},
		{Key: "guidance_scale", Type: TypeInteger, Default: 7, Description: "Guidance Scale"},
		{
			Key: "download_dir", Type: TypeString, Default: sdDefaultDir(),
			Description: "Output Directory", Widget: WidgetDirectoryPath,
		},
		{
			Key: "interval", Type: TypeString, Default: "Daily", Description: "Generation Interval",
			Enum: []string{"Hourly", "Daily", "Weekly"},
		},
		{Key: "limit", Type: TypeInteger, Default: 1, Description: "Images per Run"},
		{Key: "max_files", Type: TypeInteger, Default: 50, Description: "Retention Limit"},
		{Key: "seed", Type: TypeInteger, Default: 0, Description: "Seed (0 = random)"},
		{Key: "threads", Type: TypeInteger, Default: 0, Description: "CPU Threads (0 = half the logical CPUs)"},
		{
			Key: "backend", Type: TypeString, Default: "auto", Description: "Backend",
			Enum: []string{"auto", "cpu", "vulkan"},
		},
		{Key: "device", Type: TypeInteger, Default: -1, Description: "GPU Device Index (-1 = library default)"},
	}
}

func modelNames() []string {
	var names []string
	for _, m := range models.Catalog() {
		names = append(names, m.Name)
	}
	return names
}

var errSDNothing = errors.New("no images generated")

// sdOpener, sdLibDir and sdStore return the injected dependency or the real one.
func (p *stableDiffusion) opener() sdgen.Opener {
	if p.deps.SDOpen != nil {
		return p.deps.SDOpen
	}
	return sdgen.Open
}

func (p *stableDiffusion) libDir() (string, error) {
	if p.deps.SDLib != nil {
		return p.deps.SDLib()
	}
	return sdlib.Resolve()
}

func (p *stableDiffusion) store() models.Store {
	if p.deps.Models.Dir != "" {
		return p.deps.Models
	}
	return models.NewStore()
}

func (p *stableDiffusion) pick(n int) int {
	if p.deps.Rand != nil {
		return p.deps.Rand(n)
	}
	return rand.IntN(n) //nolint:gosec // a prompt choice, not a secret
}

// ResolveModel turns a model_id into a file on disk (spec 031 R3): a catalog
// name or a v2.9.5 Hugging Face id selects a pulled model, an existing path is
// used as it is, anything else is an error that says what to do.
func ResolveModel(store models.Store, id string) (string, error) {
	id = strings.TrimSpace(id)
	if m, ok := models.Lookup(id); ok {
		if !store.Pulled(m) {
			return "", fmt.Errorf("model %q is not pulled: run `clockwork-orange models pull %s`", m.Name, m.Name)
		}
		return store.Path(m), nil
	}
	if p := config.ExpandPath(id); id != "" {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p, nil
		}
	}
	return "", fmt.Errorf("unknown model %q: use a catalog name (%s) or a path to a checkpoint, "+
		"and fetch catalog models with `clockwork-orange models pull`", id, strings.Join(modelNames(), ", "))
}

// Run mirrors the other download plugins: ensure the directory, handle
// process_blacklist, honour the interval unless forced, handle reset, generate,
// stamp .last_run, apply retention, then succeed when anything was generated
// or the directory already holds generated images.
func (p *stableDiffusion) Run(ctx context.Context, cfg map[string]any, ev events.Events) (Result, error) {
	downloadDir := config.ExpandPath(getString(cfg, "download_dir", sdDefaultDir()))
	interval := strings.ToLower(getString(cfg, "interval", "Daily"))
	maxFiles := getInt(cfg, "max_files", 50)

	if err := os.MkdirAll(downloadDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("create download dir: %w", err)
	}
	if getString(cfg, "action", "") == "process_blacklist" {
		return processBlacklist(cfg, p.deps, p.Name(), sdLog, ev)
	}
	if p.deps.History == nil {
		return Result{}, errMissingStore(p.Name(), "history")
	}
	if p.deps.Blacklist == nil {
		return Result{}, errMissingStore(p.Name(), "blacklist")
	}
	if !getBool(cfg, "force", false) && !ShouldRun(downloadDir, interval, p.deps.Now()) {
		ev.Infof("%s Skipping run (interval: %s)", sdLog, interval)
		return Result{Path: downloadDir}, nil
	}
	if getBool(cfg, "reset", false) {
		ev.Progress(0, "Resetting directory...")
		resetDir(downloadDir, sdLog, ev)
	}
	if getBool(cfg, "safety_checker", true) {
		ev.Infof("%s The NSFW safety filter is not enforced: this build has no classifier.", sdLog)
	}

	modelPath, err := ResolveModel(p.store(), getString(cfg, "model_id", "sd-1.5"))
	if err != nil {
		return Result{}, err
	}
	dir, err := p.libDir()
	if err != nil {
		return Result{}, err
	}

	total, genErr := p.generate(ctx, cfg, dir, modelPath, downloadDir, ev)

	if err := UpdateLastRun(downloadDir, p.deps.Now()); err != nil {
		return Result{}, err
	}
	ev.Progress(95, "Cleaning up old files...")
	cleanupOldFiles(p.deps.History, downloadDir, "sd_*.png", maxFiles, sdLog, ev)
	ev.Progress(100, "Done!")

	// .last_run is always there by now, so "the directory is non-empty" would
	// call a run that made nothing a success; look for generated images.
	if existing, _ := filepath.Glob(filepath.Join(downloadDir, "sd_*.png")); total > 0 || len(existing) > 0 {
		return Result{Path: downloadDir}, nil
	}
	if genErr != nil {
		return Result{}, genErr
	}
	return Result{}, errSDNothing
}

// generate opens the model once and makes up to `limit` images. A failed image
// is logged and skipped, as the Python plugin did; the last failure is returned
// so a run that produced nothing can say why.
func (p *stableDiffusion) generate(ctx context.Context, cfg map[string]any, libDir, modelPath, downloadDir string,
	ev events.Events,
) (int, error) {
	limit := max(1, getInt(cfg, "limit", 1))
	steps := max(1, getInt(cfg, "steps", 30))
	width := max(8, getInt(cfg, "width", 768)/8*8) // the VAE needs a multiple of 8
	height := max(8, getInt(cfg, "height", 512)/8*8)
	seed := int64(getInt(cfg, "seed", 0))
	prompts := ParseQueries(cfg["prompt"], sdDefaultPrompt)
	negative := getString(cfg, "negative_prompt", "")

	backend, device := getString(cfg, "backend", "auto"), getInt(cfg, "device", -1)
	ev.Infof("%s Backend %s, device %d (-1 = library default), library at %s", sdLog, backend, device, libDir)

	var imageIdx int
	var mu sync.Mutex
	ev.Progress(10, "Loading model (this may take a while)...")
	sess, err := p.opener()(sdgen.Options{
		LibDir:    libDir,
		ModelPath: modelPath,
		Backend:   backend,
		Device:    device,
		Threads:   getInt(cfg, "threads", 0),
		OnLog: func(level sdgen.LogLevel, line string) {
			ev.Logf(sdEventLevel(level), "%s %s", sdLog, line)
		},
		OnStep: func(step, total int) {
			mu.Lock()
			i := imageIdx
			mu.Unlock()
			if total > 0 {
				ev.Progress(10+(i*total+step)*85/(limit*total),
					fmt.Sprintf("Generating image %d/%d (step %d/%d)", i+1, limit, step, total))
			}
		},
	})
	if err != nil {
		return 0, err
	}
	defer sess.Close()

	made := 0
	var lastErr error
	for i := range limit {
		if err := ctx.Err(); err != nil {
			return made, fmt.Errorf("stopped after %d image(s): %w", made, err)
		}
		mu.Lock()
		imageIdx = i
		mu.Unlock()
		prompt := prompts[p.pick(len(prompts))]
		ev.Infof("%s Generating image %d/%d: %s", sdLog, i+1, limit, prompt)
		s := seed
		if s > 0 {
			s += int64(i) // a fixed seed must not repeat the same image
		}
		img, err := sess.Generate(sdgen.Request{
			Prompt: prompt, Negative: negative, Width: width, Height: height,
			Steps: steps, CFGScale: float64(getInt(cfg, "guidance_scale", 7)), Seed: s,
		})
		if err != nil {
			ev.Errorf("%s Generation failed: %v", sdLog, err)
			lastErr = err
			continue
		}
		if getBool(cfg, "upscale", true) {
			ev.Infof("%s Upscaling to %dx%d...", sdLog, sdUpscaleWidth, sdUpscaleHeight)
			img = imaging.CoverResizeCrop(img, sdUpscaleWidth, sdUpscaleHeight)
		}
		saved, err := p.save(img, downloadDir, i, ev)
		if err != nil {
			ev.Errorf("%s Saving failed: %v", sdLog, err)
			lastErr = err
			continue
		}
		if saved {
			made++
		}
	}
	return made, lastErr
}

// save writes img as sd_<timestamp>_<i>.png, drops it if blacklisted, and
// records it in history so retention knows the app made it.
func (p *stableDiffusion) save(img image.Image, dir string, i int, ev events.Events) (bool, error) {
	name := fmt.Sprintf("sd_%s_%d.png", p.deps.Now().Format("20060102_150405"), i)
	path := filepath.Join(dir, name)
	f, err := os.Create(path) //nolint:gosec // dir is the configured download dir
	if err != nil {
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return false, fmt.Errorf("encode %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", path, err)
	}
	hash, err := imaging.SHA256File(path)
	if err != nil {
		return false, err
	}
	if p.deps.Blacklist.IsBlacklisted(hash) {
		ev.Infof("%s Image is blacklisted. Removing %s", sdLog, name)
		return false, removeFile(path)
	}
	if _, err := p.deps.History.AddEntry("generated://stable_diffusion/"+name, path, p.Name()); err != nil {
		return false, err
	}
	ev.Infof("%s Saved %s", sdLog, name)
	ev.ImageSaved(path)
	return true, nil
}

// sdEventLevel maps the library's log levels onto the program's: warnings and
// errors keep their rank.
func sdEventLevel(l sdgen.LogLevel) events.Level {
	switch l {
	case sdgen.LogError:
		return events.LevelError
	case sdgen.LogWarn:
		return events.LevelWarn
	default:
		// The library logs every routine step at its info level; here those
		// are debug lines, so a default run shows the plugin's own few.
		return events.LevelDebug
	}
}
