# Spec 009: Config Watcher Debounce + Unbuffered Service Logging

> **Note**: This work has no associated issue tracker ticket. Consider creating one for traceability.

## Status: COMPLETE

## Context

The service daemon (`--service`, multi-plugin dynamic mode) watches
`~/.config/clockwork-orange.yml` for changes via `watchdog` so the user can
enable/disable sources without restarting. When a change is detected, the wait
loop (`_wait_for_next_cycle`) returns immediately and triggers a wallpaper
switch.

Two problems were observed in production logs (daemon PID 1556, 2026-06-27):

1. **Rapid-fire switching at login.** At service start (`18:22:04`), the config
   file was modified ~4 times within ~0.2 ms (GUI launch + `load_and_migrate`
   rewrite). Each write fired the `ConfigWatcher` independently, so the daemon
   performed ~4 wallpaper switches in a fraction of a second, bypassing the
   60 s `default_wait`. The `ConfigWatcher` has no debounce/coalescing.

2. **Misleading log clumping.** The systemd unit runs `/usr/bin/python3 ...`
   without `-u`/`PYTHONUNBUFFERED`. With stdout piped to journald, Python uses
   block buffering, so ~60 cycles of `[DEBUG]` output flush at once and journald
   stamps them with one identical timestamp. This *looks* like a burst of
   switches but is purely a logging artifact (real cadence is correctly one
   cycle per 60 s).

## Requirements

- Coalesce a burst of rapid config-file writes into a single wallpaper switch.
- Preserve the existing behavior that a genuine (settled) config change still
  interrupts the wait and triggers one prompt switch.
- Keep shutdown responsive while debouncing.
- Make service logs flush promptly so timestamps reflect real cycle timing.
- No regression to the steady-state 60 s cycle cadence.

## Acceptance Criteria

- [x] A `_drain_config_change_burst` helper absorbs further config-change events
      until the file has been quiet for a debounce window, then returns once.
      (`clockwork-orange.py` `_drain_config_change_burst`)
- [x] `_wait_for_next_cycle` calls the debounce helper on the first detected
      change instead of returning immediately, so a flurry of writes yields a
      single trigger. (`clockwork-orange.py` `_wait_for_next_cycle`)
- [x] The debounce helper returns promptly when shutdown is requested (does not
      block for the full sleep interval). (`is_shutdown` guard on the loop;
      `test_returns_promptly_on_shutdown`)
- [x] A single, settled config change still interrupts the wait (does not wait
      the full `default_wait`). (`test_single_change_returns_after_quiet_window`)
- [x] `clockwork-orange.service` runs Python unbuffered (`-u`) so journald
      timestamps track real cycle timing.
- [x] Unit tests cover: (a) coalescing a multi-write burst into one return that
      occurs only after the quiet window, and (b) a single change returning
      after roughly the debounce window (not immediately, not the full wait).
      (`tests/test_config_debounce.py`)
- [x] Existing test suite passes. (12 passed, 2 skipped — Windows-only)

## Risks & Assumptions

- **Rollback**: revert the commit; reinstall/redeploy the previous
  `clockwork-orange.service` and restart the user service. Desktop app, no
  data migration. Per `release-safety/simplified.md`.
- **Assumption**: a 2 s debounce window is long enough to absorb a login-time
  config-write flurry yet short enough to feel responsive for manual edits.
- **Shutdown latency**: debounce adds at most one debounce-window (2 s) of
  latency to shutdown in the worst case; well within systemd `TimeoutStopSec`.
- **Deploy**: the repo `clockwork-orange.service` is copied verbatim to
  `~/.config/systemd/user/` by `_service_install_linux`; the running unit must
  be redeployed + the service restarted for both fixes to take effect.

## Alternatives Considered

- Debounce inside `ConfigWatcher` (timer per event) — rejected; the wait loop
  already owns the event lifecycle, so coalescing there is simpler and keeps the
  watcher a thin event source.
- `PYTHONUNBUFFERED=1` env var instead of `-u` — equivalent; `-u` chosen as it
  is explicit at the `ExecStart` line and travels with the command.

## Executive Summary

Adds debounce coalescing to the service daemon's config-file watcher so a burst
of rapid writes (notably during login) results in a single wallpaper switch
instead of several in quick succession, and runs the systemd service with
unbuffered Python so journald log timestamps reflect real cycle timing rather
than block-buffer flush points. Reviewers should look at
`_drain_config_change_burst` / `_wait_for_next_cycle` in `clockwork-orange.py`
and the `ExecStart` change in `clockwork-orange.service`.
