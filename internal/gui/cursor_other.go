//go:build !linux

// Copied from angou (same author) — keep in sync by hand.

package gui

// applyCursorTheme is a no-op away from Linux. The problem it works around is
// specific to GLFW's Wayland backend; the macOS backend uses the system cursor
// and needs no help.
func applyCursorTheme() {}
