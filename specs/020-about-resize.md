# Spec 020: The About section resizes smoothly

**Issue**: #26

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Low
- **Branch**: `fix/about-resize`

## Executive Summary

Dragging the window edge with About showing was jerky, and on an older build
it hung. A CPU profile taken through spec 017's endpoint put a third of all
CPU in the markdown pane re-measuring: `measure` renders every block to ask
its height, and Fyne hands a widget a Resize for every step of a drag.

fynedesygn 0.1.29 adds `markdown.Options.SettleResize`, which waits for the
width to stop changing and measures once. This bumps the library and sets it.

## Context

Twenty seconds of CPU on v4.3.0, while the window was being dragged:

    Duration: 20s, Total samples = 7590ms (37.95%)

                                cum     cum%
    markdown.(*Pane).Resize    3.18s   41.90%
    markdown.(*Pane).measure   2.40s   31.62%
    glfw.MakeContextCurrent    1.62s   28.32%

The call arrives as `glfwPollEvents → cgocallback → processResized →
RunWithContext`: Fyne performs the whole relayout **synchronously inside the
event poll**. The event queue cannot drain while that runs, which is why the
drag is jerky rather than merely slow. The other two thirds are that path and
are not ours.

Worth recording: the machine that reported this was running **v4.2.0**, five
releases behind, from before the memory ceiling (018) and the preview-cache
shrink. Updating to v4.3.0 turned the hang into jerkiness on its own. The first
measurement in any report like this should be which build is running.

## Requirements

### R1. The library bump

- R1.1 `go.mod` requires `github.com/ushineko/fynedesygn v0.1.29`, up from
  v0.1.25.
- R1.2 That carries three releases this program had not taken: 0.1.26 (the
  affixed-controls rule and `fynetest.Scrolled`, whose policy this program
  already implements in `AffixedActions`), and 0.1.27/0.1.28 (the `glance`
  window archetype, which this program does not use).
- R1.3 0.1.26 changed `fynetest.Walk` to descend a `container.Split`. This
  program walks with its own helper in `gui_test.go`, which already had that
  case, so nothing here depends on the old behaviour.

### R2. The About pane settles

- R2.1 The README pane is built with `SettleResize: 120ms`.
- R2.2 120ms is long enough to swallow a drag and short enough that letting go
  of the edge reflows the document while the hand is still on the mouse.
- R2.3 Nothing else changes: the pane, the scroller it follows and the
  section's layout are untouched.

## Acceptance Criteria

- [x] AC1 `go.mod` is at fynedesygn v0.1.29 (R1.1)
  - Verified: `go.mod`, `go mod tidy` clean
- [x] AC2 The About pane sets a settle (R2.1)
  - Verified: `views_about.go` — `markdown.Options{SettleResize: readmeSettle}`
- [x] AC3 The three intervening releases break nothing (R1.2, R1.3)
  - Verified: `make test` green, including `tests/parity` and the whole GUI suite, with no test changed
- [x] AC4 `make test` passes
  - Verified: all packages ok
- [x] AC5 `make lint` is clean
  - Verified: 0 issues
- [x] AC6 `govulncheck ./...` is clean
  - Verified: no vulnerabilities found

## Risks & Assumptions

- **Rollback**: revert the commit; v4.3.0 is installable from the AUR and the
  GitHub artifacts.
- **The document is briefly laid out for the old width.** While a drag is in
  progress the heights are the ones from before it, corrected when the drag
  stops. A smaller wrongness than the one it replaces.
- **Not measured after the change.** The mechanism is pinned by the library's
  tests and the profile says what share it was, but a second profile during a
  real drag is what would show the improvement. That is the next step on #26.
- **Two thirds of the cost remain.** `MakeContextCurrent` and Fyne's
  synchronous relayout inside `processResized` are the driver's design.

## Gaps found

- **`Visual(i)` renders a block and keeps it forever**, so measuring the
  document renders all of it: the pane's virtualisation applies to the widget
  tree, not to what has been built. Recorded against the library
  (fynedesygn spec 015).
- Carried and still open: the Service section rebuilding on a moving timestamp
  (015), and nothing telling the user their unit is stale (015).
