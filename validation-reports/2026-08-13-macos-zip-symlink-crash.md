## Validation Report: macOS release crash — true root cause (zip dereferenced symlinks)

**Date**: 2026-08-13
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: none (release-build bug fix)
**Supersedes diagnosis in**: `2026-08-13-macos-broken-release-build.md` (v2.9.8)

### Correction

The v2.9.8 report attributed the macOS launch crash to a non-reproducible
build toolchain (unpinned `PyInstaller` / `PyQt6-Qt6`). **That diagnosis was
wrong.** After v2.9.8 shipped with the pinned toolchain, the CI-built bundle
**still segfaulted** with the identical backtrace, while a local build from the
same source and same pinned toolchain launched fine. That contradiction
isolated the real cause.

The v2.9.8 changes (pin the Qt stack, pin PyInstaller, stamp the version) are
retained — they are correct hygiene and the version stamp works (the released
bundle now reports 2.9.8, not 0.0.0) — but they were not the fix.

### Root cause (confirmed)

The `Package App as ZIP` step in `.github/workflows/build.yml` used
`zip -r` **without `-y`**. Per `man zip`, without `-y` (`--symlinks`) zip
**follows** symbolic links and stores the referenced file's content instead of
the link. A macOS `.app` relies on framework symlinks
(`Python.framework/Python -> Versions/Current/Python`, the Qt frameworks,
etc.). Dereferencing them produces a malformed bundle: `codesign` reports
"bundle format is ambiguous", and Qt's startup path resolution
(`QLibraryInfoPrivate::paths` -> `CFBundleCopyBundleURL` ->
`__CFCheckCFInfoPACSignature`) faults with `EXC_BAD_ACCESS` before any Python
runs -> `SIGSEGV` (exit 139).

The build itself is fine; the *packaging* destroyed it. A locally-run
`dist/` build never hits this because it is launched directly, never zipped.

### Proof (local, macOS 26.5.2 arm64)

Took the known-good local build (99 framework symlinks) and packaged it two
ways, then extracted and launched each:

| Package cmd | symlinks after extract | codesign | `--self-test` exit |
|-------------|------------------------|----------|--------------------|
| `zip -r` (old CI) | 0 | "bundle format is ambiguous" | **139 (crash)** |
| `zip -r -y` (fix) | 99 | valid (rc=0) | **0 (works)** |

The `zip -r` case reproduces the released v2.9.8 bundle exactly (0 symlinks,
ambiguous format, crash). `zip -r -y` preserves all 99 symlinks, keeps the
signature valid, and launches.

The downloaded v2.9.8 release asset was also inspected directly:
`zipinfo Clockwork-Orange-macOS.zip` shows **0 symlink entries** and
`Python.framework/Python` stored as a 5 MB regular file — confirming the
symlinks were dereferenced at package time in CI.

### Change

- `.github/workflows/build.yml`, `Package App as ZIP`:
  `zip -r` -> `zip -r -y`, with a comment explaining why `-y` is mandatory.

### Phase 3: Tests / Verification

- No application code changed (CI-packaging one-flag fix). Unit suite
  unaffected; `pytest` not installed locally (not installed, per tool policy —
  it validates nothing here).
- Verification is the packaging A/B test above, on the affected OS
  (macOS 26.5.2 / Tahoe, arm64). `zip -r -y` output: 99 symlinks preserved,
  signature valid, self-test exit 0, GUI launches.
- Status: PASSED

### Phase 4: Code Quality

- One-line flag change plus an explanatory comment so the flag is not
  "cleaned up" later. No dead code, no duplication.
- Status: PASSED

### Phase 5: Security

- No dependency or code-surface change; no new packages. `pip-audit` not run
  (not installed) — no CVE surface delta to scan. No secrets added or logged.
- Status: PASSED

### Phase 5.5: Release Safety (simplified — desktop app)

- Rollback: revert this one-line workflow change; prior releases remain on
  GitHub Releases.
- Additive/low-risk: makes zip preserve symlinks (correct macOS `.app`
  behaviour). No effect on Linux/Windows packaging paths.
- Coverage gap (unchanged from v2.9.8, still worth closing): CI runs
  `--self-test` on the freshly-built `dist/` tree *before* zipping, so it never
  exercises the packaged/round-tripped bundle and cannot catch symlink loss.
  Follow-up: unzip the produced artifact and run `--self-test` on the extracted
  `.app` as a post-package CI gate.
- Status: PASSED
