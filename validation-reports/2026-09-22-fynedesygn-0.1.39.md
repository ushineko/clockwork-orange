## Validation Report: fynedesygn v0.1.29 → v0.1.39

**Date**: 2026-09-22
**Spec**: none — dependency bump
**Status**: PASSED

### Summary

Ten library releases since v0.1.29. Nothing in this program needed an edit
except one test, whose assertion the library deliberately changed:

- **0.1.38, `Shell.Select`** — an unknown section title is now ignored rather
  than navigating to the first section (fynedesygn spec 028). `Options.Section`
  keeps the old answer, so `--section nope` still opens the first section.
  `gui_test.go` asserted the fallback through `Select`, which is the wrong
  door; the test now checks both questions separately, and the `--section`
  contract is unchanged.
- **0.1.39, top navigation** — a navigation along the top is drawn inside the
  header instead of a strip below it. This program offers `NavTop`
  (`app.go:430`), so a user on that placement gets a window one row shorter.
  `NavLeft`, the default, is unchanged.
- **0.1.39, dialogs** — every dialog closes from its corner. Free, no call site.
- The rest is markdown table rendering, an image cache, a swatch widget, glance
  windows and profiling. Nothing this program imports changed shape.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages ok, 0 failing.
- One test updated: `TestSectionSelectionResolvesNamesAndFallsBackToTheFirst`
  now asserts that `Select` on a missing name leaves the reader where they are,
  and separately that `Options.Section` with a typo opens the first section.
  Both halves of the library's new contract, exercised through this program's
  own shell options.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- Change is `go.mod`, `go.sum` and one test function. No production code.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- Ten releases of a library already in the tree. Added surface is `imagecache`,
  `widgets.Swatch`, `glance` and `profiling`, none of which this program
  imports. No new network path, no new external command, no credential
  handling.
- No hardcoded secrets in the diff.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit, or reinstall the previous release from the
  AUR or the GitHub artifacts. No on-disk format change; config, history and
  blacklist are untouched.

### Not verified

The top-navigation and corner-close changes are library behaviour pinned by
the library's own tests; they were not driven by hand in this program's window.

### Overall

**PASSED.**
