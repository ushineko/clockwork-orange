# Spec 021: the plugins fold, and a local build says so

## Status: COMPLETE

## Context

Two things, both about telling one thing from another at a glance.

**The navigation.** The window lists `Local`, `Wallhaven` and
`DuckDuckGo Images` between `Service` and `History`: three of nine entries,
all three of one kind. Along the top that is nine buttons on one line, and a
fourth plugin would be a tenth. The reader wants "the plugins" from that list
far more often than they want a particular one.

**The version.** `.tag` is stamped into every build, so a window built from a
working tree reported the same string as the package — `v4.3.2` either way.
A window open beside a terminal could not be told from the one pacman
installed, which is exactly the confusion a local build creates. Taken from
hotaru, which had the same problem and the same fix.

## Requirements

- **R1** The plugin sections are folded under one `Plugins` heading in the
  navigation, in every shape the shell draws.
- **R2** The heading is not a section. `--section wallhaven`, `Ctrl+1..9`, the
  tray's About item and every `Select` call still name the plugin itself, and
  the plugin titles do not change.
- **R3** The group's members are exactly the available plugins, and they are
  contiguous in the section order.
- **R4** A build that is not the release reports `<version>-<commit>-dev`.
  Released is HEAD sitting exactly on this version's tag with nothing modified.
- **R5** A package is always the plain version, however it was checked out.

## Design

The folding itself is fynedesygn's (its spec 031, released in v0.1.40): this
program declares one `shell.NavGroup` and the library draws it. `pluginGroup`
builds the group from `core.AvailablePluginNames`, which is the same list
`sectionTitles` walks — so the members cannot drift from the sections.

The version is the Makefile's: `VERSION` is `.tag` when `git describe
--exact-match` finds this version's tag on HEAD and `git status --porcelain`
is empty, and `$(TAG)-$(COMMIT)-dev` otherwise. Both PKGBUILDs pass `VERSION`
explicitly, because `clockwork-orange-git` builds from a checkout that is
never sitting on a tag and would otherwise ship a `-dev` string in a package.

## Acceptance Criteria

- [x] The plugin sections are drawn under a `Plugins` heading (fynedesygn
      spec 031; declared in `shellOptions`, `internal/gui/app.go`)
- [x] The group's members are exactly the plugin titles
      (`TestThePluginGroupNamesEveryPluginSectionAndNothingElse`)
- [x] `Plugins` is not among the section names, so nothing can navigate to it
      (same test)
- [x] The plugin sections are contiguous (`TestThePluginSectionsAreContiguous`)
- [x] A working-tree build reports `4.3.2-<commit>-dev`; verified by running
      `make build` on a dirty tree sitting on the v4.3.2 tag
- [x] Both PKGBUILDs pass `VERSION` so a package is the plain version

## Risks & Assumptions

- **Rollback**: revert the commit and reinstall the previous release. No
  on-disk change; the open/closed state is one added key in the window's own
  `gui-settings.json`, which the shell already writes and which an older build
  ignores.
- The CI job that compiles on every push has no tags fetched, so its build
  reports `-dev`. That job produces no released artifact; the four packaging
  jobs check out with `fetch-depth: 0` and build at the tag.

## Deliberate Deviations

None. The navigation is the GUI's own (spec 013), and 2.9.x had no equivalent.
