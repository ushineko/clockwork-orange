# Spec 015: The unit names the daemon, not the window

> **Note**: This work has no associated issue tracker ticket (personal public
> GitHub repository, no tracker). Consider creating a GitHub issue for
> traceability.

## Status: COMPLETE

- **Priority**: High — the window cannot install a working service, and has not
  been able to since v4.0.0
- **Estimated Complexity**: Low
- **Branch**: `fix/gui-service-install`

## Executive Summary

The window could not install a working service, and had not been able to since
v4.0.0. `installedUnit` took the unit's `ExecStart` from `os.Executable()`,
which from the window is `clockwork-orange-gui` — a binary that does not take
`--service`. systemd started it, it exited 2 on the unknown flag, and
`Restart=always` with `RestartSec=10` and no start limits restarted it every
ten seconds forever. The window's five-second status poll rebuilt the Service
section on every change, so the symptom was a flickering window and a service
that would not come up.

`platform.DaemonExecutable` now resolves the daemon — sibling, `$PATH`,
packaged path — and the unit names it whichever front end installs it. The
window explains itself if it is ever handed `--service` again, and the shipped
unit gives up after five failures in two minutes instead of flapping forever.

Reviewers should look at `daemonFor` and `daemonBeside` in
`internal/platform/service.go`; everything else follows from them.

## Context

`service install` from the window writes a unit that cannot start, and the
failure is loud, permanent and hard to read.

`installedUnit` takes the unit's `ExecStart` from `os.Executable()`. From the
command line that is `/usr/bin/clockwork-orange`, which is right. From the
window it is `/usr/bin/clockwork-orange-gui`, which is not the daemon: it is a
separate binary with four flags of its own and no `--service` among them, so
systemd starts it, Go's `flag` package reports `flag provided but not defined:
-service`, prints a usage block into the journal and exits 2.

The unit carries `Restart=always` and `RestartSec=10`, and nothing else.
systemd's default start limit is five starts in ten seconds, and restarts ten
seconds apart never reach it, so the unit is started again every ten seconds
for as long as it stays enabled. It never lands in `failed`; it flaps.

The window then makes it visible. The status poll runs every five seconds and
rebuilds the Service section whenever the state or the detail text has changed,
and for a flapping unit both change on every poll — so the section redraws
continuously, and `announceService` sends a desktop notification on each
transition. What the user sees is a window that flickers and a service that
will not come up, with the Install button that would fix it sitting in the
section doing the flickering.

This is not a v4.2.0 regression. It has been true since the Go window shipped
in v4.0.0; it surfaces now because a machine upgraded from the Python line has
a stale unit to repair, and repairing it from the window is the obvious move.

Reported from a real upgrade: the command line's `service install` fixed the
machine, the window's did not.

## Requirements

### R1. The unit names the daemon

- R1.1 `platform.DaemonExecutable` is the binary a unit runs: the running one
  when the daemon is running, and the daemon beside it when the window is.
- R1.2 The window's binary is the daemon's name plus `-gui`, and the two are
  installed together — by the packages into `/usr/bin`, by `install.sh` into
  `~/.local/bin` — so the sibling is the first place to look, then `$PATH`,
  then `/usr/bin/clockwork-orange` as the last resort.
- R1.3 Only the basename counts. `/opt/clockwork-orange-gui/bin/clockwork-orange`
  is the daemon, not the window, and a Windows `.exe` keeps its extension.
- R1.4 `installedUnit` goes through it, so `service install` writes the same
  unit whichever front end runs it.

### R2. The window says why it cannot run the service

- R2.1 `clockwork-orange-gui --service` prints what is wrong and what to do
  about it, and exits 2. Only systemd passes that flag, and only from a unit
  written by a window before this fix, so the message names the repair:
  `clockwork-orange service install`.
- R2.2 It is checked before `flag.Parse`, so the usage block does not bury it.

### R3. A unit that cannot start gives up

- R3.1 The shipped unit carries `StartLimitIntervalSec=120` and
  `StartLimitBurst=5` in `[Unit]`. With `RestartSec=10`, five failures in two
  minutes is a unit that is not going to start, and `failed` is a state the
  user can see and the Service section reports.
- R3.2 This bounds the blast radius of any unstartable unit, not only this
  one's cause.

### R4. Guards

- R4.1 A test pins that the daemon resolved for a window binary is the daemon,
  for `/usr/bin`, for `~/.local/bin`, for a Windows path, and for the three
  cases that are not the window at all.
- R4.2 A test pins that `DaemonExecutable` never answers with a window binary,
  whatever the fallbacks do — including with an empty `$PATH`.
- R4.3 A test pins the unit's start limits and that they are `[Unit]`
  directives, which is where systemd reads them and not where `Restart=` and
  `RestartSec=` live.

### R5. Release

- R5.1 v4.2.2, a patch.

## Acceptance Criteria

- [x] AC1 A unit installed from the window names the daemon beside it (R1.1, R1.2)
  - Verified: `TestTheWindowInstallsAUnitForTheDaemonBesideIt` — the sibling daemon, and the ExecStart built from it
- [x] AC2 A `-gui` directory in a parent path is not mistaken for the window, and a Windows `.exe` keeps its extension (R1.3)
  - Verified: `TestTheDaemonBesideTheWindowIsTheDaemon` — seven cases including `/opt/clockwork-orange-gui/bin/clockwork-orange` and a Windows `.exe`
- [x] AC3 With no sibling and an empty `$PATH`, the answer is the packaged daemon and still not the window (R1.2, R4.2)
  - Verified: `TestTheWindowInstallsAUnitForTheDaemonBesideIt` with `PATH` emptied, and `TestDaemonExecutableIsNeverTheWindow`
- [x] AC4 `service install` writes the same unit from the command line and from the window (R1.4)
  - Verified: `installedUnit` calls `DaemonExecutable`; `TestInstallWritesEmbeddedUnitUnderXDGConfigHomeThenReloadsAndEnables` still pins what Install writes
- [x] AC5 `clockwork-orange-gui --service` explains itself and exits 2 (R2)
  - Verified: `refuseServiceFlag` in `cmd/clockwork-orange-gui/main.go`, checked before `flag.Parse`. Verified by hand before the fix: `clockwork-orange-gui --service` printed `flag provided but not defined: -service` and a usage block
- [x] AC6 The shipped unit carries the start limits, in `[Unit]` (R3.1, R4.3)
  - Verified: `TestTheUnitBoundsItsRestarts`, which also pins that the directives precede `[Service]`
- [x] AC7 `make test` passes
  - Verified: `make test` green (race detector, `-tags parity`)
- [x] AC8 `make lint` is clean
  - Verified: `make lint` — 0 issues
- [x] AC9 `govulncheck ./...` is clean
  - Verified: `govulncheck ./...` — no vulnerabilities found
- [x] AC10 Released as v4.2.2 (R5.1)
  - Verified: see the validation report

## Risks & Assumptions

- **Rollback**: revert the commit; v4.2.1 is installable from the AUR and the
  GitHub artifacts. Rolling back restores the bug, so the rollback is forward.
- **Existing broken units are not repaired automatically.** A unit already on
  disk keeps its wrong `ExecStart` until something rewrites it. `service
  install` from either front end now does, and that is the documented repair;
  the window's Install button is the same call, so pressing it once is enough.
- **The packaged fallback can be wrong.** On a machine with the window in
  `~/.local/bin`, no daemon beside it and nothing on `$PATH`, the unit names
  `/usr/bin/clockwork-orange`, which may not exist. That unit fails to start —
  but it now fails five times and stops (R3), and it names a binary whose
  absence is obvious, rather than one that exists and cannot do the job.
- **The start limits change a shipped on-disk artifact.** Users keep the unit
  they have until a reinstall. The directives are systemd 229 and newer; Plasma
  6 implies far newer.
- **Not covered**: whether the Service section should notice that the installed
  unit's `ExecStart` disagrees with this binary and say so. That would have
  turned this into a visible message rather than a flicker. Recorded below.

## Gaps found

- **Nothing tells the user their unit is stale.** A unit in
  `~/.config/systemd/user/` outranks the packaged one permanently, so a machine
  upgraded from the Python line keeps whatever it had, and neither
  `--self-test`, nor `service status`, nor the Service section compares the
  installed `ExecStart` with the binary that would install it. Both this bug
  and the original upgrade complaint would have been one line of output instead
  of an investigation. Worth its own spec: a check in `--self-test` and a
  banner in the Service section.
- **The Service section rebuilds on every status poll when the detail text
  moves.** `pollService` compares `Details` verbatim, and systemd's detail text
  carries timestamps, PIDs and an invocation ID, so for any unit that is
  changing at all the comparison always differs and the whole section is
  rebuilt every five seconds. With the flapping fixed this is no longer
  user-visible, but the redraw ladder is doing the most expensive thing
  available for a label that moved. Worth its own spec.

## Alternatives Considered

- **Teach the window to run the service.** Rejected: the two binaries are
  separate precisely because the daemon must not need CGO, OpenGL or a display.
- **Have the window refuse to install the service and point at the CLI.**
  Rejected: installing the service is a thing the window is supposed to do, and
  the CLI/GUI parity rule says every operation is reachable from both.
- **Write the packaged path always.** Rejected: `install.sh` puts both binaries
  in `~/.local/bin`, and a unit naming `/usr/bin` would fail on exactly the
  machine that just installed from source — which is the bug the original
  `UnitFileFor` was written to avoid.
