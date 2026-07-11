## Validation Report: Surface running instance on second launch

**Date**: 2026-07-11
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: none (bug fix — no dedicated spec file)

### Problem

The Windows build appeared "not to launch." Investigation showed the frozen
build is healthy — `--self-test` passes and a clean launch shows the main
window. The real cause is the single-instance guard combined with
minimize-to-tray behaviour:

1. `gui/main_window.py:main()` acquires a named lock
   (`acquire_instance_lock`). When a prior instance already holds it, the code
   printed a debug line and `return 0` — **no window, no message, no attempt to
   surface the existing instance**.
2. `closeEvent` hides the window to the system tray instead of quitting, so a
   running instance is easy to leave in the tray.

Result: with an instance already running (e.g. minimized to tray), every new
launch exited silently and nothing appeared — indistinguishable from "won't
launch" to the user.

The recent DuckDuckGo changes were ruled out: `git show --stat` on `4c8b1bb`
and `2935784` shows they touch only `plugins/duckduckgo_images.py` and its
tests, not the launch path.

### Changes

- Added a Qt local-socket single-instance channel (`QLocalServer`/
  `QLocalSocket`, cross-platform via `PyQt6.QtNetwork`):
  - `_notify_existing_instance()` — a blocked second launch connects to the
    primary instance and sends a short `SHOW` message; returns whether the
    primary was reached.
  - `ClockworkOrangeGUI.start_single_instance_server()` — the primary instance
    listens on a fixed name and, on connection, raises its window.
  - `_on_secondary_instance()` / `_raise_to_front()` — restore from minimized,
    `show()`, `raise_()`, `activateWindow()`.
- `main()` now signals the running instance in the lock-held branch and starts
  the server after the window is created.
- Server restricted to the current user (`SocketOption.UserAccessOption`);
  payload is ignored regardless of content.
- Added `PyQt6.QtNetwork` as an explicit `--hidden-import` in
  `scripts/build_windows.ps1` and `scripts/build_windows_production.ps1`
  (PyInstaller also auto-detects the import; explicit for safety).

### Phase 3: Tests

- New `tests/test_single_instance.py` (2 cases): `_notify_existing_instance`
  returns `False` when no instance is listening (guards the lock-held branch
  from hang/exception); a `QLocalServer` started as the app starts one receives
  the `SHOW` payload. Both pass.
- Full suite: `python -m pytest -q --ignore=scripts` → 24 passed, 0 failures.
  (`scripts/test_watchdog_frozen.py` is a standalone helper, not a pytest test;
  its collection error is pre-existing and unrelated.)
- End-to-end (Windows, from source): launched primary, minimized its window
  (`IsIconic=True`), ran a second launch; the second launch logged
  "Asked the running instance to show its window" and the primary window
  restored (`IsIconic=False, Visible=True`). Verified both before and after the
  `UserAccessOption` hardening.
- `python -c "import ast; ast.parse(...)"` clean on `gui/main_window.py`.
- Status: PASSED

### Phase 4: Code Quality

- Dead code: none introduced.
- Duplication: window-surfacing centralised in `_raise_to_front`, reused by the
  connection handler.
- Encapsulation: additive methods on `ClockworkOrangeGUI` and one module-level
  helper; no signature churn to existing methods. Server name is a single named
  constant (`SINGLE_INSTANCE_SERVER`).
- Status: PASSED

### Phase 5.5: Release Safety (simplified — desktop app)

- Rollback: revert the commit / reinstall previous release. No schema, no
  migration, no persisted-state change.
- Additive change: only adds behaviour to the previously-silent lock-held path;
  the primary launch path is unchanged apart from starting the listener.
- Failure mode is graceful: if the local server cannot listen or cannot be
  reached, behaviour degrades to the prior silent-return (a warning is logged).
- Status: PASSED

### Security (Phase 5)

- No new third-party dependencies (`PyQt6.QtNetwork` ships with the existing
  PyQt6 requirement); no CVE surface added, so `pip-audit` not run this session.
- No secrets added or logged.
- Local IPC surface: the server accepts only local connections and, via
  `UserAccessOption`, restricts the socket to the current user. The received
  payload is discarded; the only effect of a message is raising the app window
  (no code execution, no data exposure). Worst case for a same-user local
  process is a nuisance window-raise at equal privilege.
- Status: PASSED
