## Validation Report: Spec 016 — The run dialog's preview is scaled

**Date**: 2026-09-18 23:00
**Spec**: specs/016-the-run-dialog-preview-is-scaled.md
**Status**: PASSED

### Summary

Reported as a compositor problem — "the window blurring effect eats a ton of
CPU, makes the window transparent and flicker, and it does not do that on the
other machine". It is not the compositor. Measured over SSH on the affected
host during a Wallhaven run:

    %CPU  RSS       NLWP  ELAPSED  COMMAND
     231  1.6 GB      86    07:04  /usr/bin/clockwork-orange-gui

KWin desaturates a window that stops answering, so a saturated UI thread reads
as the compositor doing something to the window. The difference between the two
machines was that one was downloading 4K wallpapers at the time.

`runEvents`'s `OnImageSaved` decoded each saved image at full size and handed
the frame to `canvas.Image`, which makes Fyne re-scale 3840×2160 on every
redraw — and the run dialog redraws constantly, because the log pane pumps and
the progress bar moves for the whole download. That host's Wallhaven directory
holds 499 files and 3.2 GB with `atleast: 3840x2160`; a decoded 4K RGBA frame is
about 33 MB.

`internal/gui/preview.go` was written for this exact problem in the review pane,
and says so in its doc comment. The dialog never used it. It does now: decoded
once, scaled to 1600×900, bounded at sixteen frames that go with the dialog.

Also checked and ruled out on that host: the spec 015 service bug is cleared
there — `ExecStart=/usr/bin/clockwork-orange --service`, active, `NRestarts=0`.

### Phase 3: Tests

- `make test`: all packages ok, 0 failing.
- `TestTheRunDialogPreviewIsScaledAndBounded` drives `OnImageSaved` with twenty
  images, each larger than the preview box in both axes, and pins that what
  reaches `canvas.Image` fits the box and that the cache evicted rather than
  grew.
- **Checked against the bug**: with the previous body it fails with
  `"3200" is not less than or equal to "1600"`.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- No new machinery: the fix is the existing `previewCache`, used from a second
  caller. The `imaging` import leaves `rundialog.go` with it.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- No new I/O: the same files are read, by the same decoder, one layer further
  in. Nothing reaches a shell, no network, no credentials. The change reduces
  memory held per run, which is the resource that was being exhausted.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit; v4.2.2 is installable from the AUR and the
  GitHub artifacts. Rolling back restores the bug.
- No on-disk change, no format change, no migration.

### Not verified on this host

That the CPU comes down on the affected machine. The mechanism is pinned by a
test and the arithmetic is unambiguous, but the confirmation is a manual check
during a real download on that host.

### Overall

**PASSED.** Released as v4.2.3, a patch.
