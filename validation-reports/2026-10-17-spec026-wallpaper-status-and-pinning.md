## Validation Report: Spec 026, wallpaper status, pause, skip and pin

**Date**: 2026-10-17
**Spec**: `specs/026-wallpaper-status-and-pinning.md` (#43)
**Status**: PASSED (one manual criterion open; see "Not verified")

### Summary

New core operations:
- `RotationStatus`, `RotationPause`, `RotationResume`, `RotationSkip` and
  `RotationPin`;
- `Rotate` and `RotationTargets`;
- `rotation.json` and `rotation-control.json`, written atomically under a
  cross-process lock.

What uses them:
- `RunLoop`'s wait now wakes on the control file;
- the GUI timer rotates through `Rotate` and watches the control file;
- the CLI gains `wallpaper status|pause|resume|skip|pin`;
- the Service section (and Activity on Windows and macOS) gains a Wallpaper
  card and Pause/Resume and Skip;
- the Review tab gains a Pin menu.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages pass.
- New core tests (`internal/core/rotation_test.go`), against the fake
  platform and real files in a temp directory:
  - state is recorded;
  - concurrent writers serialise;
  - a failed write keeps the previous file;
  - a pinned monitor holds while the other rotates;
  - pause, then resume;
  - skip while paused moves the pins;
  - skip leaves a partial pin alone;
  - a pin applies at once and keeps the other monitor;
  - the loop honours a pin on its first cycle, and a skip cuts its wait;
  - a deleted pin is dropped;
  - bad pin paths are refused.
- New GUI tests (`internal/gui/views_wallpaper_test.go`):
  - the card shows each monitor and the countdown, and Pause becomes Resume
    with no change in the section's minimum size;
  - the Pin menu follows the mode and writes the pin.
- New CLI test: the `wallpaper status` output.
- `tests/parity`: five new leaves mapped; no allow-list change.
- Contract quality: the tests assert which image each target shows, what
  the files hold, and what the window presents. None asserts call order.
  `TestTheControlsThatNeedOneCarryATip` was updated from 2 to 3 tips:
  Pause/Resume carries one, because its effect outlives a restart.

### Live verification

Run on Plasma 6 Wayland, three monitors, with `install.sh` from this branch
and the service restarted. Each step was checked against the images KDE
reports.
- Skip changed all three monitors.
- Pause, then skip while paused: the images changed and the display stayed
  paused.
- Resume restarted the clock.
- `pin --monitor 2` changed only monitor 2.
- Two skips with that pin in place changed only monitors 1 and 3.
- The pin survived `systemctl --user restart`.
- The Service card was checked in a screenshot.

The live run found one defect, which is fixed and has a test: a skip moved a
partial pin.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- `Rotate` was over 50 lines, so its bookkeeping became `recordRotation`.
- `Cycle` is split so `Rotate` shares its source collection
  (`cycleSources`).
- `engine.pickAvoiding` is exported as `PickAvoiding`.
- No dead code.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- Pin paths are validated: absolute, a regular file, an image (R10). The KDE
  script still escapes them (DV1, unchanged).
- The two files live in the user's own state directory, created 0700 by
  `config.MkdirAll`. Their content is parsed as JSON only, and paths taken
  from them are re-checked before they are applied: a stale pin is dropped.
- No secrets, no network, no new external command.

### Phase 5.5: Release Safety

- Additive. An older build ignores the two files.
- Rollback: reinstall the previous release. Deleting the two files returns
  this build to plain rotation.

### Not verified

- Pinning from the Review tab's menu by hand. The menu is covered by a
  headless test.
- A pin surviving a reboot. Restart was verified.
- Windows and macOS, where the GUI timer is the rotator, were not run by
  hand. The timer path is shared code.

### Overall

**PASSED.**
