## Validation Report: DuckDuckGo transport diagnostic probe

**Date**: 2026-08-17 21:50
**Commit**: (pre-commit, branch `diagnostics/ddg-transport-probe`)
**Status**: PASSED
**Spec**: (none — diagnostic tooling following user report of irrelevant
DDG downloads and to verify the unproven TLS-block claim in
validation-reports/2026-04-17-1646-ddg-ddgs-backend.md)

### Context

Two open questions motivated this work:

1. The DDG plugin has been downloading irrelevant images (cartoons, sports
   teams, etc.). The shipped path uses `ddgs.images()`, whose 9.x backends
   drop the server-side content filters.
2. The 2026-04-17 report attributed a Windows "0 images" failure to a DDG
   "TLS-fingerprint soft-block" — but that was labeled "consistent with"
   (an inference), never measured. The `ddgs`/`primp` dependency and the
   per-OS code-path split rest on that unverified claim.

Rather than act on a four-month-old inference, this change adds a probe that
measures the actual OpenSSL/TLS stack, the JA3/JA4 fingerprint, and the real
`i.js` block status for three transports, runnable inside the frozen build
where the failure was seen.

### Change

- **New**: `ddg_probe.py` — self-contained diagnostic (no PyQt deps).
  Reports environment (OpenSSL, certifi, requests/primp/ddgs versions),
  TLS fingerprint via `tls.browserleaks.com` (peet.ws fallback), and the
  real vqd + `i.js` flow for three transports: `requests`,
  `primp-default` (as ddgs drives it), `primp-imp` (browser impersonation).
- **`clockwork-orange.py`**: `--ddg-probe` flag + handler (mirrors the
  existing `--self-test` pattern), so the probe runs inside frozen builds.
- **`.github/workflows/build.yml`**: non-fatal (`continue-on-error`) probe
  steps on the Windows and macOS jobs. Windows uses `Start-Process
  -RedirectStandardOutput` because the production exe is `--noconsole`.

No change to `requirements.txt`, plugin behavior, or on-disk config. This is
diagnostic scaffolding only.

### Measured results (this run)

macOS dev venv AND frozen `.app` (Homebrew Python 3.13, OpenSSL 3.6.2),
identical outcomes:

| Transport      | i.js result        | JA4          |
|----------------|--------------------|--------------|
| requests       | 200, 93–97 results | t13d1712h1 (HTTP/1.1) |
| primp-default  | 200, 93–97 results | t13d1012h2 (HTTP/2)   |
| primp-imp      | **403 REJECTED**   | t13d2014h2   |

Key findings, contradicting the original inference:

- The macOS build (frozen or not) is **not blocked** — plain `requests`
  works. `requests` JA4 is **identical frozen vs unfrozen**, confirming
  PyInstaller freezing does not alter the TLS fingerprint. The 2026-04-17
  block was Windows-specific (setup-python 3.12 OpenSSL), which only the CI
  Windows probe can measure.
- **Browser impersonation gets a flat 403** on `i.js`, every OS profile.
  A prior sketch to "unify on i.js + primp impersonating a browser" would
  have shipped a 403. The working primp config is the **default**
  (no impersonation), matching how `ddgs` uses it.

### Phase 3: Tests

- `pytest tests/ test_platform_utils_build.py`: **22 passed, 2 skipped**.
- Full run also surfaces 1 pre-existing collection ERROR in
  `scripts/test_watchdog_frozen.py` (a helper fn `test_single_import` that
  pytest mis-collects; last modified in `da97deb`, unrelated to this change).
- `--ddg-probe` executed successfully inside the frozen `.app` (proves the
  new module bundles and runs in a PyInstaller build).
- `ast.parse` clean on both changed Python files.

### Phase 4: Code Quality

- `ddg_probe.py` is self-contained; each transport factored into its own
  function; per-step try/except so the probe never crashes a build. No dead
  code. Pyright flags on `requests`/`primp`/`certifi` imports are expected
  (optional, guarded, absent from the linter venv).

### Phase 5: Security Review

- **Secrets**: grep for key/secret/password/token in `ddg_probe.py` — none.
- **Dependencies**: `requirements.txt` unchanged; no new dependency ships.
  `ddgs`/`primp` were already pinned; `pytest`/`ddgs`/`primp` installed into
  the local `.venv` only.
- `pip-audit`: not run (not installed; per tooling policy, not installed
  without approval). No new deps introduced, so risk delta is zero.
- **Egress**: probe contacts fixed diagnostic hosts (browserleaks, peet.ws,
  duckduckgo). Hardcoded, no user-controlled URLs; not reachable in normal
  app operation (guarded behind `--ddg-probe`).

### Phase 5.5: Release Safety (Simplified)

- **Rollback**: revert the commit / delete the branch. `--ddg-probe` is an
  additive, opt-in flag; no default behavior changes. CI steps are
  `continue-on-error` and cannot fail a release.
- **User-visible impact**: none in normal operation.

### Overall

- All gates passed: YES
- Verified locally on a frozen macOS `.app`. CI will additionally run the
  probe on the Windows and macOS build jobs to capture the one environment
  (Windows/OpenSSL) where the block was originally reported.
