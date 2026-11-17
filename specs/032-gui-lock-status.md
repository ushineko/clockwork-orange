# Spec 032: The Wallpaper card keeps reporting the window's timer as running

**Issue**: #63

## Status: INCOMPLETE (E2E on the affected Mac pending the release)

- **Priority**: High
- **Estimated Complexity**: Low
- **Branch**: `fix/gui-lock-status`

## Executive Summary

`core.GUIRunning` now answers true while this process holds the GUI
single-instance lock, instead of probing the lock file again. A window left
open for a while no longer drifts to "Not rotating: no window timer is
running." Reviewers should look at `TryGUILock` and `guiHeld` in
`internal/core/wallpaper.go`.

## Context

On macOS, after the window has been up for a while, the Wallpaper card in
Activity reads "Not rotating: no window timer is running." The timer keeps
changing wallpapers; only the display is wrong, and restarting the app
clears it. On macOS and Windows the window in the tray is the only
long-running process, so its status has to stay accurate.

`RotationStatus` decides `Running` for a GUI-rotated state by calling
`GUIRunning`, which tries to take `/tmp/clockwork_orange_gui_lock.lock` and
reports "running" only when that fails. The window takes that lock at launch
and holds it through an open file description. If the lock file is removed
while the window runs, the probe creates a new file, takes the lock, and
reports "not running", although the window is alive.

The likely remover is macOS, which clears old files from `/tmp`. That has
not been confirmed on the affected Mac. The in-process fix is correct for any
cause that makes the probe succeed against a live window.

## Requirements

- R1. While this process holds the GUI lock, `GUIRunning` returns true
  without probing the file.
- R2. Releasing the lock clears that state, so a process that has released it
  falls back to probing the file.

## Acceptance Criteria

- [x] With the GUI lock held and its lock file deleted, `GUIRunning` is true
  (`TestGUIRunningSurvivesLockFileRemoval`).
- [x] After the release function runs, `GUIRunning` is false
  (same test).
- [x] `make test` and `make lint` pass.
- [ ] On the affected Mac, a window left open past the point where the card
  used to drift keeps showing the countdown (E2E step 1).

## Risks & Assumptions

- Assumes the lock file is being removed under the window. If the card still
  drifts after this change, the cause is elsewhere; E2E step 2 distinguishes
  the two.
- Not addressed: with the lock file gone, a second launch can take a fresh
  lock and start a second window and timer. Moving the lock file changes the
  location the daemon and 2.9.x share, so it is left for a separate spec.
- The CLI (`wallpaper status`) is a different process and still probes the
  file; it is unaffected by this change.
- Rollback: revert; the card goes back to probing the file.

## Alternatives Considered

- Considered moving the lock file out of `/tmp` on macOS; rejected here
  because it changes a path shared with other versions and does not help a
  window that is already running.

## E2E Test Plan

- Environment: the affected Mac, release build with this change, window
  rotating.
- Steps:
  1. Leave the window open (visible or in the tray) for longer than it took
     the card to drift before. Expected: the card still shows a countdown
     (R1).
  2. If it drifts anyway, run `ls -l /tmp/clockwork_orange_gui_lock.lock`.
     Expected on a fix of the right cause: card unaffected; a missing file
     with a wrong card would mean the fix did not take.
- Results: pending the release.

### Prod visibility

Desktop application: no production service.
