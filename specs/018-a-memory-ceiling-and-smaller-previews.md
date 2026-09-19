# Spec 018: A memory ceiling and smaller previews


**Issue**: #18

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Low
- **Branch**: `fix/memory-ceiling`

## Executive Summary

The window's memory was reported as climbing during review and download. A heap
profile taken with spec 017's endpoint, on the affected host after a session of
both, says there is no leak: live heap after a forced collection is 228 MB, of
which 176 MB is the two preview caches.

What produced the 1.89 GB high-water mark is churn — 17.7 GB allocated over the
session, because every 4K image is decoded at full size and discarded — and
Go's arena, which grows to hold a burst and does not give it back. Resident
memory then overstates even that, because Go releases with `MADV_FREE` and the
pages stay counted until the kernel wants them.

Two changes, both sized from the profile. `GOMEMLIMIT` at 768 MiB makes the
collector run during a burst rather than let the arena grow to fit it. The
preview box drops from 1600×900 to 1200×675 and the cache from sixteen frames
to twelve, which takes the two caches from 176 MB to 74 MB.

## Context

Measured on the affected host at v4.2.4, after a review of a 499-image
directory and several downloads:

    HeapAlloc     228 MB     live heap, after a forced GC
    HeapReleased  455 MB     already handed back to the OS
    Sys          1760 MB     arena grown during the session
    TotalAlloc  17738 MB     allocated over the session
    VmRSS        1590 MB
    VmHWM        1888 MB

`go tool pprof -top -inuse_space`, after `?gc=1`:

          flat  flat%
         176MB 78.14%   image.NewRGBA
                          ← 100% from internal/gui.fitPreview

Three separate things were being read as one:

1. **The caches are the live heap.** 1600×900 RGBA is 5.76 MB a frame; the
   review holds sixteen and the run dialog holds sixteen more, so a session
   that used both held 176 MB in thumbnails for panes a few hundred pixels
   across. Bounded, working as designed, and much larger than it needs to be.
2. **The high-water mark is churn.** Go grows its heap to about twice the live
   set at the last collection and does not return the arena. A review of 4K
   wallpapers allocates 33 MB per image and discards it; 420 million mallocs
   later the arena is 1.9 GB and stays there.
3. **Resident memory overstates both.** Go releases with `MADV_FREE`, so
   released pages remain in RSS until the kernel reclaims them. 455 MB of the
   1.59 GB was already given back.

This is the fourth look at this window's memory and the first with a profile.
The previous three were readings of the code; one of them was wrong. Spec 017
exists because of that, and this spec is what it was for.

## Requirements

### R1. A soft ceiling

- R1.1 The window sets `debug.SetMemoryLimit` to 768 MiB before anything
  allocates.
- R1.2 Soft, not a cap: Go exceeds it rather than failing an allocation, so a
  machine doing something the number did not anticipate degrades into more GC
  and not a crash.
- R1.3 768 MiB is room for two full preview caches (74 MB), the font tables
  (~25 MB), a decode in flight and the rest of the window, several times over,
  and far below the 1.9 GB a burst reached without it.

### R2. The ceiling is overridable

- R2.1 `GOMEMLIMIT` wins outright: it is the runtime's own switch, read before
  this program runs, and overriding it from inside would make the documented
  variable a lie.
- R2.2 `CLOCKWORK_MEMLIMIT` sets it in bytes; `0` or a negative value turns it
  off and leaves the runtime's own behaviour.
- R2.3 An unreadable value leaves the runtime alone rather than guessing. A
  typo in a desktop entry should not silently change how the window collects.

### R3. Smaller previews

- R3.1 The preview box is 1200×675 rather than 1600×900, and the cap twelve
  rather than sixteen: 74 MB for both caches rather than 176 MB.
- R3.2 Still over a pixel per pixel for the pane at a 1.5× desktop scale, which
  is what the geometry was for.
- R3.3 The prefetcher still reaches one neighbour each way, so the cache covers
  a run of arrow-key presses; it was never meant to cover a directory.

### R4. Guards

- R4.1 Tests pin the default, the `GOMEMLIMIT` deference, the override, the
  off switch and the unreadable value.
- R4.2 A test pins the cache arithmetic against the ceiling, so a later change
  to the geometry has to look at the number again.
- R4.3 The memory-limit tests restore the process-wide limit, because
  `SetMemoryLimit` outlives the test that set it.

## Acceptance Criteria

- [x] AC1 The window runs under a 768 MiB soft ceiling by default (R1.1)
  - Verified: `TestTheMemoryCeilingDefaults`
- [x] AC2 `GOMEMLIMIT` is deferred to (R2.1)
  - Verified: `TestTheRuntimesOwnLimitWins`
- [x] AC3 `CLOCKWORK_MEMLIMIT` overrides, and `0`/`-1` turn it off (R2.2)
  - Verified: `TestTheCeilingCanBeSetOrTurnedOff`
- [x] AC4 An unreadable value changes nothing (R2.3)
  - Verified: `TestAnUnreadableCeilingIsIgnored`
- [x] AC5 The preview box is 1200×675 and the cap twelve (R3.1)
  - Verified: `preview.go`; two full caches are 74 MB against the previous 176 MB
- [x] AC6 The cache arithmetic is pinned against the ceiling (R4.2)
  - Verified: `TestTheTwoPreviewCachesFitTheCeiling`
- [x] AC7 The tests restore the process-wide limit (R4.3)
  - Verified: `restoreMemLimit`, a `t.Cleanup` on every one of them
- [x] AC8 `make test` passes
  - Verified: all packages ok
- [x] AC9 `make lint` is clean
  - Verified: 0 issues
- [x] AC10 `govulncheck ./...` is clean
  - Verified: no vulnerabilities found

## Risks & Assumptions

- **Rollback**: revert the commit; v4.2.4 is installable from the AUR and the
  GitHub artifacts. Both changes are constants and a one-line runtime call.
- **A ceiling trades CPU for memory.** Under it the collector runs more often
  during a download. 768 MiB is three times the measured live heap, so a
  session like the one profiled should not approach it; a much larger
  directory might, and would pay in GC rather than in RSS.
- **Smaller previews are a visible change, in principle.** 1200×675 is still
  above the pane's pixel count at 1.5×, so it should not be visible in
  practice. If it is, `previewMaxW`/`previewMaxH` are one line.
- **Not yet measured after the change.** The arithmetic is certain; the effect
  on the high-water mark under a real session is not, and is the reason to
  re-profile on the affected host once this ships.
- **Resident memory will still look high.** `MADV_FREE` is the runtime's
  behaviour, not this program's. `GODEBUG=madvdontneed=1` makes RSS track
  reality promptly, at a cost, and is a thing to reach for when reading the
  number rather than a default to ship.

## Gaps found

- **The full-size decode remains.** Every 4K JPEG is decoded whole (~33 MB)
  before being scaled, which is what generates the churn a ceiling now absorbs
  rather than prevents. Go's `image/jpeg` has no scaled decode, so fixing it
  means a different decoder — deliberately out of scope here, and the thing to
  do if the numbers still disappoint after this.
- Carried and still open: the Service section rebuilding on a moving timestamp
  (015), and nothing telling the user their unit is stale (015).

## Alternatives Considered

- **Ship `GODEBUG=madvdontneed=1`.** Rejected: it makes the number smaller
  without making the program use less, and costs page faults on every reuse.
- **Drop the run dialog's cache and share the review's.** Rejected in spec 016
  and still rejected: a shared cache keeps a finished run's frames alive behind
  whatever the user looks at next.
- **A much lower ceiling, 256 MiB.** Rejected: live heap is 228 MB, so the
  collector would run continuously for no benefit.
