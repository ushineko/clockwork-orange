## Validation Report: fynedesygn v0.1.84 → v0.1.103

**Date**: 2026-10-17
**Spec**: none — dependency bump
**Status**: PASSED

### Summary

Nineteen library releases, no breaking change to a package this program
imports (`fynedesygn`, `dialogs`, `dragout`, `fynetest`, `logpane`,
`markdown`, `shell`, `table`, `theme`, `widgets`). Most of the range is
`glance` and the new `wizard` package, neither of which is imported here.
What reaches this program, with no code change:

- **0.1.93, links in a `markdown.Pane` work.** The About section renders the
  README in one: `#fragment` links now scroll to the heading, `http(s)`
  links open in the browser, other links draw as plain text.
- **0.1.94, the log pane's line counter no longer widens the pane.** Affects
  the Service log and the plugin log panes.
- **0.1.96, a markdown table column is never narrower than its longest word.**
- **0.1.90, `golang.org/x/net` v0.60.0** (GO-2026-6612), carried into this
  module's `go.sum`.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages ok, 0 failing.
- No new tests: no behaviour of this program changes beyond the library's own
  fixes, which the library tests.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- Change is `go.mod` and `go.sum` only.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- No source change; no new network path, external command or credential
  handling.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit, or reinstall the previous release. No
  on-disk format change.

### Not verified

- The README links in the About section were not clicked in a live window.
- Windows and macOS were not run by hand; CI covers the builds.

### Overall

**PASSED.**
