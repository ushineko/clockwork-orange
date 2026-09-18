# Project-Specific Guidelines: clockwork-orange (v4, Go)

This file extends the global Ralph methodology (`~/.claude/CLAUDE.md`).
It describes the **Go program (v4, spec 010)**. The Python 2.9.x line it
replaced lives in the `v2.9.5` tag; specs 001–009 document it.

---

## Selected Policies

- `languages/go.md`
- `languages/bash.md`
- `git/standard.md`
- `release-safety/simplified.md`
- `security/owasp-review.md`
- `testing/philosophy.md`
- `communication/standards.md`

---

## Project Overview

- **Type**: Go CLI/daemon + desktop GUI (Fyne)
- **Purpose**: Wallpaper and lock-screen manager with source plugins (local
  folder, Wallhaven, DuckDuckGo Images), shared blacklist/history, systemd
  daemon on KDE Plasma 6, tray app on Windows 10/11 and macOS 13+.
- **Module**: `github.com/ushineko/clockwork-orange`
- **Design reference**: `github.com/ushineko/fynedesygn` (checkout at
  `~/git/fynedesygn`): the Fyne design system `internal/gui` is built on. Its
  `docs/design-system.md` is the rulebook; a shape a second section needs
  belongs in the library, not copied here. Anything the library lacks is
  recorded in the spec's "Gaps found" for a library spec, never worked around
  with a copy. `~/git/nmsbonker` remains the reference for build layout and
  CI. When this file and their conventions disagree, this file wins.
- **Spec of record**: `specs/010-go-rewrite-v4.md`. Requirement IDs (`R2.6`,
  `DV1`) are referenced from code comments and tests.

---

## Issue Tracking

Personal public GitHub repository, no issue tracker. Spec files are named
without ticket IDs (`specs/NNN-short-description.md`). Do not prompt for
ticket IDs.

---

## Architecture rules

- **CLI/GUI parity**: every user-facing operation is a headless function in
  `internal/core` taking a request struct and returning a result struct. The
  cobra CLI (`cmd/clockwork-orange`) and the Fyne GUI
  (`cmd/clockwork-orange-gui`) render only. A new operation lands in core
  first, then in both front ends, in the same commit. Enforced by
  `tests/parity` with a documented allow-list.
- **Straight port**: behaviour matches Python 2.9.5 unless the spec's
  "Deliberate Deviations" table says otherwise. Known quirks are ported, not
  fixed, without a spec entry. Golden fixtures in `tests/golden/` are the
  oracle; do not regenerate them from Go.
- **On-disk compatibility**: `~/.config/clockwork-orange.yml` (YAML, sorted
  keys), `~/.config/clockwork-orange/history.db` and `blacklist.db` must stay
  readable by both 2.9.x and 4.x. Unknown YAML keys and plugin blocks
  (notably `stable_diffusion`) are preserved on save.
- **Long-running work is cancellable** (`context.Context`) and reports
  progress through `internal/events`; the GUI never blocks its render thread
  (`fyne.Do`, never `fyne.DoAndWait`).
- **Nothing transient may reflow the interface**: result banners and the
  progress indicator float over the content as popups and never insert
  themselves into a section's layout.
- **Platform code lives in `internal/platform`** behind interfaces with fakes;
  build-tagged files per OS. External tools (`qdbus6`, `kwriteconfig6`,
  `systemctl`, `journalctl`, `osascript`, `du`) are invoked with exact argv
  recorded in golden tests.
- **The daemon builds with `CGO_ENABLED=0`.** Only the GUI may need CGO.

---

## Environment

- Go from `go.mod` (`go 1.25.0` minimum, no `toolchain` line; the linter pin
  lives in the Makefile). Local toolchain may be newer.
- Fyne needs CGO, OpenGL and X11/Wayland headers for `make build-gui`.
- Runtime tools on Linux: KDE Plasma 6 (`qdbus6`, `kwriteconfig6`), systemd
  user session.
- `make test` runs `go test -race -tags parity ./...`; `make lint` runs the
  pinned golangci-lint with `config/.golangci-v2.12.2.yml`.
- Live integration tests are env-gated: `CLOCKWORK_LIVE_KDE=1`,
  `CLOCKWORK_LIVE_NET=1`, `CLOCKWORK_LIVE_SYSTEMD=1`.

---

## Platform-Specific Testing

- Test on Windows when touching `internal/platform/*_windows.go`.
- Test on macOS when touching `internal/platform/*_darwin.go` or cgo code.
- Run `--self-test` on every packaged artifact (CI does this).

---

## Git

- Feature work happens on branches and lands on `main` through a PR.
- Never add `Co-Authored-By` trailers or AI attribution footers. No
  exceptions.
- Commit subjects: lowercase conventional prefix, imperative
  (`feat(config): port google_images migration`).
- Connectivity check before push/pull (`git/standard.md`).

---

## Release Workflow

`.tag` (`vX.Y.Z`) is the version of record; the Makefile, PKGBUILD and CI
derive from it. When the user says **"release"**: determine the semver bump,
confirm with `AskUserQuestion`, update `.tag`, run `./release_version.sh`,
then verify the tag on GitHub and the Actions build. The CI `release` job
refuses a tag that differs from `.tag`.

---

## Security Extensions

- No network credentials in source; the Wallhaven API key comes from the
  config file only and is never logged.
- Validate paths for wallpaper sources; escape paths interpolated into the
  KDE JavaScript payload (DV1).
- Plugins never execute code from network sources.
- Dependency scan: `govulncheck ./...` before each release commit.

---

## Configuration Summary

| Category | Setting | Policy Module | Notes |
|----------|---------|---------------|-------|
| Git | Standard | `git/standard.md` | Conventional commits, connectivity checks, no co-authored-by |
| Release Safety | Simplified | `release-safety/simplified.md` | Desktop app; rollback = reinstall previous release |
| Go | Policy | `languages/go.md` | Plus the architecture rules above |
| Validation Reports | Strict | *(core methodology)* | Required before every commit with code changes |
| Code Quality Checks | Strict | *(core methodology)* | Dead code, duplication, `make lint` clean |
| Test Requirements | Strict | *(core methodology)* | `make test` must pass |
| Communication Style | Strict | `communication/standards.md` | Factual language, no superlatives |
| Tool Installation | Strict | *(core methodology)* | Always ask before installing |
| Security | Mandatory | `security/owasp-review.md` | govulncheck, OWASP checks |

---

*Updated 2026-09-15 — rewritten for the v4 Go port (spec 010 Phase 1)*
