## Validation Report: Spec 014 — Controls that do not scroll away

**Date**: 2026-09-18
**Spec**: specs/014-affixed-controls.md
**Branch**: `fix/affixed-controls`, off `main` at 209e1fe
**Status**: PASSED

### Summary

A regression in v4.2.0, found by looking at the window. Spec 013 R4.1 made the
Service section a split and put the heading, the Status card — which carries a
fixed-height 150 px details pane — and the Control card into the scrolling half
above the divider. At the divider's default position the Control card is below
the fold, so the five verbs the section exists for are not on screen.

Worse than below the fold: the section's other half is a log pane, which is a
`widget.List` and scrollable in its own right, and at any ordinary window
height it is the larger target. A scroll gesture goes where the pointer is, so
in a window not tall enough to show the Control card the log takes the wheel
and the controls cannot be reached at all.

Spec 013 had already met this bug once, in the plugin section (R11), and wrote
the general rule up in its "Gaps found" as proposed wording for fynedesygn. The
rule was written and not applied — including to the section the same spec had
just restructured. This spec applies it and makes it a test.

### The rule

> Every control that starts, cancels or commits work is affixed: it occupies
> the same place in the section however much of the section is scrolled. What
> scrolls is the material the control acts on — the form, the table, the
> statistics, the prose — never the control itself.

### Changes

- **Service**: Control card and journal controls affixed above the divider;
  heading and Status card scroll behind them.
- **History**: Actions card affixed; heading, statistics and the explanatory
  note scroll behind it.
- **Appearance**: Reset to defaults affixed; form, four notes and the type
  sample scroll behind it.
- **Blacklist** and the **plugin** sections already satisfied the rule and are
  unchanged.
- One deliberate departure, commented at the point of departure as the design
  system requires: Appearance's "Restart the window now" stays inline beside
  the interface scale, because it appears only when that row changes and acts
  on nothing else.

### Phase 3: Tests

- `make test`: all packages ok, 0 failing.
- `gui.AffixedActions` names, per section, the controls that must not scroll.
  A hand-written list rather than an inferred rule: a walker cannot tell a
  section's action from a row's own button, and the plugin form's search terms
  carry a remove button per row which *should* scroll with its row. Naming them
  is also the point — `tests/parity` names every operation for the same reason.
- `TestTheAffixedControlsDoNotScroll` builds every named section and fails when
  a named control is enclosed by a `container.Scroll`.
- `TestEverySectionWithControlsIsNamed` fails when a section has neither an
  entry nor a recorded reason for having none, so a section added with a
  toolbar cannot pass by not being looked at.
- **Checked against the bug, not the fix**: restoring the v4.2.0 shape fails
  with `Service: "Start" is inside a scroller, so it scrolls away from the
  section it acts on`.
- What the tests cannot do: tell whether an affixed control is *visible* at a
  given window size. They pin that it is outside the scroller, which is the
  property that was violated. `fynetest` can render a window to an image, so a
  stronger check is reachable if this recurs.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- No duplication: the three sections use the same `Border(nil, controls, …,
  VScroll(body))` shape, which is the one the design system prescribes.
- `AffixedActions` is exported and documented with the rule it encodes, so the
  rule lives beside the code it binds rather than in a spec nobody rereads.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- Layout only. No new dependency, no I/O, no external command, no secrets.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit; v4.2.0 is installable from the AUR and the
  GitHub artifacts. Note that rolling back restores the regression — the
  rollback for this one is forward, to v4.2.1.
- **Additive**: no data, no file format, no on-disk change.

### Not verified on this host

That the controls are on screen at the author's window size and scale. The
tests pin the structural property; the visual one is a manual check, and the
build is installed to `~/.local` for it.

### Overall

**PASSED.** Released as v4.2.1, a patch: this fixes a regression and adds no
capability.
