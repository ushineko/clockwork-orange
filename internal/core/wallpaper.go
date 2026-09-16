package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/engine"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/platform"
	"github.com/ushineko/clockwork-orange/internal/plugins"
)

// Errors the CLI maps to exit codes and messages (R6.3).
var (
	ErrDualNeedsDirectory   = errors.New("dual wallpaper mode with single file not supported (need different images); use --desktop --lockscreen -d /path/to/directory instead")
	ErrLockscreenNeedsLocal = errors.New("lock screen mode requires either --file or --directory")
	ErrNoEnabledPlugins     = errors.New("no enabled plugins found")
	ErrNoSource             = errors.New("no source specified and no plugins enabled; use --plugin, --file, --directory, or --url")
	ErrDaemonRunning        = errors.New("another clockwork-orange daemon is already running")
)

// daemonLockID is the single-instance id the cycling loop holds (DV10) so a
// systemd daemon and a GUI timer do not both rotate wallpapers.
const daemonLockID = "clockwork_orange_service_lock"

// SetRequest sets from an explicit local source.
type SetRequest struct {
	Request
	Mode Mode
	// Path is a file or a directory.
	Path string
}

// SetFromFile sets one file (R6.3): desktop/default → set_local_wallpaper,
// lockscreen → set_lockscreen_wallpaper, dual → error.
func SetFromFile(ctx context.Context, req SetRequest) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	switch req.Mode {
	case ModeDual:
		return ErrDualNeedsDirectory
	case ModeLockscreen:
		req.Events.Debugf("Lock screen file mode: %s", req.Path)
		return d.Platform.SetLockscreen(ctx, abs(req.Path))
	default:
		req.Events.Debugf("%s file mode: %s", modeWord(req.Mode), req.Path)
		return engine.SetLocalFile(ctx, d.Platform, req.Path, req.Events)
	}
}

// SetFromDirectory does one random set from a directory (R6.3).
func SetFromDirectory(ctx context.Context, req SetRequest) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	req.Events.Debugf("%s directory mode: %s", modeWord(req.Mode), req.Path)
	return setFromSources(ctx, d.Platform, req.Mode, []string{req.Path}, req.Events)
}

// SetURLRequest downloads and sets one image (desktop/default only; the CLI
// rejects --url with --lockscreen before calling).
type SetURLRequest struct {
	Request
	URL string
}

// SetFromURL is download_and_set_wallpaper (R3.5, DV4).
func SetFromURL(ctx context.Context, req SetURLRequest) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	req.Events.Debugf("Downloading image from: %s", req.URL)
	path, cleanup, err := engine.DownloadToTemp(ctx, d.HTTP, req.URL)
	if err != nil {
		return err
	}
	defer cleanup()
	req.Events.Debugf("Created temporary file: %s", path)
	return d.Platform.SetWallpaper(ctx, path)
}

// CollectRequest runs every enabled plugin and gathers their source paths.
type CollectRequest struct {
	Request
	Doc config.Document
}

// CollectSources is collect_plugin_sources (R3.6): each enabled plugin runs;
// a success contributes its path; failures are logged and skipped.
func CollectSources(ctx context.Context, req CollectRequest) ([]string, error) {
	d := req.deps()
	if err := d.ensureRegistry(); err != nil {
		return nil, err
	}
	var sources []string
	for _, name := range req.Doc.EnabledPlugins() {
		p, ok := plugins.Lookup(d.Registry, name)
		if !ok {
			// Unknown to this build (e.g. stable_diffusion, D6): keep the
			// config, skip the run.
			req.Events.Debugf("Plugin %s is enabled but not available in this build; skipping", name)
			continue
		}
		res, err := p.Run(ctx, req.Doc.Plugins[name], req.Events)
		if err != nil {
			req.Events.Errorf("Failed to execute plugin %s: %v", name, err)
			continue
		}
		if res.Path != "" {
			sources = append(sources, res.Path)
		}
	}
	return sources, nil
}

// CycleRequest is one wallpaper change.
type CycleRequest struct {
	Request
	Mode Mode
	// Sources, when set, are used as-is (directory cycling). When empty the
	// cycle is dynamic: the config is reloaded from disk and enabled plugins
	// are run (_execute_dynamic_cycle).
	Sources []string
}

// CycleResult reports what the cycle used.
type CycleResult struct {
	Doc     config.Document
	Sources []string
}

// Cycle performs one change (R3.6). In dynamic mode errors are returned for
// the caller to log; the loop keeps going regardless, as the Python did.
func Cycle(ctx context.Context, req CycleRequest) (CycleResult, error) {
	d := req.deps()
	d.ensurePlatform(req.Events)
	res := CycleResult{Sources: req.Sources}
	if len(req.Sources) == 0 {
		loaded, err := LoadConfig(ctx, req.Request)
		if err != nil {
			return res, err
		}
		res.Doc = loaded.Doc
		sources, err := CollectSources(ctx, CollectRequest{Request: req.Request, Doc: loaded.Doc})
		if err != nil {
			return res, err
		}
		res.Sources = sources
		if len(sources) == 0 {
			return res, fmt.Errorf("%w for %s mode", ErrNoEnabledPlugins, modeWord(req.Mode))
		}
	}
	return res, setFromSources(ctx, d.Platform, req.Mode, res.Sources, req.Events)
}

// setFromSources dispatches on mode over a source list.
func setFromSources(ctx context.Context, p platform.Platform, mode Mode, sources []string, ev events.Events) error {
	switch mode {
	case ModeDual:
		return engine.SetDualFromSources(ctx, p, sources, ev)
	case ModeLockscreen:
		img, err := engine.RandomImageFromSources(sources, ev)
		if err != nil {
			return err
		}
		return p.SetLockscreen(ctx, img)
	default:
		return engine.SetRandomFromSources(ctx, p, sources, ev)
	}
}

// LoopRequest is continuous cycling (R3.6, R6.8).
type LoopRequest struct {
	Request
	Mode Mode
	// Wait is the interval; a dynamic loop re-reads default_wait from the
	// config every cycle and falls back to this when the key is absent.
	Wait time.Duration
	// Sources fixes the sources (directory cycling). Empty means dynamic.
	Sources []string
	// Debounce overrides the config-change debounce; zero means the default.
	Debounce time.Duration
	// OnCycle, when set, is called after every cycle with its outcome.
	OnCycle func(res CycleResult, err error)
}

// RunLoop cycles until ctx is cancelled (cycle_* functions and
// cycle_dynamic_plugins). A dynamic loop watches the config file and a
// settled change interrupts the wait (R2.6). It holds the daemon lock (DV10)
// and returns ErrDaemonRunning if another loop holds it.
func RunLoop(ctx context.Context, req LoopRequest) error {
	lock, ok := platform.TryLock(daemonLockID)
	if !ok {
		return ErrDaemonRunning
	}
	defer lock.Release()

	debounce := req.Debounce
	if debounce == 0 {
		debounce = time.Duration(config.DebounceSeconds * float64(time.Second))
	}
	dynamic := len(req.Sources) == 0
	var changed <-chan struct{}
	if dynamic {
		path := req.ConfigPath
		if path == "" {
			path = config.DefaultPath()
		}
		w, err := config.Watch(ctx, path)
		if err != nil {
			req.Events.Warnf("Config watcher unavailable, continuing without it: %v", err)
		} else {
			defer func() { _ = w.Close() }()
			changed = w.Changed()
		}
		req.Events.Debugf("Starting dynamic multi-plugin mode (%s) with %s interval", modeWord(req.Mode), req.Wait)
	} else {
		req.Events.Debugf("Starting %s continuous mode with %s interval", modeWord(req.Mode), req.Wait)
	}

	for {
		res, err := Cycle(ctx, CycleRequest{Request: req.Request, Mode: req.Mode, Sources: req.Sources})
		if err != nil && ctx.Err() == nil {
			req.Events.Errorf("%v", err)
		}
		if req.OnCycle != nil {
			req.OnCycle(res, err)
		}
		if ctx.Err() != nil {
			return nil
		}
		wait := req.Wait
		if dynamic && res.Doc.DefaultWait > 0 {
			wait = time.Duration(res.Doc.DefaultWait) * time.Second
		}
		if wait <= 0 {
			wait = time.Duration(config.DefaultWaitGUI) * time.Second
		}
		req.Events.Debugf("Waiting %s before next change...", wait)
		if config.WaitForNextCycle(ctx, wait, changed, debounce) {
			req.Events.Debugf("Config change detected, interrupting wait cycle")
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// DaemonRunning reports whether a cycling loop holds the daemon lock (DV10);
// the GUI consults it before starting its own timer.
func DaemonRunning() bool {
	lock, ok := platform.TryLock(daemonLockID)
	if ok {
		lock.Release()
	}
	return !ok
}

func modeWord(m Mode) string {
	switch m {
	case ModeDesktop:
		return "Desktop"
	case ModeLockscreen:
		return "Lock screen"
	case ModeDual:
		return "Dual wallpaper"
	default:
		return "Default"
	}
}
