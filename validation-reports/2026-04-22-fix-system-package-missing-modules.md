## Validation Report: Fix missing modules in Arch/Debian packaging

**Date**: 2026-04-22
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: (none — packaging bugfix following user report)

### Context

AUR build of `clockwork-orange-git` on a cachyos system failed at runtime
with `ModuleNotFoundError: No module named 'config_migrations'`.

`config_migrations.py` was introduced in `be895b7` (DDG plugin swap) as a
top-level import in `clockwork-orange.py:22` but never added to the
distro-package install lists. The Debian `debian/install` was also missing
`platform_utils.py`, which would fail the same way once exercised.

Windows/macOS PyInstaller builds are unaffected: PyInstaller discovers
these modules via static import analysis and bundles them automatically.
Only package installers that enumerate files explicitly (PKGBUILD,
debian/install) were broken.

### Fix

- `PKGBUILD`: added `install -Dm644 config_migrations.py ...` alongside
  the existing `plugin_manager.py` / `platform_utils.py` installs.
- `debian/install`: added `config_migrations.py` and `platform_utils.py`
  to the install manifest.

### Phase 3: Tests

- No runtime code changed; existing pytest suite unaffected.
- Manual verification: `grep "^import\|^from" clockwork-orange.py`
  confirms the full top-level import list (`platform_utils`,
  `config_migrations`, `plugin_manager`); all three are now listed in
  both packaging manifests.

### Phase 4: Code Quality

- Change is purely additive to two install manifests.
- No dead code introduced. `repro_interval.py` at repo root is
  orphaned (no imports anywhere) but out of scope for this fix.

### Phase 5: Security Review

- No source code changes; no new dependencies.
- Packaging manifests expose the same files already tracked in git.
- No secrets, no network, no new attack surface.

### Phase 5.5: Release Safety (Simplified)

- Rollback: users reinstall v2.9.2 (AUR) or keep the previously installed
  version. Debian users were never affected in practice (no published
  .deb release). No config/state migration.
- User-visible impact: AUR installs launch successfully instead of
  crashing on first import.

### Overall

- All gates passed: YES
- Cut as patch release v2.9.3.
