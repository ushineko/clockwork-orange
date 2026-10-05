## Validation Report: spec 024, drag an image out of the review pane

**Date**: 2026-10-04
**Spec**: `specs/024-drag-an-image-out-of-review.md` (#40)
**Status**: PASSED

### Summary

The review preview is wrapped in fynedesygn's `dragout.Source`, which offers
the image on screen as a file copy. fynedesygn goes from v0.1.83 to v0.1.84,
whose only change is the `dragout` package (fynedesygn spec 055 phase 1).

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages ok, 0 failing.
- New `TestReviewDragOffersTheImageOnScreen`: the offered path follows →, and
  an empty or missing directory offers nothing.
- New `TestReviewDragReportsOnlyRealFailures`: `dragout.ErrUnsupported` raises
  no banner; another error does.
- Existing review tests pass unchanged (R5).
- Manual, KDE Plasma 6 Wayland: drag into Dolphin copies the file and leaves
  the original; after → the next image is dragged. With `FYNE_PLATFORM=x11`
  (XWayland), choosing Move in Dolphin moves the file and the review advances
  without an error.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- Change: `go.mod`, `go.sum`, about 25 lines in `review.go`, two tests. No dead
  code. The platform code lives in fynedesygn, not here.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- The drag offers only the path the review already shows, which comes from
  scanning the plugin's directory. No new network path, external command or
  credential handling. No secrets in the diff.
- New native code (Wayland and X11 C) arrives through the fynedesygn bump; it
  was reviewed in fynedesygn#167.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit, or reinstall the previous release. No
  on-disk format change.

### Not verified

- A native X11 session (tested under XWayland only).
- Windows and macOS by hand; there `dragout` reports unsupported, and CI
  covers the builds.

### Overall

**PASSED.**
