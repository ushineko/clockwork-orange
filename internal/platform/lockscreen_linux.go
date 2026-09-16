//go:build linux

package platform

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// lockscreenWallpaperSection is the kscreenlockerrc group kwriteconfig6
// writes the image into; KDE flattens nested groups into one header.
const lockscreenWallpaperSection = "Greeter][Wallpaper][org.kde.image][General"

// iniSection is one [header] block of a KDE rc file, keys in file order.
type iniSection struct {
	Name string
	Keys []iniKey
}

type iniKey struct{ Key, Value string }

func (s iniSection) lookup(key string) (string, bool) {
	for _, k := range s.Keys {
		if k.Key == key {
			return k.Value, true
		}
	}
	return "", false
}

// parseKDERC reads a KDE rc file. Section headers such as
// "[Greeter][Wallpaper][org.kde.image][General]" are kept as the single
// string between the outermost brackets. Unlike Python's configparser it
// keeps key case and tolerates duplicate keys (the reason the Python had a
// "clean up redundant entries" step at all).
func parseKDERC(r io.Reader) ([]iniSection, error) {
	var sections []iniSection
	cur := -1
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			sections = append(sections, iniSection{Name: line[1 : len(line)-1]})
			cur = len(sections) - 1
		default:
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			if cur < 0 {
				sections = append(sections, iniSection{Name: ""})
				cur = 0
			}
			sections[cur].Keys = append(sections[cur].Keys,
				iniKey{Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read rc file: %w", err)
	}
	return sections, nil
}

// DebugLockscreen writes the kscreenlockerrc dump behind --debug-lockscreen
// (R4.4): every section and key, then the lock-screen image key and the
// legacy Greeter/wallpaper key. The file is $XDG_CONFIG_HOME/kscreenlockerrc,
// which is where kwriteconfig6 writes it.
func DebugLockscreen(w io.Writer) error {
	return debugLockscreenFile(w, filepath.Join(configHome(), "kscreenlockerrc"))
}

// linePrinter writes formatted lines and remembers the first write error so
// the dump reads as plain print statements.
type linePrinter struct {
	w   io.Writer
	err error
}

func (p *linePrinter) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	if _, err := fmt.Fprintf(p.w, format, args...); err != nil {
		p.err = fmt.Errorf("write debug output: %w", err)
	}
}

func debugLockscreenFile(w io.Writer, path string) error {
	out := &linePrinter{w: w}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			out.printf("[DEBUG] Configuration file does not exist: %s\n", path)
			return out.err
		}
		out.printf("[ERROR] Failed to read configuration file: %v\n", err)
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sections, err := parseKDERC(f)
	if err != nil {
		out.printf("[ERROR] Failed to read configuration file: %v\n", err)
		return err
	}

	out.printf("[DEBUG] Configuration sections found:\n")
	for _, s := range sections {
		out.printf("[DEBUG]   - %s\n", s.Name)
		for _, k := range s.Keys {
			out.printf("[DEBUG]     %s = %s\n", k.Key, k.Value)
		}
	}

	var wallpaper, greeter *iniSection
	for i := range sections {
		switch sections[i].Name {
		case lockscreenWallpaperSection:
			wallpaper = &sections[i]
		case "Greeter":
			greeter = &sections[i]
		}
	}

	if wallpaper == nil {
		out.printf("[DEBUG] Wallpaper section %s not found\n", lockscreenWallpaperSection)
	} else if v, ok := wallpaper.lookup("Image"); ok {
		out.printf("[DEBUG] Current lock screen wallpaper: %s\n", v)
	} else if v, ok := wallpaper.lookup("image"); ok {
		out.printf("[DEBUG] Current lock screen wallpaper: %s\n", v)
	} else {
		out.printf("[DEBUG] No Image key found in %s\n", lockscreenWallpaperSection)
	}

	if greeter != nil {
		if v, ok := greeter.lookup("wallpaper"); ok {
			out.printf("[DEBUG] Main Greeter wallpaper: %s\n", v)
			return out.err
		}
	}
	out.printf("[DEBUG] No wallpaper key found in Greeter section\n")
	return out.err
}

// cleanLockscreenConfig is the port of clockwork-orange.py
// _clean_lockscreen_config: it deletes the lowercase duplicate keys that
// older releases left in kscreenlockerrc. Ported for completeness; nothing
// calls it yet (the Python only ran it from the lock-screen CLI path).
func cleanLockscreenConfig(ctx context.Context, r Runner, ev events.Events) {
	ev.Debugf("Cleaning up redundant configuration entries...")
	del := func(args ...string) {
		argv := append([]string{kwriteconfig, "--file", "kscreenlockerrc"}, args...)
		argv = append(argv, "--delete")
		_, _, _ = r.Run(ctx, argv[0], argv[1:]...)
	}
	del("--group", "Greeter", "--group", "Wallpaper", "--group", "org.kde.image", "--group", "General", "--key", "image")
	del("--group", "Greeter", "--key", "wallpaper")
	for _, key := range []string{"lockonresume", "timeout", "autolock"} {
		del("--group", "Daemon", "--key", key)
	}
	ev.Debugf("Redundant entries cleaned up")
}

// reloadScreensaverConfig is the port of clockwork-orange.py
// _reload_screensaver_config: it pokes every service/method pair kscreenlocker
// has answered to and reports whether any succeeded. Ported for completeness;
// SetLockscreen uses the single best-effort call the Python setter made.
func reloadScreensaverConfig(ctx context.Context, r Runner, ev events.Events) bool {
	ev.Debugf("Attempting to reload screen saver configuration...")
	success := false
	for _, service := range []string{"org.freedesktop.ScreenSaver", "org.kde.screensaver"} {
		for _, method := range []string{"configure", "org.kde.screensaver.configure"} {
			if _, _, err := r.Run(ctx, qdbus6, service, "/ScreenSaver", method); err == nil {
				ev.Debugf("Successfully called %s on %s", method, service)
				success = true
			}
		}
	}
	if !success {
		ev.Warnf("All attempts to reload screen saver configuration failed")
		ev.Debugf("You may need to log out and back in for changes to take effect")
	} else {
		ev.Debugf("Screen saver configuration reload signal sent")
	}
	return success
}
