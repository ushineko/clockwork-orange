## Validation Report: Repair broken macOS release build

**Date**: 2026-08-13
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: none (release-build bug fix — no dedicated spec file)

### Problem

The published macOS release (v2.9.7, `/Applications/Clockwork Orange.app`)
segfaults on launch on macOS 26.5.2 (Tahoe) before any Python runs.

- Direct launch: `SIGSEGV` (exit 139), no output. Reproduced repeatedly.
- Crash report (`~/Library/Logs/DiagnosticReports/Clockwork Orange-*.ips`):
  `EXC_BAD_ACCESS (KERN_INVALID_ADDRESS at 0x8)` inside Qt's static
  initializer during dyld load of `QtCore` —
  `qdarwinpermissionplugin` static init → `QLoggingCategory` →
  `QLibraryInfoPrivate::paths` → `CFBundleCopyBundleURL` →
  `__CFCheckCFInfoPACSignature`. This is the known PyInstaller + PyQt6 macOS
  Qt-`.framework` reprocessing crash
  (pyinstaller/pyinstaller#7789).
- Not a Gatekeeper/quarantine issue: crash persists after
  `xattr -cr` (quarantine removed), signature retained.
- The published bundle is stale/mis-signed: `CFBundleShortVersionString`
  is `0.0.0`, and `codesign -v --deep --strict` fails ("bundle format is
  ambiguous", `QtPdf.framework` subcomponent error; ad-hoc, no Team ID).

Root cause: non-reproducible release builds. `requirements.txt` pinned only
`PyQt6==6.10.1` — not the Qt runtime (`PyQt6-Qt6`) or `PyInstaller` — so the
released binary was produced by a toolchain combination that mangled the Qt
framework layout and left an invalid signature. CI's `--self-test` runs on the
GitHub runner's older macOS, so a Tahoe-specific launch crash is invisible to
CI.

Confirmation: a fresh build from the identical source on this Tahoe machine,
with the current pinned toolchain, launches cleanly (GUI + self-test). Same Qt
version (6.10.2) in both the broken and working bundles — the difference is the
PyInstaller packaging, not Qt itself.

### Changes

Build-infrastructure only — no application source touched.

- `requirements.txt`: pin the full Qt stack, not just the bindings —
  `PyQt6==6.10.2`, `PyQt6-Qt6==6.10.2`, `PyQt6-sip==13.11.0`. Prevents the
  Qt runtime from drifting to a newer 6.10.x independent of the pinned bindings.
- `.github/workflows/build.yml`: pin `PyInstaller==6.18.0` in both the macOS
  and Windows build jobs (previously unpinned `pip install PyInstaller`).
- `scripts/build_macos.sh`: stamp the real version into `Info.plist`
  (`CFBundleShortVersionString` = X.Y.Z from `git describe`,
  `CFBundleVersion` = full describe) via `PlistBuddy`, then re-sign the outer
  bundle (`codesign --force --sign -`, no `--deep`, since only `Info.plist`
  changed and `--deep` chokes on bundled `*.dist-info` dirs). Fixes the
  `0.0.0` version and keeps the signature seal valid after the plist edit.

### Phase 3: Tests / Verification

- No application code changed (build-infra only), so the unit suite is
  unaffected. `pytest` is not installed in the local `.venv`; not installed
  (per tool-install policy) because it validates nothing about these changes.
- Meaningful validation is the frozen-build launch test on the affected OS
  (macOS 26.5.2 / Tahoe, arm64), rebuilt with the pinned toolchain
  (PyInstaller 6.18.0, PyQt6 6.10.2 / PyQt6-Qt6 6.10.2):
  - `--self-test`: exit 0, "All passed: True".
  - Version stamp: `CFBundleShortVersionString=2.9.7`,
    `CFBundleVersion=2.9.7-dirty` (local tree on the v2.9.7 tag + dirty;
    the tagged v2.9.8 CI build will stamp `2.9.8`).
  - Signature: `codesign -v --strict` → valid (rc=0) after the plist edit +
    re-sign.
  - GUI launch: process stays alive (Qt initialises; no static-init segfault),
    versus the published build which segfaults at the same point.
- Status: PASSED

### Phase 4: Code Quality

- Dead code / duplication: none. The version-stamp block is a single guarded
  section; short-version extraction is one `sed`.
- Defensive: `PlistBuddy Set ... || Add ...` handles a missing key; re-sign
  failure degrades to a warning rather than aborting the build.
- Status: PASSED

### Phase 5: Security

- Dependency scanner: `pip-audit` not installed; not installed this session
  (per tool-install policy). Changes only pin existing dependencies to specific
  versions already in use (`PyQt6-Qt6`/`PyQt6-sip` are transitive deps of the
  already-shipped `PyQt6`); no new packages introduced, so CVE surface is
  unchanged. Noted as a gap in this report.
- No secrets added or logged. Ad-hoc signing only (no identities/keys in repo).
- Status: PASSED (with scanner gap noted)

### Phase 5.5: Release Safety (simplified — desktop app)

- Rollback: revert this commit; the previous release remains downloadable from
  GitHub Releases. No schema, migration, or persisted-state change.
- Additive/low-risk: pins tighten existing versions; the version-stamp step is
  additive and guarded. If re-signing fails, the build warns and continues
  (falls back to PyInstaller's own signature).
- Coverage gap recorded: CI self-tests macOS on the runner's OS, which is older
  than Tahoe, so it cannot catch this class of launch crash. Follow-up worth
  considering: run the frozen build on a newer macOS image, or add a
  post-release smoke test.
- Status: PASSED
