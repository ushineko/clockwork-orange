## Validation Report: Spec 010 Phases 6–7 — Packaging, CI, Cutover

**Date**: 2026-09-15
**Spec**: specs/010-go-rewrite-v4.md (R8, R10; R9.6 partial)
**Branch**: v4-go-rewrite → main
**Status**: PASSED locally; CI verification on the release tag

### Summary

Cutover commit: every Python source and Python-only tool listed in R8.9 is
deleted (`clockwork-orange.py`, `gui/`, `plugins/`, `scripts/*.ps1`,
`scripts/*.py`, root `PKGBUILD`, `debian/`, `build_deb.sh`, the Python unit
and launcher entry, `requirements.txt`, `.flake8`, the Python tests and the
golden capture script). Kept: `.tag`, `release_version.sh`,
`scripts/update_aur.sh` (paths updated), fixtures, specs, reports, `img/`.

CI (`.github/workflows/build.yml`) rewritten for Go: `test-go` (unchanged),
`build-arch` (makepkg in the Arch container, `pacman -U`, full `--self-test`),
`build-deb` (`packaging/debian/build.sh`, install, `version` + `plugins
list`; the KDE tools are Recommends and absent on the runner), `build-windows`
(MinGW via msys2, CLI without CGO, GUI through `fyne package` with the icon
and windowsgui subsystem, self-test, one zip), `build-macos` (`fyne package`
→ `Clockwork Orange.app` with the CLI copied in, self-test, zip), `release`
(tag must equal `.tag`, `softprops/action-gh-release@v2`), `publish-aur`
(`packaging/arch/aur/PKGBUILD` + `makepkg --printsrcinfo`, DV6). Windows and
macOS stamp the version through a generated `internal/buildinfo/version_ci.go`
because `fyne package` drives the Go build.

Docs: README (v4, with links), GUI.md (new screenshots of every section,
`docs/img/01–08`), WALKTHROUGH.md (Windows and macOS for v4), specs/README.md,
`.claude/CLAUDE.md`, `tests/golden/README.md`, `clockwork-orange.yml.example`.

### Phase 3: Tests

- `make test`: 10 packages ok; `make lint`: 0 issues; `go build ./...`,
  `make build build-gui`: OK, after the deletion.
- `git grep -l python` returns only historical specs, validation reports and
  spec 010 (AC R8.9), plus the fixtures README's regeneration note.
- Screenshots captured from the installed GUI on KDE Plasma 6 (Wayland,
  Breeze Dark, 1.2× interface scale) with `spectacle`.

### Phase 4: Code Quality

- The size poll's first reading is a baseline, not a resize: the window no
  longer flashes "Saved" on every start.
- Two PKGBUILDs (checkout-built and AUR) share `package()` by copy; noted at
  the top of each.

### Phase 5: Security Review

- `govulncheck -mode binary` on both binaries: clean (unchanged since the
  Phase 5 report; no new dependencies in this commit).
- CI secrets: `AUR_SSH_KEY` handling unchanged (`printf '%s\n'`, 0600, no
  echo). The release job has `contents: write` only where it uploads.
- No secrets in changed files.

### Phase 5.5: Release Safety

- Rollback: the Python program is the `v2.9.5` tag and the AUR package's
  previous revision; `git revert` of the cutover commit restores the tree.
  User data (config, databases) is shared and untouched.
- Release: `.tag` → `v4.0.0` (D1), `release_version.sh` tags and pushes;
  the `release` job refuses a mismatch.

### Phase 6.5: Spec Reconciliation

Phase 6 and 7 criteria checked or annotated: the container/CI-only checks
are verified by the release run; the manual Windows and macOS verification
(R9.6) is the operator's, after the release. Spec status stays IN_PROGRESS
until then. Executive Summary written.
