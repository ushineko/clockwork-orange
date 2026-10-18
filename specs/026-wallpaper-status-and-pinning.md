# Spec 026: Wallpaper status, pause, skip and pin

**Issue**: #43

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: High
- **Branch**: `feat/wallpaper-pin`

## Executive Summary

The daemon and the window now record what is on screen in `rotation.json`
and take pause, skip and pin requests from `rotation-control.json`. Both
files are written atomically under a cross-process lock. The Service section
(Activity on Windows and macOS) shows a tile per monitor and the lock
screen, with a countdown, Pause/Resume and Skip. The Review tab has a Pin
menu, and `clockwork-orange wallpaper status|pause|resume|skip|pin` covers
the same operations. Reviewers should start with `core.Rotate` and
`waitForRotation` (`internal/core/rotation.go`, `wallpaper.go`): they are
the pin-aware selection and the loop's new wait.

## Context

Nothing tells the user which wallpaper is on screen, where it came from, or
when it changes next. The chosen paths only appear in debug log lines. The
only way to hold a wallpaper is to stop the service, which also stops the
lock screen, and the only way to get a new one early is to edit the config so
the watcher cuts the wait short.

Two different things rotate wallpapers. On Linux it is the daemon
(`clockwork-orange --service`, `core.RunLoop`). On Windows and macOS, and on
Linux while the daemon is not running, it is the GUI's timer
(`internal/gui/timer.go`), which stays idle while the daemon holds the
cycling lock. The daemon and the GUI share only that lock and the config
file. There is no IPC between them.

This is new behaviour with no 2.9.5 equivalent. The spec 010 port table is
unaffected, and 2.9.x ignores the new files.

## Design

### Two files

Both live in the state directory beside `history.db`
(`~/.config/clockwork-orange/`).

- **`rotation.json`**: what is on screen. The rotator (the daemon or the GUI
  timer) writes it after each change and whenever its schedule changes.
  `RotationPin` writes it too, because a pin puts an image on screen without
  waiting for the rotator. It holds:
  - who rotates: `daemon`, `gui`, or `cli` for a skip run with nothing
    rotating;
  - the mode;
  - the desktop images, one per monitor in desktop order, each with the time
    it was set;
  - the lock-screen image, when the mode sets one;
  - `next_change`: empty while everything is pinned.
- **`rotation-control.json`**: what the user asked for. The front ends write
  it. It holds:
  - the pins: `desktop` (monitor index → path) and `lockscreen`;
  - `skip`: a counter, incremented by each skip request.

  The rotator writes it in only two cases: to drop a pin whose file is gone
  (R9), and to move the pins of a paused display to the images a skip chose.
  The file persists, so a pin survives a restart and a reboot.

Every write is atomic (temp file, then rename), and every read-modify-write
holds one cross-process lock (`clockwork_orange_rotation_lock`). So a
concurrent CLI, GUI and daemon cannot lose each other's change. The rotator
watches the control file the way it already watches the config, and a change
cuts its wait short.

"Is the rotator alive" needs no pid. A daemon is alive while it holds the
cycling lock. A window is alive while it holds the GUI single-instance lock
and the daemon does not hold the cycling lock.

### Operations (core, then CLI and GUI)

| Core | CLI | GUI |
|---|---|---|
| `RotationStatus` | `wallpaper status` | Service section: status card |
| `RotationPause` | `wallpaper pause` | Service: Pause |
| `RotationResume` | `wallpaper resume` | Service: Resume |
| `RotationSkip` | `wallpaper skip` | Service: Skip |
| `RotationPin` | `wallpaper pin <path> [--monitor N \| --all \| --lockscreen]` | Review: Pin ▾ (monitor N / all monitors / lock screen) |

- **Pause** pins what is on screen now: every monitor, and the lock screen
  when the mode sets one.
- **Resume** clears every pin.
- **Skip** asks for a new selection now, and the timer restarts.
  - On a paused display (every target pinned), skip picks new images and
    moves the pins to them, so the display stays paused on the new images.
  - With only some targets pinned, the pinned ones keep their images and the
    rest change. Found in the live run: moving a partial pin made a pinned
    monitor change, which is what a pin exists to stop.
  - With nothing rotating, the skip runs in the calling process.
- **Pin** applies the image to its target at once, through the platform,
  without waiting for the rotator. It works when nothing is rotating, and the
  pin is recorded for when rotation starts. The other monitors keep the
  images `rotation.json` says they have.

### Rotation with pins

- Monitors that are pinned keep their image.
- Monitors that are not pinned draw as today, still avoiding the pinned
  images (the `PickDistinct` rule).
- The lock screen is the same: pinned keeps, otherwise it draws.
- When every target the mode sets is pinned, the rotator sleeps until the
  control file or the config changes, and `next_change` is empty.

### Status when nothing is rotating

`RotationStatus` reports the rotator as stopped when the process named in
`rotation.json` no longer holds its lock (see "Two files"). It still reports
the last known images and the pins.

## Requirements

- R1 The rotator writes `rotation.json` atomically after every cycle and on
  every schedule change; a pin updates it too. Contents as above.
- R2 `rotation-control.json` is written atomically under the rotation lock;
  the rotator writes it only for R9 and for a skip on a paused display.
- R3 The daemon (`RunLoop`) and the GUI timer both read the control file
  before each cycle, honour pins, and wake on a change to it.
- R4 Pause, resume, skip and pin behave as in the Design section. A pin
  survives a restart of either rotator.
- R5 Pin applies at once, including when nothing is rotating, and keeps the
  images of the monitors not named.
- R6 Every operation exists in `internal/core`, the cobra CLI and the GUI in
  the same commit. `tests/parity` covers them, with no new allow-list entry.
- R7 Service section (Linux) and Activity section (Windows, macOS):
  - a status card shows a fixed-size tile for each monitor and for the lock
    screen when the mode sets one. Each tile has a thumbnail, the target's
    name with "pinned" when it is pinned, and the file name. The full path
    and the image's age are the tile's tip. Under the tiles: "Next change in
    m:ss", or "Paused", or "Not rotating";
  - Pause/Resume and Skip sit in the Wallpaper card, leading the countdown
    line.
    The first version put them in the affixed Control card beside Start and
    Stop, and in review they read as controls of the service;
  - the card updates when `rotation.json` changes, and the countdown ticks
    locally once a second;
  - nothing reflows: thumbnails and lines have fixed sizes.
- R8 The Review tab has a Pin button with a menu: each monitor by number, All
  monitors, and Lock screen. The menu offers only the targets the platform
  and the mode have. Pinning reports through the shell's banner.
- R9 A pinned image that is deleted (blacklist, retention, by hand) is
  unpinned on the next cycle, and rotation resumes for that target. The
  status says so once in the log.
- R10 Paths in the control file are validated as absolute paths to existing
  regular files before they are applied. KDE payload escaping (DV1) applies
  unchanged.

## Acceptance Criteria

- [x] R1/R2 Unit: round-trip of both files; a write interrupted before the
      rename leaves the previous file intact; two concurrent control writers
      both land.
- [x] R3/R4 Core test with the fake platform: a pinned monitor keeps its
      image over three cycles while an unpinned one changes; pause pins all;
      resume clears; skip on a paused display changes the image and stays
      paused.
- [x] R4 Core test: a pin written before `RunLoop` starts is honoured on its
      first cycle.
- [x] R3 Core test: a control-file change cuts the loop's wait short (fake
      clock or a short wait bound, no sleep-based timing).
- [x] R5 Core test: pin with no rotator running calls the platform once with
      the other monitors' last images unchanged
      (`TestPinAppliesAtOnceAndKeepsTheOtherMonitor`, against the fake
      platform; the KDE script for a monitor list is the existing
      `KDEMultiScript` golden test).
- [x] R6 `tests/parity` passes with the new leaves mapped and no allow-list
      change.
- [x] R7 Headless GUI test: the Service status card shows two monitors and
      the countdown from a fixture `rotation.json`; Pause flips to Resume; the
      section's minimum size does not change between the paused and running
      states.
- [x] R8 Headless GUI test: the Review Pin menu lists the monitors from the
      fake platform, and choosing one writes the pin.
- [x] R9 Core test: deleting a pinned file unpins it on the next cycle.
- [x] R10 Core test: a relative path, a missing file and a directory are
      each refused with an error naming the path.
- [x] R4 Core test: with one monitor pinned, skip changes only the others
      (`TestSkipLeavesAPartialPinAlone`).
- [x] Live, Plasma 6 Wayland, three monitors, the daemon on this build. Each
      step was checked against the images KDE reports
      (`desktops()[i].readConfig("Image")`):
      - skip changed all three monitors;
      - pause stopped the clock;
      - skip while paused changed all three and stayed paused;
      - resume restarted the clock;
      - `pin --monitor 2` changed only monitor 2;
      - two skips left monitor 2 alone and changed monitors 1 and 3;
      - `systemctl --user restart` kept the pin.

      The Service card showed the tiles, the pin marker, the countdown and
      Resume.
- [x] Live: the user pinned "All monitors" from the Review tab's menu, and
      the Service card showed every monitor pinned to that image. Persistence
      is the control file read at rotator start, so the service restart
      above stands in for a reboot. The user chose not to reboot.

## Test Strategy

- **Level**: mostly core tests against the existing fake platform and a temp
  config dir; headless Fyne tests for the two views; one env-gated live KDE
  run for the on-screen behaviour.
- **Contracts encoded**: which image each target shows after a sequence of
  operations and cycles; that the files survive a restart; that the GUI
  presents what `rotation.json` says. These are behavioural contracts, not
  checks of internal call order.
- **Mock boundary**: the `platform.Platform` fake only (it already records
  the argv and scripts). The files are real files in a temp dir.
- **Integration boundary**: the platform call is the boundary. It is
  covered by the golden KDE script test and the live run, not by mocks alone.

## Risks & Assumptions

- Assumption: skip on a paused display picks a new image and stays paused.
  The alternative (skip resumes) is one line to change.
- Assumption: per-monitor pins are by KDE desktop index. If monitors are
  added or removed, a pin for an index that no longer exists is dropped with
  a log line.
- Windows and macOS count monitors natively (`MonitorCount`), so the Pin
  menu lists them there too. Lock screen appears only where the platform
  sets one.
- Runtime state goes in two new files, not in the user's YAML config. 2.9.x
  ignores them.
- Rollback: revert. The new files are inert to an older build, and
  deleting them returns to plain rotation.
- Query and IO budget: one small JSON write per cycle; the GUI reads on an
  fsnotify event, not by polling.

## E2E Test Plan

- Environment: `make build build-gui` on KDE Plasma 6 Wayland, two monitors,
  the service installed and running.
- Steps and expected results:
  1. Open the GUI → Service. Two thumbnails show, and the countdown matches
     `default_wait`. (R7)
  2. Click Skip. Both monitors change within about 2 s, and the countdown
     restarts. (R4)
  3. Click Pause, then wait past `default_wait`. Nothing changes, and the
     card shows Paused. (R4)
  4. In a plugin's Review tab, choose Pin ▾ → Monitor 2. Monitor 2 shows the
     reviewed image at once. (R5, R8)
  5. Resume, then pin monitor 1 only. Skip twice; only monitor 2 changes.
     (R3)
  6. Run `systemctl --user restart clockwork-orange` and reboot. Monitor 1 is
     still pinned. (R4)
  7. `clockwork-orange wallpaper status` prints the same state as the card.
     (R6)
- Windows: step 2 and step 3 in the Activity section, with the GUI timer as
  the rotator.

### Prod visibility

Desktop application: no production service. Users see the state in the
Service section and in `clockwork-orange wallpaper status`.
