# Spec 012: Adopt fynedesygn

> **Note**: This work has no associated issue tracker ticket. The repository
> is a personal public project without an issue tracker.

## Status: INCOMPLETE

## Executive Summary

`internal/gui` now imports `github.com/ushineko/fynedesygn` for its design
system and runs the window on the library's shell; the 3,139 hand-copied
lines (theme, fonts, cursor, shell, runner, table, dialogs, log pane,
Appearance plumbing) and this project's own markdown pane are deleted, and
`internal/gui` non-test code goes from 6,135 to 3,707 lines. Behaviour is
kept: the same preference keys, the console font from the YAML, the same
sections and operations. Reviewers should start with `app.go` (the `ui`
struct, `sections`, `shellOptions`, `onStart`) and the "Gaps found" list
below, which records the four places the library's API made the program bend.

## Context

`internal/gui` carries 3,139 lines of design system copied from angou and
nmsbonker under "keep in sync by hand" headers (theme, fonts, cursor, the
shell in `app.go`, the plumbing in `state.go`, the detail table, the
dialogs, the log pane, Appearance and About), plus this project's own
`markdownpane.go`. Every one of those now lives in
`github.com/ushineko/fynedesygn` (specs 001 to 005 there), which was
extracted from these three programs with this one as the first adopter.

The adoption replaces the copies with imports and keeps the window's
behaviour identical. Where the library's API differs from the copy, the
program changes to the library's shape; the library does not grow
program-specific switches. Anything the program needs that the library
lacks is recorded in "Gaps found" for a library spec, not hacked around.

This branch stacks on `fix/about-section-scrolling` (PR #7), because it
deletes `markdownpane.go`, which that PR adds. PR #7 merges first.

## Requirements

### R1. Dependency

- R1.1 `go.mod` requires `github.com/ushineko/fynedesygn v0.1.0`. Fyne
  stays at v2.8.1. The `golang.org/x/*` versions the library raised above
  Fyne's pins carry over through `go mod tidy`.

### R2. Leaves (first commit)

Replace, keeping behaviour:

- R2.1 `theme.go`, `fonts.go`, `cursor_linux.go`, `cursor_other.go` with
  `fynedesygn/theme` (`fdtheme`). The console font and size stay in the
  YAML document: `theme.Options.Mono` is loaded from
  `consoleFamily(doc)`; the appearance triple moves to `fdtheme.Appearance`
  with the same preference keys (`appearance.scheme`, `appearance.font`,
  `appearance.textSize`, `appearance.scale`), so a user's saved choices
  survive. `appearance.mono` is left unset because this program keeps the
  console font in its YAML.
- R2.2 `views_table.go` with `fynedesygn/table` (`table.Detail`,
  `SetThumbnails`).
- R2.3 The small widgets in `app.go` (`dim`, `sep`, `statusText`, `marker`,
  `wrapped`, `heading`, `note`, `card`, `humanSize`, `orNone`) and
  `fixedHeight`/`fixedWidth` with `fynedesygn/widgets`; `Status` and its
  constants with `fynedesygn.Status` (`fd.StatusInfo` and so on); the
  domain rankers in `model.go` stay and return `fd.Status`.
- R2.4 `dialogs.go` with `fynedesygn/dialogs` (`ConfirmDestructive`,
  `WithBrowse`, `BrowseButton`, `ChooseFolder`, `OpenPath` with the
  program's own banner on error, `PickerStart`).
- R2.5 `logpane.go` with `fynedesygn/logpane`: `events.Level` maps to
  `logpane.Level` in one function; the panes are fed through
  `Pane.Log`/`Model.Append`; `Options.TextSize` carries the console size;
  `Options.Clipboard` and `Options.Flash` come from the shell.
- R2.6 `markdownpane.go` with `fynedesygn/markdown` (`markdown.New`,
  `Follow`, `Detach`); the About README goes through it with `Options{}`.
- R2.7 `views_appearance.go` keeps its console font and size controls and
  the scale restart; the scheme, font and text-size pickers and the sample
  come from the library (`fdtheme.Sample`), or the section is rebuilt on
  `shell.AppearanceSection` plus a program card for the console font. Either
  keeps every control the section has today.

### R3. Shell (second commit)

- R3.1 `ui` keeps the program's state (document, deps, loaded pairs, the
  timer, the review model, the run state, tray and instance handles) and
  holds a `*shell.Shell`; the shell code in `app.go` (nav, header, status
  bar frame, busy, flash, swap, detach, redraw ladder) and the runner in
  `state.go` (`perform`, `performCancellable`, `report`, `ok`,
  `invalidate`, `onScreen`, `working`, `gate`, `regate`) are deleted in
  favour of the shell's.
- R3.2 Sections become `shell.Section`s built from the existing `buildX`
  methods (`shell.NewSection(title, icon, func(*shell.Shell) ...)`), with
  `OnDetach` for Activity (log pane), the review model and the README pane.
  Plugin sections keep their dynamic titles. `SectionNames()` and
  `SchemeNames()` stay exported for the flag help and the parity test,
  computed from `shell.Names` and `fdtheme.SchemeNames`.
- R3.3 `Options.Header` supplies nothing beyond Refresh; `Options.StatusBar`
  returns the segments `statusBar()` builds today; `Options.OnStart` runs
  `loadService`, `startPolling`, `timer.start`, tray, the instance listener,
  the close intercept and the start notification; `Options.OnInvalidate`
  drops the loaded flags and reloads; `Options.OnStop` flushes the save and
  releases the lock (used by `Restart`); `Options.AlsoWorking` reports
  `u.running`.
- R3.4 Window geometry: `Options.Size` from `windowSize(doc, exists)`; the
  size poll and `noteSize` stay in the program.
- R3.5 `Run` keeps the GUI lock and the second-launch handshake before
  `shell.New`, and the `shutdown` after `ShowAndRun`.
- R3.6 `restart_unix.go`/`restart_windows.go` are deleted in favour of
  `Shell.Restart` with `OnStop`.

### R4. Tests and guards

- R4.1 Every test in `internal/gui` passes, adjusted to the new types;
  `tests/parity` passes; the headless `testUI` builds a `shell.Headless`
  shell.
- R4.2 The library's canaries cover the Fyne quirks this package tested
  itself (`TestFyneStillDrawsMarkdownCodeInsideAScroll` and the markdown
  pane tests move to the library and are deleted here; a smoke test that the
  About section renders with the README stays).
- R4.3 `make test`, `make lint`, `govulncheck` (binary mode) clean; the
  gallery of this program's sections renders headlessly in every scheme
  (extend `TestEverySectionRendersHeadlessly` over `fdtheme.SchemeNames`).

### R5. Documentation

- R5.1 `docs/architecture.md` "internal/gui" and "Design language" point at
  the library and its `docs/design-system.md`; the design-language table
  names library packages. `.claude/CLAUDE.md` names the library as the
  design reference in place of nmsbonker.
- R5.2 README "Appearance" mentions the new Windows and macOS schemes.
- R5.3 "Gaps found" below lists anything the library must gain; each becomes
  a library spec item.

## Acceptance Criteria

- [x] AC1 No file under `internal/gui` carries a "Copied from" header;
  `theme.go`, `fonts.go`, `cursor_*.go`, `views_table.go`, `dialogs.go`,
  `logpane.go`, `markdownpane.go`, `restart_*.go` are gone (R2, R3.6).
- [x] AC2 `go.mod` requires `github.com/ushineko/fynedesygn v0.1.1`; no
  `replace` directive (R1; the spec said v0.1.0, and 0.1.1 is the release
  that carries the hooks this adoption needed).
- [x] AC3 `grep -rn "func (u \*ui) \(flash\|busy\|perform\|report\|ok\|invalidate\|refresh\|rebuild\|redrawStatus\|swap\|detach\|show\|gate\|working\|regate\)(" internal/gui` finds nothing (R3.1).
- [x] AC4 `SectionNames()` returns the same list as before the change, in
  order, without a Fyne app (R3.2).
- [x] AC5 A saved appearance from the previous build (scheme, font, text
  size, scale under the same keys) is read unchanged: a test writes the four
  keys with `test.NewApp`'s preferences and asserts the theme (R2.1).
- [x] AC6 The console font family and size from the YAML reach the log
  panes and the theme's monospace face (R2.1, R2.5).
- [x] AC7 `make test` (with `-tags parity`) and `make lint` pass; every
  section renders headlessly in all nine schemes (R4).
- [x] AC8 `govulncheck -mode binary` on both built binaries: no findings.
- [ ] AC9 The GUI is run on this machine and the Service, Activity, a
  plugin, History, Blacklist, Settings, Appearance and About sections are
  opened; a plugin run streams into the Activity pane; a banner shows; the
  window hides to the tray and comes back (manual, recorded in the report).
  _Partly verified 2026-09-17 with fynedesygn's screenshot harness under a
  throwaway HOME and lock directory: Service (live unit status and journal
  pane following the tail), Settings (with the auto-save "Saved" banner
  floating over the form), Appearance, Blacklist and About rendered as
  before; the status bar carried the program's segments. Not exercised: a
  plugin run streaming into a pane, and hide-to-tray and return. Activity
  does not exist on Linux (it is the Windows and macOS section)._
- [x] AC10 `docs/architecture.md`, `.claude/CLAUDE.md` and README updated;
  "Gaps found" filled in (R5).
- [x] AC11 Line count of `internal/gui` (non-test) is reported before and
  after in the validation report. Before: 6,135 (tests 1,313). After: 3,707
  (tests 1,058). `validation-reports/2026-09-17-1830-spec012-adopt-fynedesygn.md`.

## Risks & Assumptions

- **Assumption**: the library's shell behaves as the copy did in every case
  the tests pin. Where a test reveals a difference, the library is checked
  first: a difference that is a library bug is fixed there (with a library
  test) and the dependency bumped; a difference that is program behaviour
  is adjusted here.
- **Risk**: the Appearance preference keys must not change, or every user
  loses their scheme on upgrade. The keys are asserted by AC5.
- **Risk**: `Restart` semantics: the copy released the GUI lock and flushed
  the save before exec; `OnStop` must do both.
- **Risk**: PR #7 must merge before this PR, or this branch must be rebased
  when it does. (The branch is based on PR #7's head, 2fafabf.)
- **Rollback**: revert the merge commit; the previous binary keeps reading
  the same preferences and YAML.

## Gaps found

Recorded during implementation against the pre-release library, then fixed
in fynedesygn 0.1.1 (its spec 006) and adopted here in the same branch:

1. **No hook for a monospace face from outside the preference store.**
   Resolved by `shell.Options.Theme`; `themeFor` is that hook and the second
   `SetTheme` is gone.
2. **`Appearance.Save` writes `appearance.mono` unconditionally.** Left as
   is in the library: the saved default is stable and nothing here reads it.
3. **`shell.New` used the program's callbacks before returning the shell.**
   Resolved by `shell.Options.OnCreate`; the shell is stored once.
4. **No way to add a typed-key handler beside the shell's F5.** Resolved by
   `shell.Options.OnTypedKey`; the review's keys go through it and the
   program no longer replaces the canvas handler.

Differences adopted rather than worked around (the program changed to the
library's shape, as the Context section says):

- `widgets.HumanSize` prints binary units (`1.2 MiB`) where `humanSize`
  printed `1.2 MB`, history_tab.py's wording. The History section's
  "Database size" now reads in the library's units.
- `Shell.Report` says "*what* failed: *err*" where `report` said "*what*:
  *err*", and `Report`/`OK` are UI-thread calls where the program's hopped
  with `fyne.Do` themselves; the callers that report from a goroutine wrap
  the call.
- `fdtheme.ScaleLabel` prints `1.2x` where the program printed `1.2×`; the
  library avoids the glyph on purpose (design rule: no symbols outside the
  bundled font's coverage).
- `widgets.Note` is a dimmed paragraph indented past the label column (warn
  and bad take the status colour) where `note` drew a marker beside wrapped
  text.
- The busy popup's Cancel button has no icon; the program's had
  `CancelIcon`.
- A failed read of the configuration at startup is now shown as a banner in
  `OnStart`. The previous build flashed it before the banner popup existed,
  so the message was never seen; now it is.
- `logpane.Pane.Log` stamps lines with the library level's name (`INFO`); the
  program keeps its own `[INFO]` wording (events.Level.String) by feeding
  `Model().Append` from `paneEvents`, as R2.5 asked.

## Alternatives Considered

- Considered adopting the leaves only and keeping this program's shell;
  rejected because the shell is where the three copies disagree most and
  the point of the library is one shell.
- Considered a `replace` directive to the local checkout during
  development; rejected for the PR, allowed only in a working tree that is
  never pushed.
