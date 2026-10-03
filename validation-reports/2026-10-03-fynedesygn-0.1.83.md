## Validation Report: fynedesygn v0.1.40 → v0.1.83

**Date**: 2026-10-03
**Spec**: none — dependency bump
**Status**: PASSED

### Summary

Forty-three library releases. The one breaking change (0.1.78, `glance/kwin`
scripts take a `kwin.Target`) is in a package this program does not import.
Two releases change what this program draws, and one adds a shape it was
missing:

- **0.1.55, sections scroll up and down only.** A section's minimum width is
  now the window's: content wider than the pane forces the window wider
  instead of scrolling sideways. Measured at the default window (content pane
  `1180 × (1 − 0.16) = 991`), the DuckDuckGo and Wallhaven Review tabs were
  1048–1154 wide under the test theme, varying with the temp path. The cause was the toolbar's
  "N image(s) in <dir>" label, whose width is the length of the download path.
  The label now truncates; both tabs measure 550. Every other section was
  already under 991 (widest: Appearance, 651).
- **0.1.83, `Shell.VScroll`.** A scroller whose offset survives a rebuild in
  place (fynedesygn spec 054). The four sections with affixed controls
  (spec 014) each built a plain `container.NewVScroll`, so every rebuild threw
  the body back to the top. Service rebuilds on its 5 s status poll, so its
  status pane could not stay scrolled for longer than that. History, Service,
  Appearance and each plugin's Configuration tab now use `u.sh.VScroll`.
  Navigation still starts at the top (library behaviour).
- **0.1.70, settings store stops with the shell.** Free; `Shell.Stop` calls it.
- **0.1.79, tips go away when the pointer leaves the window.** Free.
- The rest is `glance`, `glance/kwin`, the font chooser, markdown and docs.
  Nothing this program imports changed shape.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages ok, 0 failing.
- New `TestEverySectionFitsTheDefaultWindow`: builds every section under both
  plugin tabs and requires its minimum width to fit the default content pane.
  Fails on the pre-change toolbar ("Wallhaven (plugin tab 1) is 1154 wide; the
  default content pane is 991"), passes after.
- New `TestARebuildKeepsTheSectionsScrollPosition`: for every section named in
  `AffixedActions` except Blacklist (whose only scroller is its table's), sets
  the body scroller's offset, rebuilds the section, and requires the new
  scroller to carry the offset. Fails on the pre-change views ("Service: a
  rebuild threw the body back to the top"), passes after.
- Both tests assert what the window does (width, scroll position), not which
  helper builds it.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- Change is `go.mod`, `go.sum`, five lines across four view files, two tests.
  No dead code introduced. The truncating label is local, following
  `review.go`'s info label; the library has no truncating `Dim`.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- No new network path, external command, or credential handling. No
  hardcoded secrets in the diff.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit, or reinstall the previous release from the
  AUR or the GitHub artifacts. No on-disk format change; config, history,
  blacklist and `gui-settings.json` are untouched.

### Not verified

- Widths were measured under the Fyne test theme, not the real fonts. The
  test bounds the toolbar by construction (the path label truncates), so the
  real face changes the numbers but not the conclusion.
- Scroll retention was exercised through section rebuilds in a headless
  shell, not by scrolling a live window during a status poll.
- Windows and macOS were not run by hand; CI covers the builds.

### Overall

**PASSED.**
