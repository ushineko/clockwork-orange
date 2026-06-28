## Validation Report: Config Watcher Debounce + Unbuffered Service Logging

**Date**: 2026-06-27
**Spec**: specs/009-config-watcher-debounce.md
**Branch**: main
**Status**: PASSED (with pre-existing dependency advisories)

### Summary

Adds debounce coalescing to the service daemon's config-file watcher so a burst
of rapid writes (notably during login) results in a single wallpaper switch
instead of several in quick succession, and runs the systemd service with
unbuffered Python (`-u`) so journald log timestamps reflect real cycle timing.

Root cause (from daemon PID 1556 logs, 2026-06-27):
- Visible rapid switching = ConfigWatcher firing once per config write with no
  debounce; ~4 writes within ~0.2 ms at login => ~4 instant switches.
- Apparent hourly "burst" in logs = stdout block buffering (no `-u`); real
  cadence was correctly one cycle per 60 s.

### Phase 3: Tests

- Command: `python3 -m pytest tests/ test_platform_utils_build.py`
- Results: **12 passed, 2 skipped** (skips are Windows-only watchdog APIs)
- New tests: `tests/test_config_debounce.py` (3 cases)
  - `test_single_change_returns_after_quiet_window` — a settled change returns
    after ~debounce, not instantly, not the full wait.
  - `test_burst_is_coalesced_into_single_return` — a multi-write burst keeps
    resetting the quiet window; returns only after writes settle.
  - `test_returns_promptly_on_shutdown` — shutdown short-circuits the wait.
- `python3 -m py_compile clockwork-orange.py` — OK
- Status: ✓ PASSED

### Phase 4: Code Quality

- Change is surgical: one module constant (`CONFIG_DEBOUNCE_SECONDS`), one new
  helper (`_drain_config_change_burst`), a 4-line change in
  `_wait_for_next_cycle`, and a one-line `ExecStart` edit.
- No dead code introduced; no duplication; helper is independently testable.
- Status: ✓ PASSED

### Phase 5: Security Review

- Dependency scan: `pip-audit -r requirements.txt` (pip-audit, miniforge).
  - **8 known vulnerabilities in 2 packages — all PRE-EXISTING, not introduced
    by this change** (no dependency edits):
    - `pillow 12.0.0`: PYSEC-2026-165, CVE-2026-25990, CVE-2026-40192,
      CVE-2026-42309/42310/42311 (fix: 12.2.0)
    - `requests 2.32.5`: CVE-2026-25645 (fix: 2.33.0)
  - **Recommendation (follow-up, out of scope for this fix)**: bump pillow ->
    12.2.0 and requests -> 2.33.0 in a dedicated dependency-update change.
- Changed-code review: no secrets/credentials; no new network, file, or
  subprocess surface. Debounce is internal thread-coordination logic; the
  service edit only adds the `-u` interpreter flag.
- Status: ✓ PASSED (no new findings; pre-existing advisories noted)

### Phase 5.5: Release Safety (Simplified — desktop app)

- Rollback: `git revert` the commit; redeploy the previous
  `clockwork-orange.service` to `~/.config/systemd/user/` and
  `systemctl --user restart`. No data migration.
- Additive/behavior-preserving: a settled config change still triggers one
  switch; only the rapid-burst case changes (now coalesced).
- Deploy note: repo `clockwork-orange.service` is copied verbatim by
  `_service_install_linux`; running unit must be redeployed + service restarted
  for both fixes to take effect.
- Status: ✓ PASSED
