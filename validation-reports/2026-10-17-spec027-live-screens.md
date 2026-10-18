## Validation Report: Spec 027, only desktops on a live screen are monitors

**Date**: 2026-10-17
**Spec**: `specs/027-only-live-screens-are-monitors.md` (#47)
**Status**: PASSED

### Summary

`LiveDesktopsJS` replaces `desktops()` in the KDE count, single and multi
scripts (DV17).

### Phase 3: Tests

- `make test`: all packages pass.
- The golden test applies exactly the DV17 substitution to the Python
  fixture.
- The count test covers the new script and the 0 → 1 fallback.
- Live, on this Plasma 6 session: the expression yields 2 of 3
  containments, and a skip after reinstall leaves the orphan untouched.

### Phase 4: Code Quality

- `make lint`: 0 issues. One constant, three call sites.

### Phase 5: Security Review

- `govulncheck`: no vulnerabilities found.
- No user input reaches the new JS; it is a constant.
- Path escaping (DV1) is unchanged.

### Phase 5.5: Release Safety

- Revert restores the old count. No on-disk change.

### Overall

**PASSED.**
