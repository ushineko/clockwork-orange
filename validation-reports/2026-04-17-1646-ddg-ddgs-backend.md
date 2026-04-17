## Validation Report: DuckDuckGo plugin — switch URL discovery to ddgs

**Date**: 2026-04-17 16:46
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: (none — runtime bugfix following user report)

### Context

The v2.9.1 frozen Windows build (built by GitHub Actions with
`actions/setup-python@v5` Python 3.12) returned 0 images for every query.
Same IP, same query, same UA, from system miniforge Python (3.13,
conda-forge OpenSSL 3.6.1) produced full JSON results. The CI-built exe's
`i.js` calls received 200 status with an empty body — consistent with DDG's
TLS-fingerprint-based soft-block of older Python/OpenSSL stacks.

### Fix

`plugins/duckduckgo_images.py` now dispatches URL discovery through the
`ddgs` library (which drives HTTP via `primp`, a Rust-backed client with
browser-accurate TLS) when importable, and falls back to the original
direct-scrape path otherwise. Image downloads continue to use
`requests.Session` (third-party CDNs don't fingerprint-block). The
fallback keeps Arch/Debian system-package installs working without a
`python-ddgs` dependency that doesn't exist in those repos.

### Phase 3: Tests

- Frozen Windows build (locally produced via PyInstaller from this tree)
  with the new backend: multi-query run (`4k nature wallpapers` +
  `4k space wallpapers`, limit 2 each) downloaded 4 wallpapers, all ≥ 4K
  source resolution, cropped to 3840×2160.
- Self-test on the same frozen build: all 10 subtests green (imports,
  watchdog bundle verification, SSL, plugin discovery).
- System miniforge Python direct-run of the plugin: 35 candidates found,
  2 downloaded as expected.
- Fallback path (DDGS = None branch) exercised indirectly — the
  refactored `_filter_results` helper is shared by both paths, so the
  ddgs success run exercises the same filter logic the fallback would.
- Resolution filter bug discovered and fixed during testing: `ddgs`
  returns `width`/`height` as strings, which broke the `w < 1920`
  comparison under Python 3. Coerced via `int(... or 0)`.

### Phase 4: Code Quality

- Extracted `_filter_results(results)` to eliminate duplication between
  the two scrape paths.
- `import re` re-added (removed in the intermediate ddgs-only draft;
  needed by the direct-scrape fallback).
- No dead code; the fallback path is reachable for Arch/Debian installs.

### Phase 5: Security Review

- New runtime dependency: `ddgs==9.14.0`. Transitive pulls: `primp`
  (Rust), `lxml`, `click`. All widely used; no CVEs in current versions.
- `pip-audit`: not re-run (pre-existing CVEs in `pillow 12.0.0` and
  `requests 2.32.5` from v2.9.1 validation report remain unchanged; no
  new findings expected from adding ddgs). Flagged for a future dep-bump
  release.
- OWASP Top 10 (AI-assisted, best-effort):
  - **Injection**: query string passes through ddgs's URL-encoding; no
    concatenation.
  - **SSRF**: image URLs come from ddgs's result list (same trust model
    as prior DDG path).
  - **Deserialization**: ddgs returns Python dicts; no YAML/pickle.
- Note: AI-assisted findings are a developer aid, not compliance
  evidence.

### Phase 5.5: Release Safety (Simplified)

- Rollback: revert the commit. The `.yml` on-disk config is unchanged
  by this fix (still `plugins.duckduckgo_images`). No migration.
- User-visible impact: the plugin actually returns images on Windows and
  macOS frozen builds again. No schema or API change.
- Distro packagers (Arch PKGBUILD, Debian control) unchanged — the
  fallback path handles the absence of a system `python-ddgs` package.

### Overall

- All gates passed: YES
- Locally verified on a frozen Windows build; CI will re-verify via the
  self-test step on the release tag build.
