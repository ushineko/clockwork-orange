//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// LinuxPlatform drives KDE Plasma 6 through qdbus6 and kwriteconfig6
// (R4.1-R4.4). Events receives the "[DEBUG]"/"[ERROR]" lines the Python
// module printed.
type LinuxPlatform struct {
	Runner Runner
	Events events.Events
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

// kdeEvaluateArgs is the qdbus6 argv that runs script inside plasmashell.
func kdeEvaluateArgs(script string) []string {
	return []string{qdbus6, "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", script}
}

// screensaverReloadArgs is the best-effort kscreenlocker reload (R4.4).
func screensaverReloadArgs() []string {
	return []string{qdbus6, "org.freedesktop.ScreenSaver", "/ScreenSaver", "configure"}
}

// MonitorCount implements Platform (R4.1): plasmashell's desktops().length,
// or 1 when qdbus6 is missing, fails or prints something that is not an int.
func (p *LinuxPlatform) MonitorCount(ctx context.Context) int {
	stdout, _, err := p.Runner.Run(ctx, qdbus6, "org.kde.plasmashell", "/PlasmaShell",
		"org.kde.PlasmaShell.evaluateScript", "print(desktops().length)")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			p.Events.Debugf("qdbus6 not found, assuming 1 monitor")
			return 1
		}
		p.Events.Debugf("Failed to detect monitor count: %v", err)
		return 1
	}
	count, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		p.Events.Debugf("Failed to detect monitor count: %v", err)
		return 1
	}
	p.Events.Debugf("Detected %d monitors", count)
	return count
}

// KDESingleScript is the plasmashell script that sets one image on every
// desktop (R4.2). The text, including its leading newline and indentation,
// is byte-identical to platform_utils.py for paths without characters that
// JSStringEscape has to touch.
func KDESingleScript(path string) string {
	return `
    desktops().forEach(d => {
        d.currentConfigGroup = Array("Wallpaper",
                                     "org.kde.image",
                                     "General");
        d.writeConfig("Image", "file://` + JSStringEscape(path) + `");
        d.reloadConfig();
    });
    `
}

// KDEMultiScript is the plasmashell script that assigns images[i % len] to
// desktop i (R4.3), byte-identical to platform_utils.py for plain paths.
func KDEMultiScript(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = `"file://` + JSStringEscape(p) + `"`
	}
	return `
    var allDesktops = desktops();
    var images = [` + strings.Join(quoted, ", ") + `];

    for (var i = 0; i < allDesktops.length; i++) {
        var d = allDesktops[i];
        d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
        var img = images[i % images.length];
        d.writeConfig("Image", img);
        d.reloadConfig();
    }
    `
}

// KwriteconfigArgs is the kwriteconfig6 argv that writes the lock-screen
// image key (R4.4). path must already be absolute.
func KwriteconfigArgs(path string) []string {
	return []string{kwriteconfig, "--file", "kscreenlockerrc",
		"--group", "Greeter", "--group", "Wallpaper", "--group", "org.kde.image", "--group", "General",
		"--key", "Image", "file://" + path}
}

// runQdbus runs a plasmashell script and translates the two failure modes
// the Python distinguished: missing binary and non-zero exit (with stderr).
func (p *LinuxPlatform) runQdbus(ctx context.Context, script string) error {
	argv := kdeEvaluateArgs(script)
	_, stderr, err := p.Runner.Run(ctx, argv[0], argv[1:]...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			p.Events.Errorf("qdbus6 not found")
			return errors.New("qdbus6 not found")
		}
		p.Events.Errorf("qdbus6 failed: %s", strings.TrimSpace(stderr))
		return fmt.Errorf("qdbus6 failed: %w: %s", err, strings.TrimSpace(stderr))
	}
	return nil
}

// SetWallpaper implements Platform (R4.2).
func (p *LinuxPlatform) SetWallpaper(ctx context.Context, path string) error {
	abs := resolvePath(path)
	p.Events.Debugf("Setting KDE wallpaper from path: %s", abs)
	if !fileExists(abs) {
		p.Events.Errorf("File does not exist: %s", abs)
		return errNotExist(abs)
	}
	return p.runQdbus(ctx, KDESingleScript(abs))
}

// SetWallpaperMulti implements Platform (R4.3). Missing files are logged and
// skipped; it is an error only when none remain.
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
	return p.runQdbus(ctx, KDEMultiScript(resolved))
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
	reload := screensaverReloadArgs()
	_, _, _ = p.Runner.Run(ctx, reload[0], reload[1:]...)
	return nil
}
