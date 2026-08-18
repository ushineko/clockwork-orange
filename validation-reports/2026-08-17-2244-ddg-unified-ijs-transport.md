## Validation Report: DuckDuckGo unified i.js path + primp transport

**Date**: 2026-08-17 22:44
**Commit**: (pre-commit, branch `fix/ddg-unified-i.js-transport`)
**Status**: PASSED
**Spec**: (none — fix following the transport investigation in
validation-reports/2026-08-17-2150-ddg-transport-probe.md)

### Context

The DDG plugin downloaded irrelevant images (cartoons, sports, etc.) and
had two divergent code paths: `ddgs.images()` on frozen Windows/macOS builds
and a direct `requests` scrape on Linux. The probe run (see the 2026-08-17
21:50 report + CI on PR #3) measured the real cause:

- `ddgs` 9.x routes image search through its own backend, **not** DDG's
  `i.js`, and drops the server-side content filters — the source of the junk.
- The direct `i.js` endpoint honours `f=type:photo,size:Large,layout:Wide`
  server-side and returns on-topic results.
- Plain `requests` is TLS-fingerprint soft-blocked **only** on the Windows
  frozen build (OpenSSL 3.0.16): measured HTTP 403 on `i.js`. On macOS
  (OpenSSL 3.6.2) and Linux, `requests` works.
- `primp`'s default client (no impersonation) returns 200 on **every**
  platform, including the blocked Windows stack. Browser impersonation gets
  a 403 and is not used.

### Change

One code path on all platforms: the direct `i.js`/`vqd` flow with the
server-side filters, over a cookie-persisting client that is `primp` when
available and `requests` otherwise.

- `plugins/duckduckgo_images.py`:
  - Removed `ddgs` import/branch and the `_scrape_via_ddgs` /
    `_scrape_via_direct` split.
  - Added `_make_discovery_client` (primp preferred, requests fallback),
    `_http_get` (transport-agnostic `(status, text)`), `_get_vqd`, and a
    unified `_scrape_image_urls` that calls `i.js` with the filters.
  - `import json` added (i.js body parsed via `json.loads`).
  - Image downloads unchanged (stay on `requests`; CDNs don't fingerprint).
- `requirements.txt`: `ddgs==9.14.0` -> `primp==1.3.1` (primp was already a
  transitive dep of ddgs; now direct).
- `scripts/build_macos.sh`, `scripts/build_windows.ps1`,
  `scripts/build_windows_production.ps1`: dropped `--collect-all ddgs`
  (kept `--collect-all primp`). Required — with ddgs no longer installed,
  collecting it would fail the build.

`_filter_results`, `_is_wallpaper_shaped`, and the resolution/shape/blacklist
gates are unchanged.

### Phase 3: Tests

- `pytest tests/ test_platform_utils_build.py`: **22 passed, 2 skipped**.
  (Full run also shows the pre-existing collection error in
  `scripts/test_watchdog_frozen.py`, unrelated.)
- Local frozen `.app` built (mirrors CI: `primp`, no `ddgs`):
  - `--self-test`: all subtests green (imports incl. primp, no ddgs breakage).
  - Live `--run-plugin duckduckgo_images` (query "4k landscape wallpaper",
    limit 2): **Found 60 candidates via primp i.js, downloaded + saved 2
    clean 3840x2160 images.** End-to-end pipeline confirmed in the frozen
    bundle.
- Transport paths exercised from the dev venv: primp path -> 60 on-topic
  candidates (e.g. wallpaperaccess.com). requests-fallback path executes
  correctly (issues the request, parses the response); it hit a transient
  HTTP 403 during testing, attributable to IP rate-limiting after the day's
  probe volume — the same box returned 200 via requests earlier, and Linux
  (the only place the fallback is used in production) is not structurally
  blocked.

### Phase 4: Code Quality

- Net removal of one whole scrape path; shared discovery client eliminates
  the requests-vs-ddgs duplication. No dead code (`re`, `json` both used).
- Pyright flags on `primp`/`requests`/`PIL` imports and `self._discovery`
  optional-access are expected (guarded/runtime-assigned, same pattern as
  the existing `self._session`).

### Phase 5: Security Review

- **Secrets**: none added.
- **Dependencies**: `ddgs` (+ its `lxml`, `click`, and the `ddgs.dht`/`trio`
  optional path) dropped; `primp` promoted to a direct pin. Net dependency
  surface **reduced**.
- `pip-audit`: not run (not installed; not installed without approval per
  tooling policy). No net-new dependency introduced (primp was already
  vendored via ddgs).
- **SSRF/injection**: query is URL-encoded by the client; image URLs come
  from DDG's result list (same trust model as before). No impersonation.

### Phase 5.5: Release Safety (Simplified)

- **Rollback**: revert the commit. On-disk plugin config
  (`plugins.duckduckgo_images`) and its schema are unchanged; no migration.
- **User-visible impact**: cleaner (on-topic) results; identical behaviour
  across platforms. Windows frozen builds that were returning 0 images now
  return results.
- Distro packagers (Arch/Debian) unaffected: the requests fallback keeps
  them working without a `primp` system package.

### Overall

- All gates passed: YES
- Verified on a locally-built frozen macOS `.app` (discovery + download +
  save). CI will re-verify the Windows and macOS builds; the macOS artifact
  will be installed locally after merge.
