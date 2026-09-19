# Spec 013: Adopt fynedesygn 0.1.25

> **Note**: This work has no associated issue tracker ticket (personal public
> GitHub repository, no tracker). Consider creating a GitHub issue for
> traceability.

## Status: COMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Medium
- **Branch**: `chore/fynedesygn-0.1.25`
- **Predecessor**: spec 012 (the adoption itself, at v0.1.0/0.1.2)

## Executive Summary

The GUI moves from fynedesygn v0.1.2 to v0.1.25 and takes up what the library
grew over those twenty-three releases. The bump alone is behaviour-preserving
and fixes three live bugs (a result banner and a hover tip each took every
click in the window for as long as they were up; a log pane drew long lines
past its right edge; a banner over a log was unreadable). On top of it: the
window's own settings move into `~/.config/clockwork-orange/gui-settings.json`
beside the SQLite stores, with the appearance migrated out of Fyne's preference
store by the shell; the navigation gains all three shapes, both placements and
`Ctrl+B`; the log pane in Service and Activity sits under a divider the user
can drag; the "something is already running" refusal names what is running and
offers its Cancel; six controls gained hover notes; and Blacklist and History
refetch on arrival rather than showing what they last read.

Three further fixes came out of looking at the built window (R10 to R12): a
mark per plugin section instead of one picture icon for all three, the plugin
Configuration tab's actions pinned so they cannot scroll below the fold, and
the Appearance section's four notes wrapped — they were `widgets.Dim`, which
does not wrap, so the section was as wide as its longest unbroken line.

Reviewers should look first at `internal/gui/app.go` (the settings path, the
nav declaration, the arrival hooks) and at `internal/gui/views_service.go`,
whose section became a split. Nothing moved out of `clockwork-orange.yml`, so
on-disk compatibility with 2.9.x and the daemon is unchanged.

## Context

Spec 012 replaced `internal/gui`'s hand-copied design system with
`github.com/ushineko/fynedesygn`, pinned at v0.1.0 and moved to v0.1.2 by the
Appearance-wrapping fix in v4.1.1. The library has since reached v0.1.25 over
twenty-three releases driven by the other two adopters (angou, nmsbonker) and
by terrariabonker's port.

Nothing in those releases is a breaking change for this program: the bump
alone builds and passes `make test` with no source edit. But the window uses
none of what was added, and three of the fixes are for bugs this window still
has:

- A result banner and a hover tip were overlays, and an overlay takes every
  click in the window. For six to twelve seconds after every operation this
  window's banner swallowed the first click aimed at anything else
  (library 0.1.13).
- A log pane drew long lines past its right edge with no way to read the rest.
  The Service journal and every plugin run log are such panes
  (library 0.1.16 to 0.1.19).
- A banner over a log pane was unreadable, because the status tint is
  translucent and had nothing but the log underneath it (library 0.1.15).

Those arrive with the version bump. This spec is about the rest: the
capabilities the library grew that this program should now be using rather
than the shapes it hand-rolled in their place.

Where the library's API and this program's arrangement disagree, the program
changes to the library's shape. Anything the program needs that the library
lacks is recorded in "Gaps found" for a library spec, not worked around here
(project rule: a shape a second section needs belongs in the library).

## Requirements

### R1. Dependency

- R1.1 `go.mod` requires `github.com/ushineko/fynedesygn v0.1.25`. Fyne stays
  where it is; `go mod tidy` carries whatever the library raised.

### R2. Settings file (library 0.1.10)

- R2.1 `shell.Options.SettingsPath` is
  `filepath.Join(config.StateDir(), "gui-settings.json")` — the directory this
  program already owns, beside `history.db` and `blacklist.db`, rather than a
  second directory named for the app ID.
- R2.2 The colour scheme, font, text size and interface scale move out of
  Fyne's preference store into that file. The shell does the migration: it
  reads the preference store once for an installation that predates the file.
  No code in this program reads `fyne.Preferences` for the appearance.
- R2.3 Everything the CLI can also see stays in `~/.config/clockwork-orange.yml`
  — the window size (`window_width`/`window_height`), the console font and its
  size, and every plugin block. The settings file holds only what belongs to
  this window and nothing the daemon or 2.9.x reads. On-disk compatibility is
  unchanged.

### R3. Navigation shape (library 0.1.10, its spec 013)

- R3.1 `shell.Options.NavModes` lists all three modes (labels, icons, hidden)
  and `NavPlacements` both placements (left, top), which puts the stock shape
  control in the header and binds `Ctrl+B`.
- R3.2 The program's own key handler (`onTypedKey`, the review's arrows and
  Space) is unchanged and does not bind `Ctrl+B`.
- R3.3 The chosen shape is remembered across runs, in the file from R2.1.

### R4. Dividers the user can drag (library 0.1.10, its spec 012)

- R4.1 The Service section puts its status and control cards over the journal
  pane in `Shell.VSplit("log", …)`, so the user decides how much of the
  section is log. The pane's fixed height becomes its minimum.
- R4.2 The Activity section (Windows, macOS) uses the same key, so the log
  does not jump when the first section differs by platform.
- R4.3 The navigation's own divider is the shell's and comes with R1.1; the
  width of the section list is now something the user can keep.
- R4.4 Nothing transient reflows: banners and the busy indicator still float.
  A divider is not transient — the user put it there and it outlives the
  rebuild.

### R5. A refusal that says what is running (library 0.1.20, 0.1.21)

- R5.1 The two `Flash("Something is already running…")` calls in `rundialog.go`
  become `Shell.SayBusy()`, which names the operation holding the indicator and
  offers its Cancel when it has one, and says nothing while the modal busy
  popup is already on screen saying it.

### R6. Hover tips (library 0.1.6)

- R6.1 `widgets.WithTip` carries the guidance a control's label cannot fit, on
  the controls whose consequence is not evident from the label: Service
  Install and Uninstall, History's two actions, Blacklist's Remove, and the
  Appearance scale picker.
- R6.2 Tips add to the sections' existing notes; no note is deleted to make
  room for one. A tip is read by whoever hovers, a note by whoever reads the
  section.

### R7. Arrival (library 0.1.5)

- R7.1 Blacklist and History refetch when the navigation arrives at them
  (`FuncSection.OnArrive`), rather than only when their loaded flag happens to
  be false. A plugin run that blacklists an image no longer leaves a stale
  table behind when the user walks over to look at it.
- R7.2 Arrival is distinct from a rebuild: a section rebuilt where it stands
  (a refresh, a status redraw, a filter keystroke) does not refetch, which is
  what would loop.

### R8. Tests and guards

- R8.1 `make test` and `make lint` pass.
- R8.2 A test pins the settings path to R2.1's location and that the appearance
  survives a restart through it.
- R8.3 A test pins that the shape control is offered (all three modes
  declared) and that a chosen shape is read back.
- R8.4 A test pins that arriving at Blacklist refetches and that rebuilding it
  in place does not.
- R8.5 `tests/parity` is unaffected: no operation is added or removed.

### R9. Documentation

- R9.1 `.claude/CLAUDE.md`'s design reference paragraph is unchanged; it
  already names the library as the rulebook.
- R9.2 This spec's "Gaps found" is the hand-off to the library.

### R10. A mark per plugin section

Found while looking at the built window: all three plugin sections drew
`theme.FileImageIcon`, so Local, Wallhaven and DuckDuckGo Images were
indistinguishable in the navigation — and R3.1 has just made icons-only a shape
the user can choose, where the mark is nearly all there is.

- R10.1 Each plugin section draws its own mark: a folder for the directory
  Local reads, a wall of tiles for the gallery Wallhaven downloads from, a
  magnifier for the search DuckDuckGo Images runs.
- R10.2 The marks are this repository's own drawings, embedded from
  `internal/gui/assets`, and are themed so they recolour with the scheme.
- R10.3 They are filled silhouettes and never stroked, and use only the
  attributes Fyne's SVG recolouring understands. Fyne replaces an SVG's fills
  and leaves its strokes as authored, and it re-marshals the document, so a
  stroke keeps the colour it was drawn in and an unknown attribute is dropped.
- R10.4 A plugin this build has no drawing for keeps the generic picture icon.

### R11. Actions that do not scroll away

- R11.1 The plugin section's Configuration tab pins its Actions card to the
  bottom and scrolls the form under it:
  `Border(nil, actions, nil, nil, VScroll(form))`, the shape the design system
  already prescribes. Wallhaven has eleven fields, and with the actions last in
  one column the two buttons the section exists for sat below the fold.
- R11.2 The Review tab's controls already sit in a `Border` top strip and are
  unchanged.

### R12. Paragraphs wrap

- R12.1 The Appearance section's four notes are `widgets.DimWrapped`, not
  `widgets.Dim`. `Dim` does not wrap, so a paragraph in one set the section's
  minimum width to the whole unbroken line and the shell's scroller scrolled
  sideways rather than reflowing — the same bug v4.1.1 fixed in the library's
  own Appearance section, in this program's copy, which was missed.
- R12.2 A test holds every section to the rule, so the next long note added in
  a `Dim` fails the build rather than the window.
- R12.3 The first note's text is corrected: the appearance is in the settings
  file from R2, not in Fyne's preference store.

### R13. A successful save says nothing

- R13.1 `performSave` no longer flashes "Saved" or sends the "Configuration
  saved" desktop notification. Every edit in every form arms the auto-save, so
  a user working through a form was told a second after each field — a banner
  over the section they were reading and a notification on the desktop — about
  the one thing they had just asked for and could see had happened.
- R13.2 A **failed** save still reports, as a `StatusBad` banner. That is the
  one the user cannot see for themselves, and silence there would lose an edit
  without saying so.
- R13.3 Operation results elsewhere (`Shell.OK` after a service start, a
  blacklist removal, a history import) are unchanged: those are outcomes the
  user asked for and cannot otherwise confirm.

## Acceptance Criteria

- [x] AC1 `go.mod` is at `fynedesygn v0.1.25` and `go mod tidy` is clean (R1.1)
  - Verified: go.mod is at v0.1.25 and `go mod tidy` leaves only that bump
- [x] AC2 `shell.Options.SettingsPath` is `~/.config/clockwork-orange/gui-settings.json` (R2.1)
  - Verified: `TestTheSettingsFileLivesBesideTheStores`
- [x] AC3 No code in `internal/gui` reads the appearance from `fyne.Preferences` (R2.2)
  - Verified: `TestAnAppearanceChosenInOneRunIsInEffectInTheNext`, `TestTheThemeComesFromTheAppearanceTheShellHolds`; the one `Preferences()` left in the tree is the migration test
- [x] AC4 The window size, console font and console size are still written to `clockwork-orange.yml`, and a 2.9.x-written file still round-trips (R2.3)
  - Verified: `TestTheSettingsFileHoldsNothingTheCLIReads`, plus the spec 012 round-trip tests
- [x] AC5 All three nav modes and both placements are declared, and the header shows the shape control (R3.1)
  - Verified: `TestTheNavigationShapeIsOfferedAndRemembered`
- [x] AC6 `Ctrl+B` hides and shows the navigation; the review's keys still work on the Review tab (R3.2)
  - Verified: `TestTheProgramsKeyHandlerDoesNotBindCtrlB`; the binding itself is the shell's, raised because this program declares `NavHidden`, and the review's keys are covered by `review_test.go`. Live confirmation in a real window is manual
- [x] AC7 A nav shape chosen in one run is in effect in the next (R3.3)
  - Verified: `TestTheNavigationShapeIsOfferedAndRemembered`
- [x] AC8 The Service journal sits under a divider the user can drag, and the position survives a section rebuild and a restart (R4.1, R4.3)
  - Verified: `TestTheLogPaneSitsUnderASharedDivider`, `TestTheServiceSectionsControlsSurviveTheSplit`; the across-runs half is the shell's own (`shell/split_test.go`)
- [x] AC9 Activity uses the same divider key as Service (R4.2)
  - Verified: `TestTheLogPaneSitsUnderASharedDivider` — one key, one position
- [x] AC10 Neither `rundialog.go` site flashes "Something is already running"; both call `SayBusy` (R5.1)
  - Verified: neither refusal site in `rundialog.go` flashes; both call `SayBusy` (the one `Flash` left in that file reports a failed plugin run, which is a different thing)
- [x] AC11 The six controls in R6.1 carry tips, and no existing note was removed (R6.1, R6.2)
  - Verified: `TestTheControlsThatNeedOneCarryATip`
- [x] AC12 Arriving at Blacklist or History refetches; a rebuild in place does not (R7.1, R7.2)
  - Verified: `TestArrivingAtTheBlacklistReadsItAgainAndARebuildDoesNot`, `TestArrivingAtHistoryReadsItAgain`
- [x] AC13 `make test` passes (R8.1)
  - Verified: `make test` green (race detector, `-tags parity`)
- [x] AC14 `make lint` is clean (R8.1)
  - Verified: `make lint` — 0 issues
- [x] AC15 Tests exist for AC2/AC7, AC5 and AC12 (R8.2, R8.3, R8.4)
  - Verified: `internal/gui/settings_test.go` and `internal/gui/sections_test.go`
- [x] AC16 `tests/parity` passes unchanged (R8.5)
  - Verified: `tests/parity` green, unchanged
- [x] AC18 Each plugin section draws a different mark, and a plugin with no drawing keeps the generic icon (R10)
  - Verified: `TestEachPluginSectionHasItsOwnIcon`, `TestAnUnknownPluginKeepsTheGenericIcon`
- [x] AC19 The plugin Configuration tab's actions are outside its scroller (R11.1)
  - Verified: `TestThePluginActionsDoNotScrollAway` — the buttons are in the section and in no `container.Scroll` in it
- [x] AC20 No section carries a long label that does not wrap (R12)
  - Verified: `TestNoSectionHasAnUnwrappedParagraph`, which fails on the pre-fix Appearance section with "a 297-character note does not wrap"
- [x] AC21 A successful save shows no banner and sends no notification; a failed one still reports (R13)
  - Verified: `performSave` in `internal/gui/state.go` — the success path does no reporting, the error path still flashes `StatusBad`. Not covered by a test: the reporting branch was behind `Shell.OnScreen()`, which is false under the headless test driver, so a headless assertion would pass whatever the code did
- [x] AC17 `govulncheck ./...` is clean (security extension)
  - Verified: `govulncheck ./...` — no vulnerabilities found (govulncheck v1.8.0, DB 2026-09-16)

## Risks & Assumptions

- **Rollback**: revert the commit; the previous release is installable from
  the AUR/GitHub artifacts. The one thing a revert does not undo is the
  settings file written under R2.1 — a 4.1.1 binary ignores it and reads the
  appearance from the Fyne preference store, which is still there and still
  holds the pre-migration values, so the window comes back looking as it did.
- **On-disk compatibility**: R2.3 is the load-bearing constraint. The settings
  file is new and private to this window; nothing moves out of
  `clockwork-orange.yml`, so 2.9.x and the daemon are unaffected. AC4 is the
  check.
- **A new file in the state directory**: `gui-settings.json` joins the two
  SQLite stores in `~/.config/clockwork-orange/`. The 2.9.x line never reads
  that directory's contents by listing it, so an unknown file there is inert.
- **A new section shape**: R11 makes the plugin Configuration tab a `Border`
  with an inner scroller, so that tab now has a scroller inside the shell's
  own. The design system prescribes exactly this for a pinned action strip and
  the Appearance section already does it, so it is the sanctioned nesting
  rather than a new one.
- **Layout change**: R4.1 changes how the Service section is proportioned on
  first run. It is the section's own arrangement, not a data change, and the
  divider's default offset reproduces roughly what the fixed height gave.
- **Platform**: the nav placement and the divider are drawn by the library and
  are covered by its own tests on all three platforms; this program's
  platform-specific files are untouched.

## Gaps found

*(for a fynedesygn spec, not to be worked around here)*

- **A rubric: a control that starts work never scrolls out of view.** The
  design system has this rule in a narrow form — "Sections whose bottom action
  strip must stay visible use `Border(nil, actions, nil, nil, VScroll(body))`"
  — stated as a shape for the sections that already know they need it. R11 is
  the case for stating it as a rule instead, and the plugin section is the
  evidence: it was built as a single column, the shape reads correctly, and the
  actions still went below the fold the moment a plugin had eleven fields. The
  proposed wording, for a library spec:

  > Every control that starts, cancels or commits work is affixed: it occupies
  > the same place in the section however much of the section is scrolled. What
  > scrolls is the material the control acts on — the form, the table, the
  > document — never the control itself. A section with such a control is
  > therefore a `Border` with the controls in a fixed edge and a scroller in
  > the centre, not a column that happens to fit today.

  Worth carrying into the library with a check of the gallery's own sections
  and, if it can be made to work headlessly, a `fynetest` helper that fails a
  section whose buttons are inside its scroller — the shape of the test in
  `TestThePluginActionsDoNotScrollAway`.

- **A structural walk that descends a split.** `fynetest.Walk` opens a
  `container.Scroll` and a tab set, because a section's content would otherwise
  be invisible to a test — and stops at a `container.Split`, which is a widget
  with two exported halves. Every section built with the library's own
  `Shell.VSplit` is therefore invisible to the library's own non-rendered
  walk; this program's Service section became so the moment R4.1 landed, and
  the local test helper in `gui_test.go` grew a `case *container.Split` to see
  past it. `fynetest.Tips` already descends one, through the renderer, so the
  two walkers disagree about the same tree.

- **A table whose rows can be ticked.** `widgets.PickList` (library 0.1.22 to
  0.1.24) is exactly the shape the Blacklist section needs — several rows
  picked, one action, one confirmation — but it is built on `widget.List` and
  the Blacklist is a `table.Detail`: five columns and a thumbnail column. The
  section therefore keeps its hand-rolled tick: a first column holding "✓" or
  a space, an `OnSelected` that toggles the row's hash and immediately
  unselects, and a `map[string]bool` on `*ui`. That is a shape a second
  section would copy, so it belongs in the library — a tick column on
  `table.Detail` with `Picked`/`ClearPicks` in `PickList`'s vocabulary. The
  table picks by key, not by row number, because the Blacklist's rows are
  filtered live and row three of a filtered table is a different image.

## Alternatives Considered

- **Bump only, adopt nothing.** Rejected: the bump's free fixes are worth
  having on their own, but leaving the settings, nav shape and splits unused
  means the next adoption is twenty-three releases plus whatever lands next,
  which is how the hand-copied design system got to 3,139 lines.
- **Move the window size into the settings file too.** Rejected: 2.9.x reads
  `window_width`/`window_height` from `clockwork-orange.yml` and the project's
  on-disk compatibility rule is binding.
- **Convert the Blacklist table to a `PickList`.** Rejected: it would cost the
  columns and the thumbnails. Recorded as a gap instead.
