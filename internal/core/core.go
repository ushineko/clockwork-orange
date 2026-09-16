/*
Package core holds every user-facing operation as a headless function (spec
010 R6, R7 via `docs/architecture.md`).

The rule the whole project turns on: an operation is a function taking a
request struct and returning a result struct, and the front ends only render.
The cobra CLI (cmd/clockwork-orange) and the Fyne GUI (cmd/clockwork-orange-gui)
therefore cannot drift apart in behaviour, only in presentation, and a new
operation is one place to add rather than two places to keep in step.
*/
package core

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/platform"
	"github.com/ushineko/clockwork-orange/internal/plugins"
	"github.com/ushineko/clockwork-orange/internal/store"
)

/*
Deps is everything an operation reaches for outside its own arguments: the
desktop, the service manager, the two SQLite stores, the HTTP client and the
plugin registry.

A nil field is filled with the production default the first time it is needed,
so a front end passes `&core.Deps{}` (or nil in the Request) and tests pass
fakes. Stores are opened lazily because most CLI invocations never touch them
and opening SQLite on every `--self-test` would be wrong.
*/
type Deps struct {
	Platform  platform.Platform
	Service   platform.Service
	History   *store.History
	Blacklist *store.Blacklist
	HTTP      *http.Client
	Registry  []plugins.Plugin
	Now       func() time.Time

	mu sync.Mutex
}

// Request is embedded in every operation's request struct.
type Request struct {
	// ConfigPath overrides the config file location; "" probes the defaults.
	ConfigPath string
	// Events receives log lines and progress. Every field may be nil.
	Events events.Events
	// Deps supplies the outside world; nil means production defaults.
	Deps *Deps
}

var defaultDeps Deps

func (r Request) deps() *Deps {
	if r.Deps == nil {
		return &defaultDeps
	}
	return r.Deps
}

// ensurePlatform fills Platform and Service with the host implementations.
func (d *Deps) ensurePlatform(ev events.Events) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Platform == nil {
		d.Platform = platform.NewWithEvents(platform.ExecRunner{}, ev)
	}
	if d.Service == nil {
		d.Service = platform.NewServiceWithEvents(platform.ExecRunner{}, ev)
	}
}

// ensureStores opens the default history and blacklist databases.
func (d *Deps) ensureStores() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.History == nil {
		h, err := store.OpenDefaultHistory()
		if err != nil {
			return fmt.Errorf("open history: %w", err)
		}
		d.History = h
	}
	if d.Blacklist == nil {
		b, err := store.OpenDefaultBlacklist()
		if err != nil {
			return fmt.Errorf("open blacklist: %w", err)
		}
		d.Blacklist = b
	}
	return nil
}

// ensureRegistry builds the plugin registry over the stores.
func (d *Deps) ensureRegistry() error {
	if err := d.ensureStores(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Registry == nil {
		d.Registry = plugins.Registry(plugins.Deps{
			History: d.History, Blacklist: d.Blacklist, HTTP: d.HTTP, Now: d.Now,
		})
	}
	return nil
}

// Close releases the stores. Front ends call it on exit.
func (d *Deps) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var first error
	if d.History != nil {
		if err := d.History.Close(); err != nil && first == nil {
			first = err
		}
		d.History = nil
	}
	if d.Blacklist != nil {
		if err := d.Blacklist.Close(); err != nil && first == nil {
			first = err
		}
		d.Blacklist = nil
	}
	d.Registry = nil
	return first
}

// Mode is which surface an operation targets (R6.3).
type Mode int

// The four dispatch modes of _perform_wallpaper_operation.
const (
	// ModeDefault is neither flag: desktop behaviour without the desktop
	// wording, and the "no source, no plugins" error.
	ModeDefault Mode = iota
	ModeDesktop
	ModeLockscreen
	ModeDual
)

func (m Mode) String() string {
	switch m {
	case ModeDesktop:
		return "desktop"
	case ModeLockscreen:
		return "lockscreen"
	case ModeDual:
		return "dual"
	default:
		return "default"
	}
}

/*
ResolveMode merges the CLI flags with the config document the way
merge_config_with_args did (R2.5).

dual_wallpapers forces both. Otherwise the config keys apply only when
neither flag was given, and `desktop` is consulted before `lockscreen`, so a
file with both set (and no dual_wallpapers) yields desktop-only, and an
explicit --lockscreen is never widened to dual by a `desktop: true` in the
file. Those two edges are Python behaviour, ported as-is.
*/
func ResolveMode(doc config.Document, desktop, lockscreen bool) Mode {
	if doc.DualWallpapers {
		return ModeDual
	}
	if !desktop && !lockscreen {
		desktop = doc.Desktop
	}
	if !desktop && !lockscreen {
		lockscreen = doc.Lockscreen
	}
	switch {
	case desktop && lockscreen:
		return ModeDual
	case lockscreen:
		return ModeLockscreen
	case desktop:
		return ModeDesktop
	default:
		return ModeDefault
	}
}

// LoadConfigResult is the loaded document and where it came from.
type LoadConfigResult struct {
	Doc config.Document
	// Path is the file that was read, or the default write path when none
	// existed.
	Path string
	// Exists reports whether a file was found.
	Exists bool
}

// LoadConfig reads the config (R2.1): the override path if set, else the
// candidate paths in order, else Defaults.
func LoadConfig(_ context.Context, req Request) (LoadConfigResult, error) {
	if req.ConfigPath != "" {
		doc, err := config.Load(req.ConfigPath)
		if err != nil {
			return LoadConfigResult{}, err
		}
		return LoadConfigResult{Doc: doc, Path: req.ConfigPath, Exists: fileExists(req.ConfigPath)}, nil
	}
	doc, path, err := config.LoadDefault()
	if err != nil {
		return LoadConfigResult{}, err
	}
	res := LoadConfigResult{Doc: doc, Path: path, Exists: path != ""}
	if res.Path == "" {
		res.Path = config.DefaultPath()
	}
	return res, nil
}

// SaveConfigRequest carries the document to write.
type SaveConfigRequest struct {
	Request
	Doc config.Document
}

// SaveConfig writes the document to the override path or the default path
// (R2.3). Writers always target the primary path, never the Windows public
// copy.
func SaveConfig(_ context.Context, req SaveConfigRequest) (string, error) {
	path := req.ConfigPath
	if path == "" {
		path = config.DefaultPath()
	}
	if err := config.Save(path, req.Doc); err != nil {
		return "", err
	}
	return path, nil
}

// WriteConfigRequest is `--write-config` (R2.7): the current flags become the
// persisted defaults.
type WriteConfigRequest struct {
	Request
	Desktop, Lockscreen, Dual bool
	Wait                      int
	URL, File, Directory      string
}

// WriteConfig writes desktop/lockscreen/dual_wallpapers/default_wait and the
// legacy default_* source keys from the flags, preserving everything else in
// an existing file.
func WriteConfig(ctx context.Context, req WriteConfigRequest) (string, error) {
	loaded, err := LoadConfig(ctx, req.Request)
	if err != nil {
		return "", err
	}
	doc := loaded.Doc
	doc.Desktop = req.Desktop
	doc.Lockscreen = req.Lockscreen
	doc.DualWallpapers = req.Dual
	if req.Wait > 0 {
		doc.DefaultWait = req.Wait
	} else if doc.DefaultWait == 0 {
		doc.DefaultWait = config.DefaultWaitGUI
	}
	if req.URL != "" {
		doc.DefaultURL = req.URL
	}
	if req.File != "" {
		doc.DefaultFile = req.File
	}
	if req.Directory != "" {
		doc.DefaultDirectory = req.Directory
	}
	path := req.ConfigPath
	if path == "" {
		path = config.DefaultPath()
	}
	if err := config.Save(path, doc); err != nil {
		return "", err
	}
	req.Events.Infof("Configuration written to %s", path)
	return path, nil
}
