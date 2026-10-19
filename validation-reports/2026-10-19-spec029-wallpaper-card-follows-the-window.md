## Validation Report: Spec 029, the Wallpaper card follows the window's own rotation

**Date**: 2026-10-19
**Spec**: `specs/029-wallpaper-card-follows-the-window.md` (#52)
**Status**: PASSED

### Summary

`recordNext` reloads the card in-process after every change, skip and pause
the window's timer makes. `showWindow` reloads a card marked stale while
hidden to the tray. `applyRotation` discards a load that a newer load
superseded.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages pass.
- Three new GUI tests, one per requirement, against real `rotation.json`
  files and locks in a temporary home (R1, R2).
- With the fix reverted, the R1 and R2 tests fail; R3's test needs the new
  function and does not compile without it.
- The tests encode behaviour (what the card's state holds after a change),
  not call order.

### Phase 4: Code Quality

- `make lint`: 0 issues.
- The watcher's inline reload became `rotationChanged`, now shared by the
  watcher, `recordNext` and `showWindow`. No dead code left.

### Phase 5: Security Review

- `govulncheck ./...`: no vulnerabilities found.
- No new input, paths, network calls or credentials. The change reads the
  same state files as before, on the UI thread through `fyne.Do`.
- No secrets in the diff.

### Phase 5.5: Release Safety

- Revert restores the watcher-only behaviour. No on-disk format change.

### Overall

**PASSED.** The macOS E2E steps in the spec are pending until the release
reaches the affected Mac.
