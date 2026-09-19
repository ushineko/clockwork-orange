## Validation Report: Spec 017 — Opt-in profiling

**Date**: 2026-09-19 00:30
**Spec**: specs/017-opt-in-profiling.md
**Status**: PASSED

### Summary

A diagnostic, not a fix. The window's memory climbs during a review or a
download; measured on the affected host at v4.2.2, `VmHWM` 1.65 GB draining to
`VmRSS` 495 MB — a burst, not a leak. Spec 016 fixed one cause found by reading
the code, and a further report says it is not enough.

The code has now been read three times about this window's resource use, and
one of those readings was wrong in a way that cost real time: the flicker
attributed to KWin greying an unresponsive window turned out to be a compositor
effect with a bug in its window-matching path. `CLOCKWORK_PPROF=:6060` replaces
the fourth reading with a heap profile.

### Phase 3: Tests

- `make test`: all packages ok, 0 failing.
- `TestPprofIsOffUnlessTheEnvironmentAsks` — unset means nothing served and
  nothing said, and the stopper is safe to call regardless.
- `TestPprofBindsToLoopbackWhateverItIsAsked` — `:6060`, `0.0.0.0:6060`,
  `6060`, `[::]:6060` and `192.168.1.5:6060` all resolve to `127.0.0.1:6060`.
- `TestPprofServesAndStops` — the heap profile is served, and after the stopper
  runs the listener refuses.

### Phase 4: Code Quality

- `make lint`: **0 issues**. Two findings on the first pass, both fixed rather
  than suppressed: `net.Listen` became `(*net.ListenConfig).Listen` with a
  five-second context, and the test's second response body is closed.
- The handlers are registered on a mux of this program's own rather than on
  `DefaultServeMux` through the package's import side effect, so nothing else
  in the process can serve profiles by accident.

### Phase 5: Security Review

This adds a network listener to a shipped binary, so the review is the point
rather than a formality.

- **Bound to loopback whatever it is asked for.** `:6060` means every interface
  to `net.Listen`; `loopback()` discards the host and keeps the port. Five
  spellings are pinned by test, including `0.0.0.0` and `[::]`.
- **Off by default.** No variable, no listener, no goroutine, no port.
- **What it exposes**: heap, goroutine, cmdline, CPU profile and trace. That is
  the program's memory and argv — which is why it is loopback-only and opt-in.
  Reaching it requires an account on the machine.
- **Timeouts**: read-header 10 s, write 2 min (a CPU profile legitimately takes
  30 s), listen 5 s. A stuck client cannot hold the listener.
- **The daemon is untouched**: this is `internal/gui`, and the CGO-free
  `clockwork-orange` gains nothing.
- `govulncheck ./...`: **no vulnerabilities found**.

### Phase 5.5: Release Safety

Rollback: revert the commit. With the variable unset the code is inert, so
leaving it in is equally safe. No on-disk change, no format change.

### Overall

**PASSED.** The memory question stays open until a profile is taken on the
affected host during a real review and download; this is what makes that
possible.
