# Spec 029: The Wallpaper card follows the window's own rotation

**Issue**: #52

## Status: COMPLETE

- **Priority**: High
- **Estimated Complexity**: Low
- **Branch**: `fix/wallpaper-card-sync`

## Executive Summary

The window's timer now reloads the Wallpaper card itself after every change,
skip and pause it makes (`recordNext`), so the card no longer depends on the
file watcher reporting the window's own writes. Showing the window from the
tray reloads a stale card, and a card load that finishes after a newer one
started is discarded. Reviewers should look at `recordNext` in
`internal/gui/timer.go` and `applyRotation` in
`internal/gui/views_wallpaper.go`.

## Context

On macOS the Wallpaper card from spec 026, in the Activity section, falls
behind the desktop. After a timed change it keeps the old images and the
countdown reads "Changing now…". Skip clears the countdown but shows the
image the skip just replaced, so the card stays one change behind.

The card's only signal that something changed is a file watcher on
`rotation.json` (`watchRotation`). On macOS and Windows the rotator is the
window's own timer, so the window changes the wallpaper, writes the file, and
then waits for the filesystem to report its own write back to it. On the
affected Mac that report does not reach the card. Either the event does not
arrive (fsnotify's kqueue backend with a rename over an existing file), or it
arrives while the window is hidden to the tray, which only marks the card
stale, and `showWindow` never reloads it. Which of the two has not been
confirmed on the Mac; both are closed here.

Skip reloads the card as soon as it has written the request into
`rotation-control.json`, before the timer has acted on it. That read returns
the state from before the skip, and with no later reload it stays.

Separately, two loads in flight can land out of order, so an older read can
replace a newer one.

## Requirements

- R1 When the window's timer has rotated, paused or rescheduled, it reloads
  the card itself, in-process, without waiting for the file watcher. The
  watcher stays for a rotator in another process: the Linux daemon, or a CLI
  skip or pin.
- R2 A change noticed while the window is hidden to the tray marks the card
  stale. Showing the window again reloads a stale card.
- R3 A card load that finishes after a newer one was started is discarded,
  so the card never goes back to older state.
- R4 Skip's immediate reload stays (it is correct for an in-process skip and
  for Pause and Resume); R1 and R3 make the rotator's reload after it the one
  that lands.

## Acceptance Criteria

- [x] R1 GUI test: after `rotation.json` records a new image, the timer's
      `recordNext` alone (no watcher token) updates the card's state to the
      new image and the new next-change time.
- [x] R2 GUI test: with the window hidden to the tray, a change marks the
      card stale without loading; `showWindow` loads it and the card shows
      the new image.
- [x] R3 GUI test: of two loads, the one started first and finished last is
      discarded.
- [x] R1 and R2 run against real `rotation.json` files and rotation locks in a
      temporary home, not fakes of the state store.
- [x] `make test` and `make lint` pass.
- [x] README changelog line under `### Unreleased`.

## Risks & Assumptions

- The in-process reload runs on the UI thread through `fyne.Do`, as the
  watcher's does. A load with the card off screen only clears the loaded flag.
- When the daemon holds the cycling lock, `recordNext` writes nothing; its
  reload is then a harmless extra read of the daemon's file.
- Whether fsnotify on macOS reports a rename over `rotation.json` stays open.
  After this change it only matters for a CLI skip or pin with the window
  open, and the window's reload after the resulting rotation covers a skip.
- Rollback: revert; the card goes back to depending on the watcher.

## Alternatives Considered

- Considered a debug line in the Activity log per watcher event to settle
  the kqueue question; rejected because it adds two log lines to every
  change for every user, and R1 removes the dependency on macOS.
- Considered removing Skip's immediate reload; rejected because Pause,
  Resume and a skip with no rotator running need it, and R3 makes the
  rotator's later reload the one that lands.

## E2E Test Plan

- Environment: the affected Mac, release build with this change, window
  rotating (no service on macOS).
- Steps:
  1. Open Activity, wait through a timed change. Expected: the card shows the
     new image and a fresh countdown within a second (R1).
  2. Press Skip. Expected: the card shows the image the desktop now shows,
     not the one replaced (R1, R4).
  3. Close the window to the tray, wait through a change, reopen from the
     tray. Expected: the card shows the current image (R2).
- Results: pending, to be run on the Mac after the release.

### Prod visibility

Desktop application: no production service.
