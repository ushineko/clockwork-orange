# Spec 016: The run dialog's preview is scaled

> **Note**: This work has no associated issue tracker ticket (personal public
> GitHub repository, no tracker). Consider creating a GitHub issue for
> traceability.

## Status: COMPLETE

- **Priority**: High — the window is unusable during a download of large
  images, and has been since v4.0.0
- **Estimated Complexity**: Low
- **Branch**: `fix/run-dialog-preview`

## Executive Summary

The run dialog decoded every saved image at full size and handed the frame to
`canvas.Image`, so Fyne re-scaled 3840×2160 on every redraw of a dialog that
redraws constantly — the log pane pumps and the progress bar moves for the
whole download. On a host fetching 4K wallpapers the window ran at over 200%
CPU with 1.6 GB resident, and KWin greyed it as unresponsive, which reads as
the window going transparent and flickering.

`internal/gui/preview.go` was written for exactly this problem, for the review
pane, and the run dialog never used it. It now does: each saved image is
decoded once, scaled to preview size and cached, bounded at sixteen frames that
go when the dialog does.

## Context

Reported as "the window blurring effect eats a ton of CPU, makes the window
transparent and flicker — and it does not do that on the other machine".

Measured on the affected host during a Wallhaven run:

    %CPU  RSS       NLWP  ELAPSED  COMMAND
     231  1.6 GB      86    07:04  /usr/bin/clockwork-orange-gui

The blur was a symptom, not the cause. KWin desaturates a window that stops
answering, so a saturated UI thread reads as the compositor doing something to
the window. The other machine was not running a download of 4K images at the
time, which is the whole of the difference between the two hosts.

The cause is in `runEvents`:

    OnImageSaved: func(path string) {
        d.lastSaved = path
        img, _, err := imaging.DecodeFile(path)   // full 3840x2160
        ...
        fyne.Do(func() { d.showPreview(img) })    // straight to canvas.Image
    }

`preview.go`'s own doc comment describes why this is wrong, in the review's
words: *"Decoding one takes a tenth of a second and handing the full 3840×2160
frame to a canvas.Image costs more on every redraw than the decode did, because
Fyne re-scales it to the pane each time."* The review was fixed; the dialog,
which has the same images and a far higher redraw rate, was not.

That host's Wallhaven directory holds 499 files and 3.2 GB, with
`atleast: 3840x2160`. A decoded 4K RGBA frame is about 33 MB.

## Requirements

### R1. The preview goes through the cache

- R1.1 `runDialog` holds a `*previewCache`, created with the dialog.
- R1.2 `OnImageSaved` fetches through it, so each image is decoded once and
  scaled to `previewMaxW`×`previewMaxH` before reaching `canvas.Image`.
- R1.3 The cache belongs to the dialog, not to the program: the frames go when
  the dialog does, rather than outliving the run that produced them.
- R1.4 `OnImageSaved` already runs off the UI thread, which is where the cache
  requires the decode to happen; the hop to `fyne.Do` is unchanged.

### R2. Guard

- R2.1 A test drives `OnImageSaved` with more images than the cache holds, each
  larger than the preview box in both axes, and pins that what reaches
  `canvas.Image` is within the box and that the cache evicted rather than grew.

### R3. Release

- R3.1 v4.2.3, a patch.

## Acceptance Criteria

- [x] AC1 The run dialog owns a preview cache and fetches through it (R1.1, R1.2)
  - Verified: `rundialog.go` — `previews: newPreviewCache()` at construction, `d.previews.get(path)` in `OnImageSaved`
- [x] AC2 The frame handed to `canvas.Image` fits the preview box (R1.2)
  - Verified: `TestTheRunDialogPreviewIsScaledAndBounded`; with the old body it fails with `"3200" is not less than or equal to "1600"`
- [x] AC3 A run saving more images than the cap does not keep them all (R2.1)
  - Verified: same test — `len(previews.items) <= previewCap` after 20 saves
- [x] AC4 The cache does not outlive the dialog (R1.3)
  - Verified: it is a field on `runDialog`, which the dialog's close drops; no package-level cache is introduced
- [x] AC5 `make test` passes
  - Verified: all packages ok
- [x] AC6 `make lint` is clean
  - Verified: 0 issues
- [x] AC7 `govulncheck ./...` is clean
  - Verified: no vulnerabilities found
- [x] AC8 Released as v4.2.3
  - Verified: see the validation report

## Risks & Assumptions

- **Rollback**: revert the commit; v4.2.2 is installable from the AUR and the
  GitHub artifacts. Rolling back restores the bug.
- **The preview is now a thumbnail.** It was already meant to be: the dialog's
  preview pane is a few hundred pixels, and `previewMaxW`×`previewMaxH` is
  1600×900, larger than the pane so a 1.5× HiDPI scale still gets a pixel per
  pixel. Nothing visible should change except that it arrives sooner.
- **Not measured on the affected host after the fix.** The mechanism is pinned
  by a test and the arithmetic is unambiguous — 33 MB per frame re-scaled
  several times a second against a scaled frame roughly twenty times smaller —
  but the confirmation that the CPU comes down is a manual check on that
  machine.

## Gaps found

- **Nothing bounds what the window decodes.** This was the second place to hand
  a full-size frame to Fyne; the review was the first, and `preview.go` exists
  because of it. A third will be written eventually. Worth a check of the kind
  used for the affixed controls: a test that fails when a `canvas.Image` is
  assigned a frame larger than the preview box, wherever it happens.
- **The Service section's poll rebuilds on a moving timestamp** (carried from
  spec 015, still open).
- **Nothing tells the user their unit is stale** (carried from spec 015, still
  open).

## Alternatives Considered

- **Share one cache across the window.** Rejected: the run dialog's images are
  the ones just downloaded and the review's are the directory's; a shared cache
  would keep a finished run's frames alive behind whatever the user looks at
  next, which is the memory behaviour being fixed.
- **Scale in `showPreview` instead.** Rejected: that scales on the UI thread,
  which is the thread that was saturated.
