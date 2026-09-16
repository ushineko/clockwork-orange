//go:build linux

// Copied from angou (same author) — keep in sync by hand.

package gui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Cursor theme, on Wayland.
//
// GLFW's Wayland backend does not implement the cursor-shape-v1 protocol, which
// is how a native Wayland client asks the compositor for the user's themed
// pointer. Its only fallback is the XCURSOR_THEME and XCURSOR_SIZE environment
// variables (glfw/src/wl_init.c). Plasma sets those for XWayland clients but not
// for native ones, which are expected to use the protocol GLFW lacks — so the
// window gets the default pointer while every other window on the desktop has
// the user's, and the cursor visibly changes as it crosses our border.
//
// This reads the theme out of the desktop's own configuration and sets the
// variables before GLFW starts. It is a workaround for a toolkit gap, not a
// feature, and it disappears if GLFW gains cursor-shape-v1.
//
// Config files are read directly rather than shelling out to gsettings or
// kreadconfig: the only subprocess this project starts is MBINCompiler, and a
// cursor theme is not a good enough reason to add a second.

// cursorSources are the files consulted, in order of authority. The first file
// that yields a theme name wins.
func cursorSources(home string) []struct{ path, themeKey, sizeKey string } {
	return []struct{ path, themeKey, sizeKey string }{
		{filepath.Join(home, ".config", "kcminputrc"), "cursorTheme", "cursorSize"},
		{filepath.Join(home, ".config", "gtk-4.0", "settings.ini"), "gtk-cursor-theme-name", "gtk-cursor-theme-size"},
		{filepath.Join(home, ".config", "gtk-3.0", "settings.ini"), "gtk-cursor-theme-name", "gtk-cursor-theme-size"},
		{filepath.Join(home, ".gtkrc-2.0"), "gtk-cursor-theme-name", "gtk-cursor-theme-size"},
	}
}

// applyCursorTheme sets XCURSOR_THEME and XCURSOR_SIZE from the desktop's
// configuration when they are not already set. It must run before the toolkit
// initializes, and it never overrides a value the user exported themselves.
func applyCursorTheme() {
	if os.Getenv("XCURSOR_THEME") != "" {
		return // the user, or the session, has already decided
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	for _, src := range cursorSources(home) {
		theme, size := readCursorConfig(src.path, src.themeKey, src.sizeKey)
		if theme == "" {
			continue
		}
		_ = os.Setenv("XCURSOR_THEME", theme)
		if size != "" && os.Getenv("XCURSOR_SIZE") == "" {
			_ = os.Setenv("XCURSOR_SIZE", size)
		}
		return
	}
}

// readCursorConfig pulls two keys out of an INI-shaped file. Sections are
// ignored: the keys are distinctive enough that matching on the key alone
// cannot collide, and tracking sections would mean handling the differences
// between KDE's and GTK's file layouts for no gain.
func readCursorConfig(path, themeKey, sizeKey string) (string, string) {
	f, err := os.Open(path) //nolint:gosec // a fixed path under the user's own config directory
	if err != nil {
		return "", ""
	}
	defer func() { _ = f.Close() }()

	var theme, size string
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scan.Text()), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		// gtkrc-2.0 quotes its values; the .ini files do not.
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if value == "" {
			continue
		}
		switch key {
		case themeKey:
			theme = value
		case sizeKey:
			size = value
		}
	}
	return theme, size
}
