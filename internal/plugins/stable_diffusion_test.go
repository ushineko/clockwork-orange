package plugins

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
	"github.com/ushineko/clockwork-orange/internal/models"
	"github.com/ushineko/clockwork-orange/internal/sdgen"
)

// fakeSD records what the plugin asked for and returns a flat image.
type fakeSD struct {
	opts   sdgen.Options
	reqs   []sdgen.Request
	closed bool
	failAt map[int]bool
	openEr error
}

func (f *fakeSD) open(o sdgen.Options) (sdgen.Session, error) {
	if f.openEr != nil {
		return nil, f.openEr
	}
	f.opts = o
	return f, nil
}

func (f *fakeSD) Generate(r sdgen.Request) (image.Image, error) {
	i := len(f.reqs)
	f.reqs = append(f.reqs, r)
	if f.failAt[i] {
		return nil, errors.New("boom")
	}
	img := image.NewNRGBA(image.Rect(0, 0, r.Width, r.Height))
	for y := range r.Height {
		for x := range r.Width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x + i), G: uint8(y), B: uint8(i * 40), A: 255})
		}
	}
	return img, nil
}

func (f *fakeSD) Close() { f.closed = true }

// sdFixture builds the plugin with a pulled model in a temp store.
func sdFixture(t *testing.T, fake *fakeSD) (*stableDiffusion, string, map[string]any, *[]string) {
	t.Helper()
	h, b := testStores(t)
	store := models.Store{Dir: t.TempDir()}
	m, _ := models.Lookup("sd-1.5")
	path := store.Path(m)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	// A sparse file at the catalog size stands in for the checkpoint.
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(m.File.Size))
	require.NoError(t, f.Close())

	p := newStableDiffusion(Deps{
		History: h, Blacklist: b, Now: func() time.Time { return fixedNow }, Models: store,
		SDOpen: fake.open, SDLib: func() (string, error) { return "/libs", nil },
		Rand: func(int) int { return 0 },
	})
	dir := t.TempDir()
	cfg := map[string]any{"download_dir": dir, "upscale": false, "width": 64, "height": 40, "steps": 3}
	var logs []string
	return p, dir, cfg, &logs
}

func runSD(t *testing.T, p *stableDiffusion, cfg map[string]any, logs *[]string) (Result, error) {
	t.Helper()
	ev := events.Events{OnLog: func(_ events.Level, m string) { *logs = append(*logs, m) }}
	return p.Run(context.Background(), cfg, ev)
}

func TestSchemaKeepsTheV295KeysInOrder(t *testing.T) {
	var keys []string
	for _, f := range (&stableDiffusion{}).Schema() {
		keys = append(keys, f.Key)
	}
	require.Equal(t, []string{
		"prompt", "negative_prompt", "model_id", "steps", "width", "height", "upscale",
		"safety_checker", "guidance_scale", "download_dir", "interval", "limit", "max_files",
		"seed", "threads", "backend", "device",
	}, keys)
}

func TestRunGeneratesSavesRecordsAndReports(t *testing.T) {
	fake := &fakeSD{}
	p, dir, cfg, logs := sdFixture(t, fake)
	cfg["limit"] = 2
	cfg["prompt"] = []any{map[string]any{"term": "a castle", "enabled": true}, map[string]any{"term": "off", "enabled": false}}
	cfg["negative_prompt"] = "ugly"
	cfg["backend"] = "vulkan"
	cfg["device"] = 0
	cfg["threads"] = 4

	var saved []string
	ev := events.Events{OnImageSaved: func(s string) { saved = append(saved, s) }}
	res, err := p.Run(context.Background(), cfg, ev)
	require.NoError(t, err)
	require.Equal(t, dir, res.Path)
	require.Len(t, saved, 2)
	require.True(t, fake.closed)

	require.Equal(t, "/libs", fake.opts.LibDir)
	require.Equal(t, "vulkan", fake.opts.Backend)
	require.Equal(t, 0, fake.opts.Device)
	require.Equal(t, 4, fake.opts.Threads)
	require.Len(t, fake.reqs, 2)
	require.Equal(t, "a castle", fake.reqs[0].Prompt)
	require.Equal(t, "ugly", fake.reqs[0].Negative)
	require.Equal(t, 3, fake.reqs[0].Steps)

	for _, s := range saved {
		require.True(t, strings.HasPrefix(filepath.Base(s), "sd_20260915_120000_"), s)
		img, _, err := imaging.DecodeFile(s)
		require.NoError(t, err)
		require.Equal(t, 64, img.Bounds().Dx())
		ours, err := p.deps.History.Downloaded(s)
		require.NoError(t, err)
		require.True(t, ours, "history knows the app made %s", s)
	}
	_ = logs
}

func TestRunRoundsSizeDownToAMultipleOfEight(t *testing.T) {
	fake := &fakeSD{}
	p, _, cfg, logs := sdFixture(t, fake)
	cfg["width"], cfg["height"] = 515, 77
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	require.Equal(t, 512, fake.reqs[0].Width)
	require.Equal(t, 72, fake.reqs[0].Height)
}

func TestRunUpscalesToTheWallpaperSize(t *testing.T) {
	fake := &fakeSD{}
	p, dir, cfg, logs := sdFixture(t, fake)
	cfg["upscale"] = true
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	files, _ := filepath.Glob(filepath.Join(dir, "sd_*.png"))
	require.Len(t, files, 1)
	img, _, err := imaging.DecodeFile(files[0])
	require.NoError(t, err)
	require.Equal(t, sdUpscaleWidth, img.Bounds().Dx())
	require.Equal(t, sdUpscaleHeight, img.Bounds().Dy())
}

func TestRunHonoursTheIntervalUnlessForced(t *testing.T) {
	fake := &fakeSD{}
	p, dir, cfg, logs := sdFixture(t, fake)
	require.NoError(t, UpdateLastRun(dir, fixedNow))
	res, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	require.Equal(t, dir, res.Path)
	require.Empty(t, fake.reqs, "inside the interval nothing is generated")

	cfg["force"] = true
	_, err = runSD(t, p, cfg, logs)
	require.NoError(t, err)
	require.Len(t, fake.reqs, 1)
}

func TestRunResetClearsTheDirectoryFirst(t *testing.T) {
	fake := &fakeSD{}
	p, dir, cfg, logs := sdFixture(t, fake)
	stale := filepath.Join(dir, "old.png")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o600))
	cfg["reset"] = true
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	require.NoFileExists(t, stale)
}

func TestRunSeedIsOffsetPerImageSoRepeatsDiffer(t *testing.T) {
	fake := &fakeSD{}
	p, _, cfg, logs := sdFixture(t, fake)
	cfg["limit"], cfg["seed"] = 3, 100
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	require.Equal(t, []int64{100, 101, 102}, []int64{fake.reqs[0].Seed, fake.reqs[1].Seed, fake.reqs[2].Seed})
}

func TestRunSkipsAFailedImageAndKeepsTheRest(t *testing.T) {
	fake := &fakeSD{failAt: map[int]bool{0: true}}
	p, dir, cfg, logs := sdFixture(t, fake)
	cfg["limit"] = 2
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	files, _ := filepath.Glob(filepath.Join(dir, "sd_*.png"))
	require.Len(t, files, 1)
}

func TestRunWithNothingGeneratedReportsTheCause(t *testing.T) {
	fake := &fakeSD{failAt: map[int]bool{0: true}}
	p, _, cfg, logs := sdFixture(t, fake)
	_, err := runSD(t, p, cfg, logs)
	require.ErrorContains(t, err, "boom")
}

func TestRunRemovesABlacklistedImage(t *testing.T) {
	fake := &fakeSD{}
	p, dir, cfg, logs := sdFixture(t, fake)
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	files, _ := filepath.Glob(filepath.Join(dir, "sd_*.png"))
	require.Len(t, files, 1)
	hash, err := imaging.SHA256File(files[0])
	require.NoError(t, err)
	require.NoError(t, p.deps.Blacklist.Add(hash, "stable_diffusion", files[0]))

	// The same bytes again: remove the file, as the other download plugins do.
	cfg["force"] = true
	require.NoError(t, os.Remove(files[0]))
	fake.reqs = nil // the fake derives the pixels from the request count
	_, _ = runSD(t, p, cfg, logs)
	again, _ := filepath.Glob(filepath.Join(dir, "sd_*.png"))
	require.Empty(t, again)
}

func TestRunAppliesRetentionToItsOwnFiles(t *testing.T) {
	fake := &fakeSD{}
	p, dir, cfg, logs := sdFixture(t, fake)
	cfg["limit"], cfg["max_files"] = 3, 2
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	files, _ := filepath.Glob(filepath.Join(dir, "sd_*.png"))
	require.Len(t, files, 2)
}

func TestRunLogsOnceThatTheSafetyFilterIsNotEnforced(t *testing.T) {
	fake := &fakeSD{}
	p, _, cfg, logs := sdFixture(t, fake)
	cfg["safety_checker"] = true
	_, err := runSD(t, p, cfg, logs)
	require.NoError(t, err)
	n := 0
	for _, l := range *logs {
		if strings.Contains(l, "safety filter is not enforced") {
			n++
		}
	}
	require.Equal(t, 1, n)
}

func TestRunNamesTheFixWhenTheModelIsNotPulled(t *testing.T) {
	fake := &fakeSD{}
	p, _, cfg, logs := sdFixture(t, fake)
	p.deps.Models = models.Store{Dir: t.TempDir()}
	_, err := runSD(t, p, cfg, logs)
	require.ErrorContains(t, err, "models pull sd-1.5")
	require.Empty(t, fake.reqs)
}

func TestRunReportsAMissingLibrary(t *testing.T) {
	fake := &fakeSD{}
	p, _, cfg, logs := sdFixture(t, fake)
	p.deps.SDLib = func() (string, error) { return "", errors.New("no library") }
	_, err := runSD(t, p, cfg, logs)
	require.ErrorContains(t, err, "no library")
}

func TestResolveModelCoversEveryForm(t *testing.T) {
	store := models.Store{Dir: t.TempDir()}
	m, _ := models.Lookup("sd-1.5")
	require.NoError(t, os.MkdirAll(filepath.Dir(store.Path(m)), 0o750))
	f, err := os.Create(store.Path(m))
	require.NoError(t, err)
	require.NoError(t, f.Truncate(m.File.Size))
	require.NoError(t, f.Close())

	for _, id := range []string{"sd-1.5", "runwayml/stable-diffusion-v1-5"} {
		got, err := ResolveModel(store, id)
		require.NoError(t, err, id)
		require.Equal(t, store.Path(m), got)
	}

	own := filepath.Join(t.TempDir(), "mine.safetensors")
	require.NoError(t, os.WriteFile(own, []byte("x"), 0o600))
	got, err := ResolveModel(store, own)
	require.NoError(t, err)
	require.Equal(t, own, got)

	_, err = ResolveModel(store, "prompthero/openjourney")
	require.ErrorContains(t, err, "unknown model")
	require.ErrorContains(t, err, "models pull")

	_, err = ResolveModel(models.Store{Dir: t.TempDir()}, "sdxl-base-1.0")
	require.ErrorContains(t, err, "models pull sdxl-base-1.0")
}
