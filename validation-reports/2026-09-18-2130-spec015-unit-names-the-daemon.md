## Validation Report: Spec 015 — The unit names the daemon, not the window

**Date**: 2026-09-18
**Spec**: specs/015-the-unit-names-the-daemon.md
**Branch**: `fix/gui-service-install`, off `main` at 263605e
**Status**: PASSED

### Summary

Reported from a real upgrade: the command line's `service install` repaired the
machine, the window's did not, and attempting it from the window left the
interface flickering.

`installedUnit` took the unit's `ExecStart` from `os.Executable()`. From the
command line that is `/usr/bin/clockwork-orange`. From the window it is
`/usr/bin/clockwork-orange-gui`, a separate binary with four flags of its own
and no `--service` among them. Confirmed by running it:

    $ clockwork-orange-gui --service
    flag provided but not defined: -service
    Usage of clockwork-orange-gui:
    ...
    exit 2

The unit carried `Restart=always` and `RestartSec=10` and no start limits.
systemd's default is five starts in ten seconds; restarts ten seconds apart
never reach it, so the unit restarted every ten seconds for as long as it was
enabled and never landed in `failed`. The window's five-second poll rebuilds
the Service section whenever the state or the detail text changed — both change
on every poll for a flapping unit — so the section redrew continuously and
`announceService` sent a notification on each transition.

Not a v4.2.0 regression: true since the Go window shipped in v4.0.0. It
surfaced now because a machine upgraded from the Python line has a stale unit
to repair, and repairing it from the window is the obvious move.

### Changes

- `platform.DaemonExecutable` / `daemonFor` / `daemonBeside`: the unit names the
  daemon — the running binary when the daemon runs it, else the sibling beside
  the window, else `$PATH`, else the packaged `/usr/bin/clockwork-orange`.
  `installedUnit` goes through it, so both front ends write the same unit.
- `clockwork-orange-gui --service` explains what is wrong and names the repair,
  before `flag.Parse` so the usage block does not bury it.
- The shipped unit gains `StartLimitIntervalSec=120` and `StartLimitBurst=5` in
  `[Unit]`, so any unstartable unit gives up after five failures in two minutes
  instead of flapping forever.

### Phase 3: Tests

- `make test`: all packages ok, 0 failing.
- `TestTheDaemonBesideTheWindowIsTheDaemon` — seven cases, including a Windows
  `.exe` and `/opt/clockwork-orange-gui/bin/clockwork-orange`, which is the
  daemon despite the parent directory's name.
- `TestTheWindowInstallsAUnitForTheDaemonBesideIt` — the sibling, and the
  fallback with `$PATH` emptied.
- `TestDaemonExecutableIsNeverTheWindow` — the property the unit depends on,
  stated once and independently of how it is reached.
- `TestTheUnitBoundsItsRestarts` — the start limits, and that they precede
  `[Service]`, because `StartLimit*` are `[Unit]` directives and systemd
  silently ignores them in the wrong section.
- `daemonFor` was split out of `DaemonExecutable` so the fallbacks could be
  tested against a directory the test owns rather than against whatever the
  test binary happens to sit beside.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- `daemonBeside` is pure and separately tested; that is where the decision is.
- No duplication: one resolution used by `installedUnit` and available to
  callers that need to name the daemon.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- `exec.LookPath` is new. It reads `$PATH` for a fixed, hard-coded basename and
  never runs what it finds; the result goes into a unit file the user's own
  systemd starts. No user input reaches it. The fallback order prefers paths
  derived from the running binary over `$PATH`, so a poisoned `$PATH` is the
  third choice rather than the first.
- No credentials, no network, no new external command invocation.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit; v4.2.1 is installable from the AUR and the
  GitHub artifacts. Rolling back restores the bug, so the rollback is forward.
- **Existing units are not repaired automatically** — a unit on disk keeps its
  `ExecStart` until something rewrites it. `service install` from either front
  end now does, which is the documented repair.
- The start limits change a shipped on-disk artifact; users keep the unit they
  have until a reinstall.

### Not verified on this host

That a unit installed from the window starts. This machine's unit is a symlink
into the author's dotfiles and is deliberately not managed by `service
install`; overwriting it to test would replace a hand-maintained file. The unit
*contents* are pinned by tests, and the resolution that produced them is pinned
independently.

### Overall

**PASSED.** Released as v4.2.2, a patch.
