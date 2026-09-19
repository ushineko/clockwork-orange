# Spec 017: Opt-in profiling

> **Note**: This work has no associated issue tracker ticket (personal public
> GitHub repository, no tracker). Consider creating a GitHub issue for
> traceability.

## Status: COMPLETE

- **Priority**: Medium — a diagnostic, for an open memory question
- **Estimated Complexity**: Low
- **Branch**: `feat/pprof`

## Executive Summary

The window's memory climbs during a review or a download: a measured peak of
1.65 GB on one host, draining back to 495 MB, which is a burst rather than a
leak. Spec 016 fixed one cause found by reading the code. A second report says
it is not enough, and the code has now been read twice with one wrong answer
between the readings — the flicker attributed to an unresponsive window turned
out to be a compositor effect.

So: a heap profile instead of another reading. `CLOCKWORK_PPROF=:6060` starts
`net/http/pprof` on the loopback interface; without it nothing is served and
nothing is said.

## Context

Measured on the affected host, running v4.2.2, after a review and a download:

    VmHWM:  1653000 kB     (peak)
    VmRSS:   495476 kB     (at rest)

The in-window structures are all bounded: the review holds paths, a mark map
and the same sixteen-frame `previewCache`; the blacklist's thumbnails come from
the database at thumbnail size. What is not bounded is the churn — each 4K JPEG
is decoded at full size, about 33 MB, and only then scaled to 1600×900, and the
review scales neighbours ahead of the one on screen.

That is a plausible account of a high-water mark that drains. It is also the
third account produced by reading rather than measuring, and the second was
wrong. The point of this spec is to stop doing that.

## Requirements

### R1. Off by default

- R1.1 Nothing is served and nothing is said unless `CLOCKWORK_PPROF` names an
  address.
- R1.2 The stopper returned is always safe to call, so the caller needs no
  branch.

### R2. Loopback, whatever is asked for

- R2.1 The listener binds to `127.0.0.1` regardless of the host in the
  variable. `:6060` means every interface to `net.Listen`, which is how a
  debugging endpoint ends up reachable from the network.
- R2.2 A bare port is honoured; a host is replaced, not rejected. The variable
  is a convenience, not a security control — the control is that the host is
  ignored.
- R2.3 The handlers are registered on a mux of this program's own, not on
  `DefaultServeMux` via the package's import side effect.

### R3. Never fatal

- R3.1 A server that cannot bind is reported and the window opens anyway.
- R3.2 The report is a banner: a profiling server the user turned on and which
  did not come up is worth saying out loud.
- R3.3 The server is shut down with the rest of the window.

### R4. Guards

- R4.1 A test pins that an unset variable starts nothing and says nothing.
- R4.2 A test pins the loopback rewrite for five spellings, including
  `0.0.0.0:6060` and `[::]:6060`.
- R4.3 A test pins that the heap profile is served and that stopping closes the
  listener.

## Acceptance Criteria

- [x] AC1 Unset `CLOCKWORK_PPROF` starts nothing and reports nothing (R1)
  - Verified: `TestPprofIsOffUnlessTheEnvironmentAsks`
- [x] AC2 Every spelling binds to 127.0.0.1 (R2.1, R2.2)
  - Verified: `TestPprofBindsToLoopbackWhateverItIsAsked` — `:6060`, `0.0.0.0:6060`, `6060`, `[::]:6060`, `192.168.1.5:6060`
- [x] AC3 The handlers are on the program's own mux (R2.3)
  - Verified: `pprof.go` — `http.NewServeMux()` and five explicit `HandleFunc` registrations; `net/http/pprof` is imported for its handlers, not its side effect
- [x] AC4 The heap profile is served, and stopping closes the listener (R4.3)
  - Verified: `TestPprofServesAndStops`
- [x] AC5 A failure to bind does not stop the window (R3.1)
  - Verified: `startPprof` reports and returns a no-op stopper on listen error
- [x] AC6 The server stops with the window (R3.3)
  - Verified: `shutdown` in `timer.go` calls `stopPprof`
- [x] AC7 `make test` passes
  - Verified: all packages ok
- [x] AC8 `make lint` is clean
  - Verified: 0 issues
- [x] AC9 `govulncheck ./...` is clean
  - Verified: no vulnerabilities found

## Risks & Assumptions

- **Rollback**: revert the commit. With the variable unset the code is inert,
  so leaving it in place is also safe.
- **It is a debugging endpoint in a shipped binary.** That is the point of
  R2: the host in the variable is ignored and the listener is loopback, so
  reaching it needs an account on the machine. `pprof.Cmdline` exposes the
  program's own argv and `Profile`/`Trace` are CPU-expensive on demand — both
  are behind the same switch, which defaults to off.
- **The daemon is untouched.** This is in `internal/gui`; `CGO_ENABLED=0
  clockwork-orange` gains nothing and serves nothing.
- **It diagnoses; it does not fix.** The memory question stays open until a
  profile is taken on the affected host during a real review and download.

## Gaps found

- Carried and still open: the Service section rebuilding on a moving timestamp
  (015), nothing telling the user their unit is stale (015), and nothing
  bounding what the window hands to Fyne (016).

## Alternatives Considered

- **Fix the decode path on inference.** Rejected for now; that is the third
  reading of the same code and the second was wrong. The profile decides.
- **`GOMEMLIMIT`.** A soft ceiling would cap the high-water mark by collecting
  harder. It is a mitigation without a diagnosis, and worth revisiting once the
  profile says where the bytes are.
- **A `--pprof` flag rather than an environment variable.** Rejected: the
  window is usually started from a desktop entry, where an environment variable
  is easier to set for one run than an argument is.
