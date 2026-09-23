## Validation Report: Spec 021 — the plugins fold, and a local build says so

**Date**: 2026-09-23
**Spec**: specs/021-plugins-fold-and-a-dev-version.md
**Status**: PASSED

### Summary

Two changes.

The plugin sections are folded under a `Plugins` heading. The folding is
fynedesygn's (its spec 031, released in v0.1.40 and reported from here); this
program declares one `shell.NavGroup` built from `core.AvailablePluginNames`,
the same list `sectionTitles` walks, so the members cannot drift from the
sections. The library dependency moves v0.1.39 → v0.1.40.

A build that is not the release now reports `<version>-<commit>-dev`. `.tag`
was stamped unconditionally, so a window built from a working tree said
`v4.3.2` exactly as the packaged one did. Taken from hotaru, which had the
same problem.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages ok, 0 failing.
- Two new tests, both about the seam between the group and the sections rather
  than about the library's drawing:
  - `TestThePluginGroupNamesEveryPluginSectionAndNothingElse` — the members are
    exactly the plugin titles, every member is a section, and `Plugins` is not
    among the section names. A member that names no section draws nothing, so
    a typo would quietly unfold the group and no other test would notice.
  - `TestThePluginSectionsAreContiguous` — the library draws the list in
    `Sections` order and puts the heading where the first member would have
    been, so a section that slipped between two plugins would be drawn under
    the heading.
- `tests/parity` unchanged and green: no operation was added, and the
  navigation is not an operation.
- The version rule was verified by running it both ways: `make build` on a
  dirty tree sitting on the v4.3.2 tag reports `4.3.2-998413a-dev`, and
  `git describe --exact-match --tags --match v4.3.2 HEAD` resolves, so a clean
  tree at the tag reports `4.3.2`.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- One helper (`pluginGroup`) and one line in `shellOptions`. The Makefile
  gains three variables and loses none.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- Dependency moves one release, to a version whose only change is the
  navigation feature this spec asked for. No new network path, no new external
  command, no credential handling, no path interpolation.
- No hardcoded secrets in the diff.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit and reinstall the previous release. No
  on-disk format change. The group's open/closed state is one added key in the
  window's own `gui-settings.json`, written by the shell and ignored by an
  older build.
- **A packaging check worth naming**: both PKGBUILDs now pass `VERSION`
  explicitly. Without it `clockwork-orange-git` — which builds from a checkout
  that is never sitting on a tag — would have shipped a `-dev` string in an
  installed package, which is the opposite of what the change is for.

### Not verified

The drawing itself is pinned by the library's tests. It was driven by hand in
this window in all four navigation shapes before the change was committed, but
this repository has no screenshot test that would catch a regression in it.

### Overall

**PASSED.**
