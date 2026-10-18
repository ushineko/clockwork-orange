# Spec 028: KDE wallpapers through Plasma's D-Bus API

**Issue**: #49

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Medium
- **Branch**: `feat/kde-native-dbus`

## Executive Summary

On KDE the desktop is now set and read over D-Bus with plasmashell's
per-screen `setWallpaper` and `wallpaper`, from Go through godbus. There is
no `qdbus6` process and no `evaluateScript` JavaScript. Screens are counted by
probing `wallpaper(n)`, so leftover containments never take part. A pin sets
only its own screen. A wallpaper changed outside the program, for example in
KDE's settings, is recorded in `rotation.json` and drops that screen's pin.
`qdbus6` is no longer a runtime dependency. Recorded as DV18. Reviewers
should start with `internal/platform/plasma_linux.go` and
`NoteDesktopWallpaper` in `internal/core/rotation.go`.

## Context

On Linux, the desktop wallpaper is set by starting `qdbus6` with a
plasmashell script (`org.kde.PlasmaShell.evaluateScript`). The script walks
the desktop containments, and since spec 027 only those on a live screen. The
monitor count is another script. This is a port of 2.9.5's
`platform_utils.py`; the golden fixture `tests/golden/platform/kde_commands.json`
holds its exact argv.

Plasma 6 exposes a per-screen API on `org.kde.plasmashell /PlasmaShell`,
checked on this machine (Plasma 6.8.0):

- `setWallpaper(QString plugin, QVariantMap params, uint screen)`.
  `params = {Image: "file:///…"}` with plugin `org.kde.image` set screen 1
  live. A screen that does not exist is ignored silently.
- `wallpaper(uint screen) → QVariantMap`, which includes `Image`. A screen
  that does not exist returns an empty map.
- signal `wallpaperChanged(uint screen)`.

godbus (`github.com/godbus/dbus/v5`) is already a direct dependency, used for
notifications. It is pure Go, so the daemon still builds with
`CGO_ENABLED=0`.

## Requirements

- R1 Desktop sets on Linux go through `setWallpaper` on the session bus,
  one call per screen. Neither `qdbus6` nor a script is involved.
  - `SetWallpaper(path)` sets every screen to the image.
  - `SetWallpaperMulti(paths)` gives screen i `paths[i % len]`.
  - Missing files are logged and skipped, as before.
- R2 `MonitorCount` probes `wallpaper(i)` from 0 until a screen returns an
  empty map, capped at 16. It answers 1 when the bus or plasmashell is
  unavailable or the probe finds none. This replaces spec 027's script and
  keeps its outcome: screens, not containments.
- R3 A new optional interface `platform.ScreenSetter` (`SetWallpaperOn(ctx,
  screen, path)`) sets one screen. Linux implements it.
  `core.RotationPin` uses it when the platform has it, so pinning monitor N
  touches only screen N. Other platforms keep the multi-set fallback.
- R4 Status follows the desktop:
  - a `platform.Watcher` on Linux reports `wallpaperChanged(screen)`;
  - the rotator reads `wallpaper(screen)` and records it in `rotation.json`,
    so a wallpaper changed in KDE's settings shows on the card. It handles
    the signal between its own changes, so an image it set itself is
    already recorded and is not taken for an outside change. The window
    timer does the same while it is the rotator;
  - a pinned screen changed this way is unpinned, with a log line, because
    the user chose something else.
- R5 The lock-screen reload (`org.freedesktop.ScreenSaver.configure`) goes
  over godbus too. The lock-screen image itself stays `kwriteconfig6`,
  because Plasma offers no D-Bus setter for `kscreenlockerrc`.
- R6 `qdbus6` is no longer needed at runtime:
  - `--self-test` stops requiring it;
  - the PKGBUILDs drop `qt6-tools`;
  - the deb drops `qdbus-qt6` from Recommends.
  `--debug-lockscreen` keeps whatever it uses, as a diagnostic.
- R7 Spec 010 records DV18, which replaces DV1 for the desktop: there is no
  JS left to escape. The golden fixture stays as the 2.9.5 oracle. The
  single and multi argv tests are replaced by tests of the D-Bus calls, and
  the lock screen and reload goldens stay.

## Acceptance Criteria

- [x] R1 Unit, against a fake bus object: a single set calls `setWallpaper`
      once per screen with `org.kde.image` and `{Image: file://…}`. A multi
      set of two paths on three screens gives 0→a, 1→b, 2→a.
- [x] R2 Unit: the probe stops at the first empty map, caps at 16, and
      answers 1 when the bus errors.
- [x] R3 Core test: a pin on monitor 2 with a `ScreenSetter` platform makes
      exactly one call, for screen 1.
- [x] R4 Core test: an external change event on a pinned screen updates
      `rotation.json` and drops the pin.
- [x] R6 `--self-test` passes with `qdbus6` absent from `PATH` (live, on
      this machine), its test asserts no `qdbus6` probe, and neither PKGBUILD
      lists `qt6-tools`.
- [x] Live, Plasma 6.8, two screens and one leftover containment, the
      daemon on this branch:
      - skip changed screens 0 and 1 and left the leftover's image as it was;
      - `pin --monitor 2` changed only screen 1;
      - an outside `setWallpaper` on screen 1 was recorded in `status`
        within 2 s and logged "Monitor 2 is no longer pinned";
      - the program's own sets were not logged as outside changes.

      The outside change came from `gdbus`, not from KDE's dialog. Both go
      through the same `wallpaperChanged` signal.
- [x] R7 DV18 row added; DV1 notes the desktop path no longer applies.

## Test Strategy

- **Level**: unit tests against a small `plasmaBus` interface that wraps
  the three methods and the signal, with a fake; core tests through the fake
  platform; one env-gated live test against the real session bus.
- **Contracts**: which screen got which image, and what the card shows after
  an external change. Not the order of calls.
- **Mock boundary**: the D-Bus object. The integration boundary is the live
  test, run here on Plasma 6.8 before merge.

## Risks & Assumptions

- Plasma 6.0+ has these methods: they arrived in Plasma 5.25. Older Plasma
  is unsupported by v4 already.
- Screen numbers are Plasma's. They are the same numbering the spec 027
  filter used, so a pin's monitor index keeps its meaning.
- `setWallpaper` of a screen that does not exist is a no-op, so the probe
  (R2) is the only guard against a wrong count. A screen unplugged between
  the probe and the set loses nothing.
- Rollback: revert. The qdbus6 path returns, and the PKGBUILD needs
  `qt6-tools` back in the same revert.

## E2E Test Plan

- Environment: this machine, Plasma 6.8 Wayland, two screens plus the
  orphaned containment, the service installed from the branch.
- Steps:
  1. `wallpaper skip`, then read `wallpaper(0)` and `wallpaper(1)`: both
     changed (R1).
  2. `wallpaper pin --monitor 2 FILE`: only `wallpaper(1)` changed (R3).
  3. Set screen 1 in KDE's "Configure Desktop and Wallpaper": the card
     updates (R4).
  4. Run `PATH=/usr/bin:/bin` with `qdbus6` moved aside: `--self-test`
     passes and a skip works (R6).

### Prod visibility

Desktop application: no production service.
