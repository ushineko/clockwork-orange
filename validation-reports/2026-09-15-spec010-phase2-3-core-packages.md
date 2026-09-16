## Validation Report: Spec 010 Phases 2–3 — Config, Imaging, Stores, Platform, Engine, Core

**Date**: 2026-09-15
**Spec**: specs/010-go-rewrite-v4.md (Phases 2 and 3; plugins from Phase 4 also land here)
**Branch**: v4-go-rewrite
**Status**: PASSED

### Summary

Implements the headless layers of the Go port: `internal/config` (YAML
document with verbatim preservation of unknown keys and plugin blocks,
google_images migration, atomic save, fsnotify watcher with spec 009
debounce), `internal/imaging` (decode, cover-resize/crop, thumbnail, MD5 and
streamed SHA-256), `internal/store` (history.db and blacklist.db over
modernc.org/sqlite, schema byte-identical to Python), `internal/platform`
(KDE qdbus6/kwriteconfig6, systemd, single-instance lock, Windows stitching
and registry, macOS AppKit/osascript and cache prune), `internal/plugins`
(local, wallhaven, duckduckgo_images), `internal/engine` (fair selection,
per-monitor de-dup, dual mode, URL download) and `internal/core`
(request/result operations, dynamic cycle, daemon loop with watcher, service
and store operations, self-test, version). Packaging drafts (desktop entry,
PKGBUILD, install/uninstall, Debian build script) are included but wired in
Phase 6.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): 8 packages ok, 207 test
  functions. Live tests (`CLOCKWORK_LIVE_KDE`, `CLOCKWORK_LIVE_NET`,
  `CLOCKWORK_LIVE_SYSTEMD`) skipped; not run against the developer's
  session in this pass.
- Golden fixtures verified: config migrations (both cases, compared as parsed
  maps), MD5/SHA-256 of all fixture images, thumbnail sizes, Python-created
  SQLite DBs (stats, seen_url, seen_image, items, is_blacklisted), Go-written
  schema identical to Python's `sqlite_master`, KDE argv (single, multi,
  lockscreen, reload), Wallhaven `_build_api_params` table, `_parse_queries`,
  DDG filenames.
- Cross-compile checks: `GOOS=windows` and `GOOS=darwin` with
  `CGO_ENABLED=0` vet clean for `internal/platform`. The macOS cgo file
  cannot be compiled on this machine; verified in Phase 6 CI.
- `go build ./...` and `go vet ./...`: OK.

### Phase 4: Code Quality

- `make lint`: 0 issues (golangci-lint v2.12.2, nmsbonker config).
- Known Python quirks are ported as-is and documented in doc comments; new
  deviations recorded as DV11–DV14 in the spec.
- Plugin tests take ~49 s under `-race` because of 3840×2160 resizes; noted
  for a possible fixture-size reduction later.

### Phase 5: Security Review

- `govulncheck ./...` (v1.1.4, GOTOOLCHAIN=go1.26.0): 0 vulnerabilities
  reachable from this module's code (6 in imported packages, 10 in required
  modules, none called).
- OWASP pass on new network code: all requests use
  `http.NewRequestWithContext` with per-request timeouts; bodies closed;
  Wallhaven API key never logged (DV13); no credentials in source. KDE JS
  payload escapes paths (DV1). Paths from config go through `ExpandPath` and
  the bare-tilde guard before directory creation.
- No secrets in changed files.

### Phase 5.5: Release Safety

- Rollback: revert the commit(s) on `v4-go-rewrite`; `main` untouched.
- Additive: no Python file modified; no packaging output changes yet.

### Phase 6.5: Spec Reconciliation

Phase 2 and Phase 3 acceptance criteria checked in the spec with two
annotated clarifications (Python-reads-Go DB step deferred to Phase 7 manual
verification; fairness test parameters). Spec status IN_PROGRESS.
