//go:build darwin

package platform

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// DarwinPlatform sets the desktop through AppKit when built with cgo (the
// GUI binary) and through osascript otherwise (the CGO-free CLI), D11, R4.9.
// platform_darwin_cgo.go and platform_darwin_nocgo.go supply
// nativeScreenCount and setDesktopImagesNative.
type DarwinPlatform struct {
	Runner Runner
	Events events.Events
}

// errNoAppKit is what the non-cgo build's native setter returns; it selects
// the osascript fallback, as ImportError did in the Python.
var errNoAppKit = errors.New("AppKit unavailable (built without cgo)")

// New returns the macOS implementation for this host.
func New(r Runner) Platform { return &DarwinPlatform{Runner: r} }

// NewWithEvents is New with a log sink.
func NewWithEvents(r Runner, ev events.Events) Platform {
	return &DarwinPlatform{Runner: r, Events: ev}
}

// Name implements Platform.
func (*DarwinPlatform) Name() string { return "darwin" }

// LockscreenSupported implements Platform.
func (*DarwinPlatform) LockscreenSupported() bool { return false }

// MonitorCount implements Platform (R4.9): NSScreen.screens when AppKit is
// linked, else System Events' desktop count; any failure is 1.
func (p *DarwinPlatform) MonitorCount(ctx context.Context) int {
	if n, ok := p.nativeScreenCount(); ok {
		p.Events.Debugf("Detected %d monitors (macOS)", n)
		return n
	}
	stdout, _, err := p.Runner.Run(ctx, "osascript", "-e", `tell application "System Events" to count of desktops`)
	if err != nil {
		p.Events.Debugf("Failed to detect monitor count on macOS: %v", err)
		return 1
	}
	n, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		p.Events.Debugf("Failed to detect monitor count on macOS: %v", err)
		return 1
	}
	return n
}

// SetWallpaper implements Platform (R4.9).
func (p *DarwinPlatform) SetWallpaper(ctx context.Context, path string) error {
	abs := resolvePath(path)
	p.Events.Debugf("Setting macOS wallpaper: %s", abs)
	if !fileExists(abs) {
		p.Events.Errorf("File does not exist: %s", abs)
		return errNotExist(abs)
	}
	if err := p.setImages(ctx, []string{abs}); err != nil {
		return err
	}
	p.pruneCache(ctx)
	return nil
}

// SetWallpaperMulti implements Platform (R4.9): screen i gets paths[i % len].
// A missing file is logged and its slot left empty so the screens that would
// have shown it are skipped, keeping the same i % len mapping as the Python.
func (p *DarwinPlatform) SetWallpaperMulti(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return errors.New("no image paths provided")
	}
	resolved := make([]string, len(paths))
	valid := 0
	for i, raw := range paths {
		abs := resolvePath(raw)
		if !fileExists(abs) {
			p.Events.Errorf("File does not exist: %s", abs)
			continue
		}
		resolved[i] = abs
		valid++
	}
	if valid == 0 {
		return errors.New("no valid image paths found")
	}
	if err := p.setImages(ctx, resolved); err != nil {
		return err
	}
	p.pruneCache(ctx)
	return nil
}

// SetLockscreen implements Platform: changing the macOS lock screen needs
// elevated privileges (R4.4).
func (p *DarwinPlatform) SetLockscreen(context.Context, string) error {
	p.Events.Infof("Lock screen wallpaper on macOS requires elevated privileges. Not supported.")
	return ErrUnsupported
}

// setImages tries AppKit and falls back to osascript, which can only set
// one picture on every desktop: the first existing path.
func (p *DarwinPlatform) setImages(ctx context.Context, paths []string) error {
	err := p.setDesktopImagesNative(paths)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errNoAppKit) {
		p.Events.Errorf("Failed to set wallpaper: %v", err)
		return err
	}
	for _, path := range paths {
		if path != "" {
			return p.setViaOsascript(ctx, path)
		}
	}
	return errors.New("no valid image paths found")
}

// setViaOsascript is the AppleScript fallback. The path is escaped for the
// AppleScript string literal the same way as for JavaScript (both use \" and
// \\), which the Python did not do.
func (p *DarwinPlatform) setViaOsascript(ctx context.Context, path string) error {
	script := `tell application "System Events" to set picture of every desktop to "` + JSStringEscape(path) + `"`
	if _, stderr, err := p.Runner.Run(ctx, "osascript", "-e", script); err != nil {
		p.Events.Errorf("osascript failed: %s", strings.TrimSpace(stderr))
		return fmt.Errorf("osascript failed: %w: %s", err, strings.TrimSpace(stderr))
	}
	return nil
}

func (p *DarwinPlatform) pruneCache(ctx context.Context) {
	if err := PruneWallpaperCache(ctx, p.Runner, MacWallpaperCacheMaxMB); err != nil {
		p.Events.Warnf("Wallpaper cache cleanup failed (non-fatal): %v", err)
	}
}
