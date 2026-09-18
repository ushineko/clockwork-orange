## Validation Report: Spec 012 — Adopt fynedesygn

**Date**: 2026-09-17 18:30
**Spec**: specs/012-adopt-fynedesygn.md
**Commits**: 5f35cfb (leaves), aed5137 (shell), e7db4d1 (docs), plus this
report and the reconciled spec; branch `adopt-fynedesygn` on 2fafabf (PR #7)
**Status**: PASSED (AC9, the manual desktop run, remains for the author)

### Summary

`internal/gui` carried 3,139 lines of design system copied by hand from
angou and nmsbonker, plus this project's own markdown pane. All of it now
comes from `github.com/ushineko/fynedesygn`:

1. Leaves (5f35cfb): `theme.go`, `fonts.go`, `cursor_*.go`, `views_table.go`,
   `dialogs.go`, `logpane.go`, `markdownpane.go` and the small widgets in
   `app.go` are deleted in favour of `fynedesygn/theme`, `widgets`, `table`,
   `dialogs`, `logpane` and `markdown`. `Status` is `fynedesygn.Status`. The
   appearance uses the same preference keys, so saved choices survive; the
   console font stays in the YAML and reaches the theme as `Options.Mono`.
2. Shell (aed5137): the window skeleton, busy indicator, banners, redraw
   ladder and runner in `app.go`/`state.go` and `restart_*.go` are deleted;
   `ui` holds a `*shell.Shell` and describes the program through
   `shell.Options` (sections, status bar, OnStart, OnInvalidate, OnStop,
   AlsoWorking).
3. Docs (e7db4d1): architecture.md, .claude/CLAUDE.md and the README point at
   the library.

Line count of `internal/gui`, non-test files: **6,135 before, 3,707 after**
(tests: 1,313 before, 1,058 after). `wc -l` over `internal/gui/*.go`
excluding `_test.go`.

### Phase 3: Tests

- Test suite: `make test` (`go test -race -tags parity ./...`)
- Results: all packages ok (cli, config, core, engine, gui, imaging,
  platform, plugins, store, tests/parity), 0 failing, after each of the three
  code commits.
- New or changed tests in `internal/gui`:
  - `TestASavedAppearanceFromThePreviousBuildIsReadUnchanged` (AC5): writes
    the four `appearance.*` keys by their literal names into the test app's
    preferences and asserts the theme and scale a fresh shell reads.
  - `TestConsoleFontAndSizeFromTheDocumentReachTheThemeAndThePanes` (AC6):
    installs a probe font family into a temp directory through
    `fdtheme.RescanFonts`, names it in the document, and asserts the theme's
    monospace face and the log pane's row size (13) come from the document.
  - `TestAboutRendersTheReadmeWithNothingThatTakesTheWheel`: the About
    section renders the README through `markdown.Pane` and
    `fynetest.ScrollableIn` finds nothing scrollable in it.
  - `TestEverySectionRendersHeadlessly` now also builds every section in all
    nine schemes.
  - `TestSectionSelectionResolvesNamesAndFallsBackToTheFirst` and
    `TestFlashShowsOneBannerAtATime` are rewritten over the shell's API.
- Deleted tests (they pinned the copied code's internals, which the library
  tests itself): the markdown pane suite (`markdownpane_test.go`, including
  the `TestFyneStillDrawsMarkdownCodeInsideAScroll` canary), `followTail`,
  `logModel` cap/replace, `detailTable` thumbnails, the chooser tap test, the
  `FYNE_SCALE` test and the flash-hold timings.
- Lint: `make lint` (golangci-lint v2.12.2, pinned config, GOTOOLCHAIN
  go1.26.0) — 0 issues after each commit. `go vet ./...` clean.
- Status: PASSED

### Phase 4: Code Quality

- Dead code: none left. Every helper the deleted files provided has a library
  replacement or was removed with its only caller (`chooseFolder`,
  `levelStatus`, `scaleLabels`, `textSizes`, `scaleChoices`); `unused` in the
  lint run agrees.
- Duplication: the `u.sh = s` assignment at the top of every builder, the
  status bar callback and the two hooks is repeated on purpose (Gaps found 3):
  the shell calls them before `New` returns the pointer. It is one line in
  `sections` for all builders.
- Encapsulation: `app.go` is 520 lines including doc comments (was 1,046);
  `state.go` 321 (was 406). The longest new function is `sectionBuilders`.
- Refactorings: none beyond the spec's scope.
- Status: PASSED

### Phase 5: Security Review

- Dependency scan: `govulncheck@v1.1.4` (Go go1.27.1), `-mode binary` on
  `bin/clockwork-orange` and `bin/clockwork-orange-gui` built from aed5137:
  no vulnerabilities found in either. Source mode not run: the installed
  scanner is built for an older toolchain than the local one (same limitation
  as the spec 011 report; no tool was installed).
- New dependency: `github.com/ushineko/fynedesygn` at commit a7673fa (same
  author, MIT), pulled through the module proxy; `go.sum` records it. It
  raised `golang.org/x/net` from v0.56.0 to v0.59.0 through `go mod tidy`;
  no other module changed. No `replace` directive.
- OWASP Top 10, AI-assisted best-effort: the diff is GUI composition. The one
  subprocess the window starts (`xdg-open` on a path from the program's own
  settings) moved into `dialogs.OpenPath`, which passes the path as a single
  argument under `context.Background` as before. No SQL, network or new
  filesystem writes; the font scanner reads font files only, as the copy did.
- Secrets: none in the diff; the Wallhaven API key field is still a password
  entry and is never logged.
- Status: PASSED

### Phase 5.5: Release Safety

- Change type: code (GUI presentation and window skeleton), documentation.
- Blast radius: `internal/gui`, `go.mod`/`go.sum`, docs. No `internal/core`
  operation changed, so CLI/GUI parity holds (`tests/parity` passes). No
  on-disk format touched: the YAML keys, `history.db`, `blacklist.db` and the
  `appearance.*` preference keys are unchanged (AC5 test). `appearance.mono`
  is now written as `"Fyne default"` by the library's Save; the previous
  build ignores unknown keys.
- Rollback: revert the merge commit and reinstall (`./install.sh`) or
  install the previous release; the previous binary reads the same
  preferences and YAML. Per `release-safety/simplified.md`.
- Status: PASSED

### Overall

- All gates passed: YES, with AC9 (the manual run: sections opened, a plugin
  run streaming into Activity, a banner, hide to tray and back) left for the
  author on a desktop.
- Notes:
  - "Gaps found" in the spec lists four library gaps handled in the program
    (theme hook for the console font, `Save` writing `appearance.mono`,
    `shell.New` calling back before it returns, no typed-key hook beside F5)
    and the display differences adopted from the library (`MiB` units,
    "failed:" wording in error banners, `1.2x` scale labels, the note shape).
  - The startup banner for an unreadable configuration is now shown from
    `OnStart`, before the window is visible. Headless tests cannot check
    where the popup lands on a canvas that has not been shown yet; AC9 covers
    it if a bad config is tried.
  - `go.mod` names the pseudo-version of a7673fa; AC2's `v0.1.0` is the tag
    that commit is to receive, and the bump is a one-line follow-up.

## Addendum 2026-09-17: manual run (AC9, partial)

The adopted GUI (`make build-gui`, pinned to fynedesygn v0.1.1) was run on
this KDE Plasma 6 Wayland machine through fynedesygn's `tools/screenshot.sh`
with `--home` (throwaway HOME and XDG directories) and `CLOCKWORK_LOCK_DIR`
pointed at a temporary directory, so the run could not touch the real
configuration or collide with the running instance. Sections captured and
checked by eye: Service (live systemd status, journal log pane with Follow
and Copy, control buttons), Settings (form, auto-save "Saved" banner floating
over the content), Appearance, Blacklist, About (README pane). The status bar
showed mode, interval, plugins and service state. Not exercised: a plugin run
streaming into a pane, hide-to-tray and return. AC9 stays open for those.

The first attempt photographed the developer's own running instance: a
class-only window search found it first. The harness now matches the pid of
the process it launched (fynedesygn commit after v0.1.1).
