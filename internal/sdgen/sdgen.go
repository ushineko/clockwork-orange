/*
Package sdgen generates images with stable-diffusion.cpp through malina
(spec 031 R2, R8). It is the only package that imports malina's sd bindings,
so the plugin and its tests see a small interface and a build that never
generates never loads a native library.

The native library is process-global: it is loaded once, the first Open
decides which directory and which Vulkan device, and later Opens reuse it.
*/
package sdgen

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"strconv"
	"sync"

	"github.com/ardanlabs/malina/pkg/sd"
)

// Options configure one Session.
type Options struct {
	// LibDir holds the stable-diffusion.cpp libraries (sdlib.Resolve).
	LibDir string
	// ModelPath is the checkpoint to load.
	ModelPath string
	// Backend is "", "auto", "cpu" or "vulkan"; "" and "auto" use the
	// library default.
	Backend string
	// Device is the Vulkan device index, or -1 for the library default. It
	// only takes effect on the first Open in the process.
	Device int
	// Threads is the CPU thread count; 0 means half the logical CPUs.
	Threads int
	// OnLog receives the library's log lines. Nil discards them.
	OnLog func(level LogLevel, line string)
	// OnStep receives sampling progress.
	OnStep func(step, steps int)
}

// LogLevel ranks a line the library logged.
type LogLevel int

// Library log levels, least to most important.
const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

func mapLevel(l sd.LogLevel) LogLevel {
	switch l {
	case sd.LogError:
		return LogError
	case sd.LogWarn:
		return LogWarn
	case sd.LogInfo:
		return LogInfo
	default:
		return LogDebug
	}
}

// Request is one image.
type Request struct {
	Prompt   string
	Negative string
	Width    int
	Height   int
	Steps    int
	CFGScale float64
	// Seed of 0 or less means a random seed.
	Seed int64
}

// Session is a loaded model that can generate several images.
type Session interface {
	Generate(req Request) (image.Image, error)
	Close()
}

// Opener loads a model. Production uses Open; tests pass a fake.
type Opener func(Options) (Session, error)

var (
	loadMu     sync.Mutex
	loadedDir  string
	logMu      sync.Mutex
	logSink    func(LogLevel, string)
	stepMu     sync.Mutex
	stepSink   func(step, steps int)
	errBadSize = errors.New("image size must be positive")
)

// DefaultThreads is half the logical CPUs, at least one. Spec 030 measured the
// library's own default at half the speed of this choice on a hybrid CPU.
func DefaultThreads() int {
	return max(1, runtime.NumCPU()/2)
}

// Open loads the library (once) and the model.
func Open(opts Options) (Session, error) {
	if err := loadLibrary(opts); err != nil {
		return nil, err
	}
	setSinks(opts)

	cp := sd.ContextParamsInit()
	cp.ModelPath = opts.ModelPath
	cp.NThreads = int32(opts.Threads) //nolint:gosec // bounded by CPU count
	if opts.Threads <= 0 {
		cp.NThreads = int32(DefaultThreads()) //nolint:gosec // bounded by CPU count
	}
	switch opts.Backend {
	case "", "auto":
	case "cpu", "vulkan":
		cp.Backend = opts.Backend
	default:
		return nil, fmt.Errorf("unknown backend %q (want auto, cpu or vulkan)", opts.Backend)
	}
	ctx, err := sd.NewContext(cp)
	if err != nil {
		return nil, fmt.Errorf("load model %s: %w", opts.ModelPath, err)
	}
	return &session{ctx: ctx}, nil
}

func loadLibrary(opts Options) error {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loadedDir != "" {
		return nil
	}
	if opts.Device >= 0 {
		// The Vulkan backend reads this when it first enumerates devices.
		if err := os.Setenv("GGML_VK_VISIBLE_DEVICES", strconv.Itoa(opts.Device)); err != nil {
			return fmt.Errorf("select device: %w", err)
		}
	}
	if err := sd.Load(opts.LibDir); err != nil {
		return fmt.Errorf("load stable-diffusion library from %s: %w", opts.LibDir, err)
	}
	if err := sd.Init(opts.LibDir); err != nil {
		return fmt.Errorf("initialise stable-diffusion library: %w", err)
	}
	sd.SetLogCallback(func(level sd.LogLevel, text string) {
		logMu.Lock()
		f := logSink
		logMu.Unlock()
		if f != nil {
			f(mapLevel(level), text)
		}
	})
	sd.SetProgressCallback(func(step, steps int, _ float32) {
		stepMu.Lock()
		f := stepSink
		stepMu.Unlock()
		if f != nil {
			f(step, steps)
		}
	})
	loadedDir = opts.LibDir
	return nil
}

func setSinks(opts Options) {
	logMu.Lock()
	logSink = opts.OnLog
	logMu.Unlock()
	stepMu.Lock()
	stepSink = opts.OnStep
	stepMu.Unlock()
}

// Probe loads the native library from dir without loading a model and returns
// the version it reports. `--self-test` uses it to prove an embedded library
// extracts, loads and is the version the binding was written against.
func Probe(dir string) (string, error) {
	if err := loadLibrary(Options{LibDir: dir, Device: -1}); err != nil {
		return "", err
	}
	return LibraryVersion(), nil
}

// LibraryVersion reports the loaded library's version string, or "" before
// the first Open.
func LibraryVersion() string {
	loadMu.Lock()
	defer loadMu.Unlock()
	if loadedDir == "" {
		return ""
	}
	return sd.Version()
}

type session struct {
	ctx sd.Context
}

func (s *session) Generate(req Request) (image.Image, error) {
	if req.Width <= 0 || req.Height <= 0 {
		return nil, errBadSize
	}
	p := sd.ImgGenParamsInit()
	p.Prompt = req.Prompt
	p.NegativePrompt = req.Negative
	p.Width, p.Height = int32(req.Width), int32(req.Height) //nolint:gosec // validated by the plugin
	p.Steps = int32(req.Steps)                              //nolint:gosec // validated by the plugin
	p.CFGScale = float32(req.CFGScale)
	p.Seed = req.Seed
	if req.Seed <= 0 {
		p.Seed = -1
	}
	img, err := sd.GenerateImage(s.ctx, p)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}
	return toImage(img)
}

func (s *session) Close() { sd.FreeContext(s.ctx) }

// toImage converts the library's packed pixels (3 or 4 channels, 8 bit).
func toImage(img *sd.SDImage) (image.Image, error) {
	if img == nil {
		return nil, errors.New("the library returned no image")
	}
	w, h, ch := int(img.Width), int(img.Height), int(img.Channel)
	if ch != 3 && ch != 4 {
		return nil, fmt.Errorf("unsupported channel count %d", ch)
	}
	if len(img.Data) < w*h*ch {
		return nil, fmt.Errorf("short image data: %d bytes for %dx%dx%d", len(img.Data), w, h, ch)
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range w * h {
		a := uint8(255)
		if ch == 4 {
			a = img.Data[i*ch+3]
		}
		out.SetNRGBA(i%w, i/w, color.NRGBA{R: img.Data[i*ch], G: img.Data[i*ch+1], B: img.Data[i*ch+2], A: a})
	}
	return out, nil
}
