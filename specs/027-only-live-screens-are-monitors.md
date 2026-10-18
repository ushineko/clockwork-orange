# Spec 027: Only desktops on a live screen are monitors

**Issue**: #47

## Status: INCOMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Low
- **Branch**: `fix/kde-orphan-containments`

## Executive Summary

On KDE, the monitor count and both wallpaper scripts now use only the desktop
containments whose screen exists (`0 ≤ screen < screenCount`), in screen
order. A containment Plasma kept for a disconnected screen no longer counts
as a monitor or takes an image. Recorded as DV17. Reviewers should look at
`LiveDesktopsJS` in `internal/platform/platform_linux.go` and the one-line
substitution the golden test applies.

## Context

On a two-screen Plasma 6 desktop the program picked three wallpapers, and
the Wallpaper card from spec 026 showed a third monitor. `desktops()` returns
every desktop containment, including one Plasma keeps for a screen that is
no longer connected. Here, containment 54 has `screen=2` while `screenCount`
is 2. 2.9.5 counted with `desktops().length` and set with
`desktops()[i]`; the port kept both, so this has been the behaviour since
2.x.

Changing it changes the KDE scripts, which the golden fixture requires to be
byte-identical to the Python. That makes it a deliberate deviation.

## Requirements

- R1 `MonitorCount` on Linux counts the containments with
  `0 ≤ screen < screenCount`. It reports 1 when that count is below 1, as
  it already does when qdbus6 fails.
- R2 The single and multi scripts set only those containments, sorted by
  screen, so image i goes to screen i.
- R3 The scripts are otherwise unchanged from the Python oracle: the golden
  test applies exactly one substitution, `desktops()` → `LiveDesktopsJS`.
- R4 Spec 010's Deliberate Deviations table records it as DV17.

## Acceptance Criteria

- [x] R1 Unit: the count script is the live-desktop expression, and an
      output of `0` gives 1.
- [x] R2/R3 Golden: the single and multi argv equal the Python fixture with
      `desktops()` replaced by `LiveDesktopsJS`, and nothing else.
- [x] R1/R2 Live, this Plasma 6 session (two screens, one orphaned
      containment): the expression yields containments 43 and 44 on screens
      0 and 1, n=2.
- [ ] Live: after reinstall and a service restart, `wallpaper status` lists
      two monitors and the orphan's image is not changed.
- [x] R4 DV17 row added.

## Risks & Assumptions

- `screenCount` and `d.screen` are Plasma 6 scripting API (used by the KDE
  wallpaper scripts in the wild). A Plasma without them would make the
  filter yield nothing, and the count falls back to 1.
- A pin from spec 026 on a monitor index that no longer exists is dropped
  by the rotator, with a log line.
- Rollback: revert; the old count returns.

## E2E Test Plan

- Environment: Plasma 6 with an orphaned containment (this machine).
- Steps: install, restart the service, run `clockwork-orange wallpaper
  status`, and read each containment's image with `evaluateScript` before
  and after a skip.
- Expected: two monitors listed; screens 0 and 1 change; containment 54's
  image does not. Covers R1 and R2.

### Prod visibility

Desktop application: no production service.
