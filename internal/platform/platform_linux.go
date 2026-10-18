//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// LinuxPlatform drives KDE Plasma 6: the desktop through plasmashell's D-Bus
// interface (spec 028), the lock screen through kwriteconfig6 (R4.4). Events
// receives the "[DEBUG]"/"[ERROR]" lines the Python module printed.
type LinuxPlatform struct {
	Runner Runner
	Events events.Events
	// Plasma is the plasmashell D-Bus client; nil means the session bus.
	Plasma Plasma
}

// New returns the KDE Plasma implementation for this host.
func New(r Runner) Platform { return &LinuxPlatform{Runner: r} }

// NewWithEvents is New with a log sink.
func NewWithEvents(r Runner, ev events.Events) Platform {
	return &LinuxPlatform{Runner: r, Events: ev}
}

// Name implements Platform.
func (*LinuxPlatform) Name() string { return "linux" }

// LockscreenSupported implements Platform.
func (*LinuxPlatform) LockscreenSupported() bool { return true }

const (
	qdbus6       = "qdbus6"
	kwriteconfig = "kwriteconfig6"
)

func (p *LinuxPlatform) plasma() Plasma {
	if p.Plasma == nil {
		return busPlasma{}
	}
	return p.Plasma
}

// maxScreens bounds the probe in MonitorCount.
const maxScreens = 16

/*
MonitorCount implements Platform (R4.1, spec 028 R2): Plasma's screens,
counted by asking each screen number for its wallpaper until one answers with
nothing. 1 when the session bus or plasmashell is unavailable, or no screen
answers.
*/
func (p *LinuxPlatform) MonitorCount(ctx context.Context) int {
	n := 0
	for ; n < maxScreens; n++ {
		_, exists, err := p.plasma().Image(ctx, n)
		if err != nil {
			p.Events.Debugf("Failed to detect monitor count, assuming 1: %v", err)
			return 1
		}
		if !exists {
			break
		}
	}
	if n == 0 {
		p.Events.Debugf("Plasma reports no screen, assuming 1 monitor")
		return 1
	}
	p.Events.Debugf("Detected %d monitors", n)
	return n
}

// KwriteconfigArgs is the kwriteconfig6 argv that writes the lock-screen
// image key (R4.4). path must already be absolute.
func KwriteconfigArgs(path string) []string {
	return []string{kwriteconfig, "--file", "kscreenlockerrc",
		"--group", "Greeter", "--group", "Wallpaper", "--group", "org.kde.image", "--group", "General",
		"--key", "Image", "file://" + path}
}

// SetWallpaper implements Platform (R4.2): the image on every screen.
func (p *LinuxPlatform) SetWallpaper(ctx context.Context, path string) error {
	abs := resolvePath(path)
	p.Events.Debugf("Setting KDE wallpaper from path: %s", abs)
	if !fileExists(abs) {
		p.Events.Errorf("File does not exist: %s", abs)
		return errNotExist(abs)
	}
	return p.setScreens(ctx, []string{abs})
}

// SetWallpaperMulti implements Platform (R4.3): screen i gets
// paths[i % len]. Missing files are logged and skipped; it is an error only
// when none remain.
func (p *LinuxPlatform) SetWallpaperMulti(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return errors.New("no image paths provided")
	}
	resolved := make([]string, 0, len(paths))
	for _, raw := range paths {
		abs := resolvePath(raw)
		if !fileExists(abs) {
			p.Events.Errorf("File does not exist: %s", abs)
			continue
		}
		resolved = append(resolved, abs)
	}
	if len(resolved) == 0 {
		return errors.New("no valid image paths found")
	}
	p.Events.Debugf("Setting wallpapers for multiple monitors: %v", resolved)
	return p.setScreens(ctx, resolved)
}

// setScreens gives screen i images[i % len], on every screen there is.
func (p *LinuxPlatform) setScreens(ctx context.Context, images []string) error {
	var errs []error
	for i := range p.MonitorCount(ctx) {
		if err := p.plasma().SetImage(ctx, i, images[i%len(images)]); err != nil {
			p.Events.Errorf("%v", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SetWallpaperOn implements ScreenSetter (spec 028 R3): one screen, the
// others untouched.
func (p *LinuxPlatform) SetWallpaperOn(ctx context.Context, screen int, path string) error {
	abs := resolvePath(path)
	if !fileExists(abs) {
		p.Events.Errorf("File does not exist: %s", abs)
		return errNotExist(abs)
	}
	p.Events.Debugf("Setting KDE wallpaper on screen %d: %s", screen, abs)
	return p.plasma().SetImage(ctx, screen, abs)
}

// WallpaperOf implements WallpaperReader: "" when the screen does not exist
// or shows something other than an image.
func (p *LinuxPlatform) WallpaperOf(ctx context.Context, screen int) (string, error) {
	path, _, err := p.plasma().Image(ctx, screen)
	return path, err
}

// WatchWallpapers implements WallpaperWatcher (spec 028 R4): every
// wallpaperChanged signal, with the image the screen now shows.
func (p *LinuxPlatform) WatchWallpapers(ctx context.Context) (<-chan ScreenImage, error) {
	screens, err := p.plasma().Changes(ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan ScreenImage, 16)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case screen := <-screens:
				path, exists, err := p.plasma().Image(ctx, screen)
				if err != nil || !exists || path == "" {
					continue
				}
				select {
				case out <- ScreenImage{Screen: screen, Path: path}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// SetLockscreen implements Platform (R4.4): kwriteconfig6 writes the key,
// then the screensaver reload is attempted and its result ignored.
func (p *LinuxPlatform) SetLockscreen(ctx context.Context, path string) error {
	abs := resolvePath(path)
	if !fileExists(abs) {
		return errNotExist(abs)
	}
	argv := KwriteconfigArgs(abs)
	if _, stderr, err := p.Runner.Run(ctx, argv[0], argv[1:]...); err != nil {
		p.Events.Errorf("Failed to set lockscreen: %v", err)
		return fmt.Errorf("kwriteconfig6 failed: %w: %s", err, strings.TrimSpace(stderr))
	}
	_ = p.plasma().ReloadScreenSaver(ctx)
	return nil
}
