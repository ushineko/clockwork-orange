## Validation Report: Spec 018 — A memory ceiling and smaller previews

**Date**: 2026-09-19 01:30
**Spec**: specs/018-a-memory-ceiling-and-smaller-previews.md
**Issue**: #18
**Status**: PASSED

### Summary

The first look at this window's memory with a profile rather than a reading.
Spec 017 shipped the endpoint in v4.2.4; this is what it was for.

Measured on the affected host after a session of review and download:

    HeapAlloc     228 MB     live heap, after a forced GC
    HeapReleased  455 MB     already handed back to the OS
    Sys          1760 MB     arena grown during the session
    TotalAlloc  17738 MB     allocated over the session
    VmRSS        1590 MB
    VmHWM        1888 MB

`go tool pprof -top -inuse_space` after `?gc=1`:

          flat  flat%
         176MB 78.14%   image.NewRGBA
                          ← 100% from internal/gui.fitPreview

**There is no leak.** Three things were being read as one: the preview caches
are the live heap (176 MB of 228 MB, bounded and working as designed); the
high-water mark is churn, because Go grows its arena to hold a burst and does
not return it; and resident memory overstates both, because `MADV_FREE` leaves
released pages counted until the kernel wants them.

Two changes, both sized from the profile rather than guessed:

- `debug.SetMemoryLimit(768 MiB)` before anything allocates, so the collector
  runs during a burst instead of the arena growing to fit it. Overridable with
  `CLOCKWORK_MEMLIMIT`, and `GOMEMLIMIT` wins outright.
- The preview box drops to 1200×675 and the cap to twelve: **176 MB → 74 MB**
  for the two caches, still over a pixel per pixel for the pane at 1.5×.

### Phase 3: Tests

- `make test`: all packages ok, 0 failing.
- Five tests on the ceiling: the default, deference to `GOMEMLIMIT`, the
  override, the off switch, and an unreadable value leaving the runtime alone.
- `TestTheTwoPreviewCachesFitTheCeiling` pins the cache arithmetic against the
  ceiling, so a later change to the geometry has to look at the number again.
- The memory-limit tests restore the process-wide limit in `t.Cleanup`:
  `SetMemoryLimit` is global and outlives the test that set it, and a test that
  left it set would change how every later test collects.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- `memlimit.go` is one function and two constants. The reasoning — why soft,
  why 768 MiB, why `GOMEMLIMIT` wins — is in the file rather than only here.
- `preview.go`'s geometry comment now carries the measurement that chose the
  numbers, so the next person to change them knows what they are trading.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- No new I/O, no network, no external command, no file access. Two environment
  variables are read; a malformed value is ignored rather than acted on.
- A memory limit is a denial-of-service surface in principle — a very low one
  would make the program collect continuously. It is soft, so Go exceeds it
  rather than failing allocations, and the only way to set a bad one is to set
  it yourself.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit; v4.2.4 is installable from the AUR and the
  GitHub artifacts. Both changes are constants and one runtime call.
- No on-disk change, no format change, no migration.

### Not verified

The effect on the high-water mark under a real session. The arithmetic on the
caches is certain — 176 MB to 74 MB — but what the ceiling does to a burst is
an empirical question, and the answer is a re-profile on the affected host
after this ships. Issue #18 stays open until then.

### Overall

**PASSED.**
