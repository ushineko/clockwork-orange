# Spec 024: Drag an image out of the review pane

**Issue**: #40

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Low
- **Branch**: `feat/review-drag-out`

## Executive Summary

Dragging the image in a plugin's Review tab now drags the file out of the
window as a copy, through fynedesygn 0.1.84's `dragout` (Wayland and X11).
A failure to start shows a banner; Windows and macOS do nothing, as before.
Reviewers should look at `reviewModel.widget` and `dragPaths` in
`internal/gui/review.go`.

## Context

The Review tab of a plugin section shows one image at a time. A user who
wants that image somewhere else has to find it in a file manager by the name
in the info line. Dragging the image into a file manager, a browser upload
field or a chat window is the expected gesture, and it does nothing today:
Fyne cannot start a drag out of its window.

fynedesygn 0.1.84 adds `dragout` (fynedesygn spec 055, ushineko/fynedesygn#166),
which starts the platform's own drag on Wayland and X11. Windows and macOS
report it unsupported until that spec's phase 2.

This is a GUI gesture, not an operation: it reads a path the review already
shows and hands it to the platform. Nothing lands in `internal/core` and the
CLI has no counterpart, so the parity rule does not apply. It is new
behaviour with no 2.9.5 equivalent, so the spec 010 port table is unaffected.

## Requirements

- R1 The review preview is wrapped in `dragout.New`. Its paths function
  returns the image on screen (`reviewModel.current`), or nothing when the
  review is empty or shows an error.
- R2 The file is offered as a copy. The image in the plugin directory is not
  touched by clockwork-orange.
- R3 On X11 a drop target may move the file anyway (fynedesygn quirk 44). The
  directory watcher already rescans on any change, so the review drops the
  missing image after its 500 ms debounce. Nothing more is added for it.
- R4 A drag that fails to start is reported through the shell's banner
  (`Shell.Report`). `dragout.ErrUnsupported` is not reported: on Windows and
  macOS a drag that does nothing is the same as today.
- R5 Arrow keys, Space and taps on the preview behave as before.
- R6 fynedesygn goes to v0.1.84.

## Acceptance Criteria

- [x] R1 Headless test: the review's drag source offers the current image,
      and offers the new one after →.
- [x] R1 Headless test: an empty review offers nothing.
- [x] R4 Headless test: `ErrUnsupported` raises no banner.
- [x] R5 Existing review tests pass unchanged.
- [x] R2 Manual, Plasma 6 Wayland: drag the reviewed image into Dolphin; the
      file is copied and still in the plugin directory.
- [x] R3 Manual, `FYNE_PLATFORM=x11`: drop into Dolphin and choose Move; the
      review shows the next image and does not error. (XWayland, not a
      native X11 session.)
- [x] R6 `go.mod` requires fynedesygn v0.1.84; `make test` and `make lint`
      pass.

## Risks & Assumptions

- Additive: one wrapper around the preview. Rollback: revert the commit, or
  reinstall the previous release.
- The fynedesygn bump also carries any other change since 0.1.83. 0.1.84 has
  only spec 055.

## E2E Test Plan

- Environment: `make build-gui` on KDE Plasma 6 Wayland, a plugin with
  downloaded images, Review tab.
- Steps: drag the shown image into a Dolphin folder; press → and drag again;
  restart with `FYNE_PLATFORM=x11`, drag into Dolphin and choose Move.
- Expected: two copies in the target folder, originals unchanged
  (`sha256sum`); after the Move the review advances to the next image within
  about a second with no error banner. Covers R2 and R3.
