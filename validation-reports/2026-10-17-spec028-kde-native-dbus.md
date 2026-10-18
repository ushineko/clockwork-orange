## Validation Report: Spec 028, KDE wallpapers through Plasma's D-Bus API

**Date**: 2026-10-17
**Spec**: `specs/028-kde-native-dbus.md` (#49)
**Status**: PASSED

### Summary

New code:
- `platform.Plasma`, with a godbus implementation (`plasma_linux.go`);
- three optional platform interfaces: `ScreenSetter`, `WallpaperReader`
  and `WallpaperWatcher`;
- `core.WatchDesktop` and `core.NoteDesktopWallpaper`.

What changed on KDE:
- the desktop set, the read and the screen count no longer use
  `qdbus6`/`evaluateScript`;
- the scripts, `LiveDesktopsJS` (spec 027) and their golden script tests
  are removed. The 2.9.5 fixture stays as the oracle for the
  `kwriteconfig6` argv.

Packaging: the PKGBUILDs drop `qt6-tools`, the deb drops `qdbus-qt6`, and
the self-test drops its `qdbus6` probe.

### Phase 3: Tests

- `make test`: all packages pass.
- New platform tests against a fake Plasma:
  - set every screen;
  - multi-set by screen;
  - a path passed as it is;
  - the screen probe and its fallbacks;
  - a one-screen set;
  - the signal arriving with the screen's image;
  - URL decoding;
  - the lock screen's D-Bus reload.
- New core tests:
  - a pin sets one screen and records the other from the desktop;
  - an outside change is recorded and drops the pin, while the program's
    own change does nothing.
- The tests assert which screen got which image, not the order of calls.

### Live verification

Plasma 6.8 Wayland, two screens and one leftover containment, with the
daemon and the window on this branch:
- skip changed both screens and left the leftover alone;
- `pin --monitor 2` changed only screen 1;
- an outside `setWallpaper` was recorded and dropped the pin;
- `--self-test` passed with `qdbus6` absent from `PATH`.

### Phase 4: Code Quality

- `make lint`: 0 issues.
- Dead code removed: `KDESingleScript`, `KDEMultiScript`, `LiveDesktopsJS`,
  `runQdbus`, `kdeEvaluateArgs` and `screensaverReloadArgs`.
  `JSStringEscape` stays for macOS.
- The daemon builds with `CGO_ENABLED=0`; godbus is pure Go.

### Phase 5: Security Review

- `govulncheck`: no vulnerabilities found.
- Paths are no longer interpolated into JavaScript; they travel as a typed
  D-Bus string. The attack surface DV1 guarded is gone.
- Session bus only. Signals are matched on path, interface and member.

### Phase 5.5: Release Safety

- Rollback: revert; the PKGBUILD's `qt6-tools` returns with it.

### Not verified

- A change made in KDE's own wallpaper dialog. The live outside change used
  `gdbus` on the same D-Bus method.
- The window timer as rotator following outside changes; covered by shared
  core code.

### Overall

**PASSED.**
