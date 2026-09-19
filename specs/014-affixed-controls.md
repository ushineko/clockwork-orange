# Spec 014: Controls that do not scroll away

> **Note**: This work has no associated issue tracker ticket (personal public
> GitHub repository, no tracker). Consider creating a GitHub issue for
> traceability.

## Status: COMPLETE

- **Priority**: High — a regression in a shipped release (v4.2.0)
- **Estimated Complexity**: Low
- **Branch**: `fix/affixed-controls`
- **Predecessor**: spec 013, which introduced the regression and wrote up the
  rule that would have caught it

## Executive Summary

v4.2.0's Service section put Start, Stop, Restart, Install and Uninstall in the
scrolling half of its split, below a heading, a status line and a fixed-height
details pane, so at the divider's default position they were off the bottom of
it — and because the section's other half is a log pane that takes the scroll
wheel, in a window of ordinary height they could not be scrolled to either. The
window reported that the service was running and offered no way to stop it.

The controls are affixed now, in Service, History and Appearance. The rule
behind it — a control that starts work never scrolls, what scrolls is the
material it acts on — is `gui.AffixedActions`, a per-section list of the
controls that must stay put, and two tests that hold the window to it: one that
fails when a named control is inside a scroller, and one that fails when a
section has neither an entry nor a recorded reason for having none.

Reviewers should look at `AffixedActions` in `internal/gui/app.go` first: the
list is the specification, and the three section changes follow from it.

## Context

Spec 013 R4.1 put the Service section's log under a divider the user can drag
and made the half above it a scroller of its own. What went into that scroller
was the heading, the Status card — which carries a fixed-height 150 px details
pane — and then the Control card holding Start, Stop, Restart, Install and
Uninstall. At the divider's default position the Control card is below the fold
of that scroller. The section reports that the service is running and offers no
visible way to stop it.

It shipped in v4.2.0 and was found by looking at the window.

Below the fold understates it. The half holding the controls is a small
scroller sharing the section with a log pane, and a log pane is a
`widget.List` — itself scrollable, and the larger of the two at any ordinary
window height. A scroll gesture lands where the pointer is, so in a window not
tall enough to show the Control card the log takes the wheel and the controls
cannot be reached at all. The user does not have a hard-to-find button; they
have no button. Which is the argument for the rule rather than for a bigger
default divider offset: a control that has to be scrolled to is a control that
can be scrolled away from, and whether the gesture reaches it depends on where
the pointer happens to be.

The same spec had already met this bug once, in the plugin section, where the
actions fell below the fold of a long form (013 R11), and wrote the general
rule up in its "Gaps found" as proposed wording for fynedesygn. The rule was
written and not applied — including to the section the same spec had just
restructured. This spec applies it, to every section, and makes it a test
rather than a paragraph.

## Requirements

### R1. The rule

> Every control that starts, cancels or commits work is affixed: it occupies
> the same place in the section however much of the section is scrolled. What
> scrolls is the material the control acts on — the form, the table, the
> statistics, the prose — never the control itself. A section holding such a
> control is a `Border` with the controls in a fixed edge and a scroller in the
> centre, not a column that happens to fit the window it was built on.

- R1.1 The rule is recorded in `gui.AffixedActions`, beside the code it binds.

### R2. The sections

- R2.1 **Service**: the Control card and the journal's controls are affixed
  above the divider; the heading and Status card scroll behind them. Affixed,
  they need no scroll gesture at all, so it does not matter that the log pane
  would take one.
- R2.2 **History**: the Actions card is affixed; the heading, statistics and
  the explanatory note scroll behind it. The note is what the reader reads, not
  what they press, so it travels with the statistics.
- R2.3 **Appearance**: Reset to defaults is affixed; the form, the four notes
  and the type sample scroll behind it. This section is mostly prose, so its
  one control is the first thing to go off the bottom.
- R2.4 **Blacklist** and the **plugin** sections already satisfy the rule
  (spec 013 R11 for the plugin tab) and are unchanged.
- R2.5 **Activity**, **Settings** and **About** hold no control the rule
  covers, and the reason for each is recorded in the test rather than left to
  be re-derived.

### R3. Deliberate departures

- R3.1 Appearance's "Restart the window now" stays inline beside the interface
  scale. It appears only once the scale is changed, it acts on that row alone,
  and affixed at the foot of the section it would be a button materialising far
  from anything the user had just touched. The departure is commented at the
  point of departure, as the design system requires.

### R4. The guard

- R4.1 `gui.AffixedActions` names, per section, the controls that must not
  scroll. Labels are matched by prefix, because three of them carry a count.
- R4.2 A test builds every section and fails when a named control is enclosed
  by a `container.Scroll`.
- R4.3 A second test fails when a section has no entry and is not one of the
  three recorded as having no such control, so a section added with a toolbar
  and no entry cannot pass by not being looked at.
- R4.4 The list is written by hand rather than inferred. "A control that starts
  work" is not something a walker can tell from a button: the plugin form's
  search terms carry a remove button per row, and those belong to their row and
  should scroll with it. Naming them is also the point, as `tests/parity` names
  every operation.

### R5. Release

- R5.1 v4.2.1, a patch: this fixes a regression in v4.2.0 and adds no
  capability.

## Acceptance Criteria

- [x] AC1 The Service section's five verbs are visible without scrolling, at the divider's default position (R2.1)
  - Verified: the Control card is the `Border` bottom of the split's top half in `buildService`, so it is outside that half's scroller
- [x] AC2 The journal's Refresh now, auto-refresh toggle and interval are affixed with them (R2.1)
  - Verified: `journalControls` is affixed beside it in the same `VBox`
- [x] AC3 History's two actions are affixed and its note scrolls with the statistics (R2.2)
  - Verified: `buildHistory` is `Border(nil, actions, nil, nil, VScroll(head + stats + note))`
- [x] AC4 Appearance's Reset to defaults is affixed (R2.3)
  - Verified: `buildAppearance` is `Border(nil, reset, nil, nil, VScroll(...))`
- [x] AC5 Blacklist and the plugin sections still satisfy the rule (R2.4)
  - Verified: `TestTheAffixedControlsDoNotScroll` covers all five sections, Blacklist and the three plugins included
- [x] AC6 The scale's Restart button stays inline, with the reason commented at the point of departure (R3.1)
  - Verified: `views_appearance.go` — the departure and its reason are commented where `restart` is built
- [x] AC7 `AffixedActions` names every section that has such controls (R4.1)
  - Verified: `TestEverySectionWithControlsIsNamed`
- [x] AC8 A test fails when a named control is inside a scroller, and fails on the v4.2.0 Service section specifically (R4.2)
  - Verified: `TestTheAffixedControlsDoNotScroll`, checked against the bug: restoring the v4.2.0 shape fails it with `Service: "Start" is inside a scroller`
- [x] AC9 A test fails when a section has neither an entry nor a recorded reason (R4.3)
  - Verified: `TestEverySectionWithControlsIsNamed` fails on a section with neither an entry nor a `case` recording why it has none
- [x] AC10 `make test` passes
  - Verified: `make test` green (race detector, `-tags parity`)
- [x] AC11 `make lint` is clean
  - Verified: `make lint` — 0 issues
- [x] AC12 `govulncheck ./...` is clean
  - Verified: `govulncheck ./...` — no vulnerabilities found
- [x] AC13 Released as v4.2.1 (R5.1)
  - Verified: v4.2.1 tagged and built; see the validation report

## Risks & Assumptions

- **Rollback**: revert the commit; v4.2.0 is installable from the AUR and the
  GitHub artifacts. Rolling back restores the regression, so the rollback for
  this one is forward.
- **Layout only**: no data, no file format, no on-disk change. `clockwork-orange.yml`
  and both databases are untouched.
- **Nested scrollers**: three more sections now hold a scroller inside the
  shell's own. The design system prescribes exactly this for a pinned control
  strip, and Appearance already had one, so it is the sanctioned nesting.
- **A short window still runs out of room.** The log pane's minimum is 360 px
  and the affixed strip takes its own, so a window shorter than their sum
  clips rather than scrolls. That is Fyne's split behaviour and not something
  this change introduces or removes; the controls are in the half that keeps
  its minimum, so they are the last thing to go rather than the first.
- **The test is a list, and a list can go stale.** R4.3 is the guard on the
  guard: a new section must either name its controls or say why it has none.
  Neither test can tell whether an affixed control is *visible* at a given
  window size — only that it is outside the scroller, which is the property
  that was actually violated. A test that renders at a given size and asserts
  the control is on screen would be stronger; `fynetest` can render a window to
  an image, so it is reachable, and it is the obvious next step if this recurs.

## Gaps found

- **The rubric still belongs in fynedesygn.** Spec 013's "Gaps found" proposed
  the wording; this spec is the evidence that the narrow form of the rule in
  `docs/design-system.md` — a shape for sections that already know they need it
  — is not enough. Two sections in one program broke it inside one week, and
  one of them broke *while the rule was being written*. Worth carrying into the
  library together with a `fynetest` helper of the shape of
  `buttonsBelowAScroller`, so an adopter gets the check rather than the
  paragraph.

## Alternatives Considered

- **Infer the rule instead of listing it.** Rejected: a walker cannot tell a
  section's action from a row's own button, and flagging the plugin form's
  per-row remove buttons would make the test noise.
- **Drop the divider from the Service section** and go back to one column.
  Rejected: the divider is not what broke this; putting the controls in the
  scrolling half is. The plugin section had the same bug with no divider in it.
