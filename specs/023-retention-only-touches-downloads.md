# Spec 023: Retention only touches what the app downloaded, and 0 turns it off

**Issue**: #38

## Status: COMPLETE

- **Priority**: High
- **Estimated Complexity**: Medium
- **Branch**: `fix/retention-history`

## Executive Summary

Retention deleted the oldest files in a plugin's download folder whether or
not the app downloaded them, and a limit of 0 deleted all of them: the likely
cause of #1. History moves to `history2.db`, which records where each
download was saved and is migrated from `history.db` on first open; existing
files are adopted by content once per folder. Retention now deletes only a
file recorded at that exact path with unchanged content, and `max_files` 0
turns it off with a GUI warning. Reviewers should look first at
`History.Adopt` and `History.Downloaded` in `internal/store/history.go` and at
`cleanupOldFiles` in `internal/plugins/schedule.go`.

## Context

The Wallhaven and DuckDuckGo plugins keep `download_dir` to `max_files` images
by deleting the oldest. `cleanupOldFiles` (`internal/plugins/schedule.go`)
globs the folder (Wallhaven `*`, DuckDuckGo `*.jpg`), sorts every regular file
by mtime and removes the oldest beyond the limit. Spec 010 ported this from
2.9.5 as-is (R5.3, R5.4), including deleting Wallhaven's `.last_run`.

An audit of the v4 code for issue #1 ("it deleted all the images in the
wallpaper folder") found that this one function explains the report:

1. **Any file in the folder is eligible.** A `download_dir` that already holds
   the user's pictures, or that is the local plugin's folder, loses those
   pictures first, because they are the oldest. `history.db` records the
   content hash of every image the app downloaded, and cleanup never asks it.
2. **`max_files` of 0 deletes every matching file**, and a negative value does
   too. The GUI saves 0 without being asked: an empty Retention Limit box reads
   as 0 (`intField.value()`, `internal/gui/views_plugin.go`), and any edit to
   another field autosaves the whole block.
3. **Cleanup runs when the run downloaded nothing** (a Wallhaven outage, a
   DuckDuckGo 403), so a run that only fails still removes files.
4. With the defaults, one run can replace the folder: `limit` is per search
   term, so 10 terms × 10 images exceeds DuckDuckGo's `max_files` of 50.

Points 1 and 2 are fixed here. Point 3 is harmless once only downloaded files
are eligible. Point 4 is the configured behaviour and stays.

### Why history needs a path

`history.db` holds `url_hash`, `image_hash` (MD5 of the file as saved),
`source` and `timestamp`: what was downloaded, not where it went. A hash-only
check would reach across directories: a downloaded wallpaper copied into
another plugin's folder has the same hash and would become deletable there.
Retention needs the path.

2.9.x compatibility of the history file is given up for this (operator
decision, 2026-10-03): v4 moves to a new `history2.db` and leaves
`history.db` as it was.

## Requirements

- **R1 — `history2.db` replaces `history.db`.** Same `downloads` table plus a
  `path` column (absolute path the file was saved to) and an `adopted_dirs`
  table. Every saved download records its path. `history import` records
  without a path, since imported files are the user's.
- **R2 — Migration on first open.** When `history2.db` does not exist and
  `history.db` does, every legacy row is copied with no path. The copy is
  built under a temporary name and renamed, so an interrupted migration is
  redone. `history.db` is not modified or deleted.
- **R3 — Existing files are grandfathered.** The first time retention runs on
  a folder, each regular file in it whose content matches a pathless,
  non-imported row takes that row's path. Each row is adopted at most once
  and each folder once, so a later copy of a download is never adopted.
- **R4 — Retention is exact to the file.** A file is eligible only if its
  absolute path is recorded and its current MD5 equals the recorded hash.
  Copies elsewhere, edited downloads, imports, `.last_run` and the user's own
  files are never deleted and do not count toward `max_files`. Among eligible
  files the oldest by mtime go first.
- **R5 — 0 disables retention.** `max_files` ≤ 0: nothing is hashed or
  deleted, and the run logs `Retention is off (max_files N); keeping every
  download.` (CLI and GUI render the same event).
- **R6 — Warning dialog.** When a plugin's Retention Limit changes from a
  positive value to 0 and stays 0 for 1.2 s, the GUI explains that retention
  is off, downloads accumulate without limit, and a positive number turns it
  back on, deleting only images the app downloaded into that folder.
- **R7 — An empty box is not 0.** An empty or out-of-range integer field
  reads back its last valid value.
- **R8 — Labels.** Both plugins describe `max_files` as "Retention Limit
  (0 = keep all)".
- **"Clear history"** empties `adopted_dirs` too.

### Deliberate Deviations

Spec 010's table gains **DV16**: the history database is `history2.db` with a
`path` column, migrated from `history.db` once; retention deletes only files
recorded at that exact path with unchanged content; `max_files` ≤ 0 disables
it. 2.9.5 deleted any file in the folder (including `.last_run`) and treated 0
as "delete all". Reason: data loss reported in #1.

## Acceptance Criteria

- [x] Retention removes only recorded downloads, oldest first; an older user
      file and `.last_run` stay (`TestCleanupDeletesOnlyRecordedDownloadsOldestFirst`;
      plugin-level: `TestDuckDuckGoResetAndRetentionOnlyTouchJPEGs`,
      `TestWallhavenRetentionRunsAfterDownloadingAndSparesFilesItDidNotDownload`).
- [x] No leak between directories, and edits are spared
      (`TestCleanupIgnoresCopiesAndEditsOfDownloads`; live run below).
- [x] `max_files` 0 and -1 delete nothing and log that retention is off
      (`TestCleanupWithLimitZeroOrLessDeletesNothing`; live run below).
- [x] The Python-written `history.db` fixture migrates row for row, reads
      identically, and is migrated only once
      (`TestLegacyHistoryIsMigratedOnceWithEveryRow`,
      `TestPythonWrittenHistoryDBReadsIdenticallyFromGoAfterMigration`).
- [x] Adoption takes pathless rows once per folder and never a later copy
      (`TestAdoptGrandfathersPathlessDownloadsOncePerFolder`).
- [x] Empty integer box reads its last valid value
      (`TestIntFieldReadsTheLastValidNumberWhenEmpty`).
- [x] Labels read "Retention Limit (0 = keep all)" (schema tests).
- [x] Warning dialog appears when Retention Limit goes to 0 (manual, GUI;
      confirmed by the operator on the desk, 2026-10-03).
- [x] Spec 010 carries DV16.
- [x] `make test` passes, `make lint` reports 0 issues, `govulncheck` is clean.

## Risks & Assumptions

- **2.9.x and 4.x no longer share history.** A 2.9.x install on the same
  machine keeps writing `history.db`, which v4 no longer reads after the
  migration. Accepted.
- **Rollback**: reinstall the previous release. It reads `history.db`, which
  the migration left untouched, so it misses only the downloads made since;
  those may be fetched again. Delete `history2.db` to rerun the migration
  after a later upgrade.
- **Adoption reads each file in a folder once**, the first time retention
  runs there (measured: 500 files / 704 MB took well under the run's network
  time).
- **A copy made before a folder's first adoption** can be adopted in place of
  the original if it is seen first. Bounded by the one-row-one-file rule.
- **After "Clear history"** nothing is eligible until new downloads arrive;
  existing files accumulate. Safe direction.
- **Out of scope:** relative `download_dir` resolution; the CLI's
  unconfirmed `--reset`.

## E2E Test Plan

| Step | Environment | Expected | Covers |
|---|---|---|---|
| Install the dev build over the running v4.4.2 (`./install.sh`), with a 2020-dated user file `zz-mine-test.jpg` in the DuckDuckGo folder; `plugin run duckduckgo_images --force --plugin-config '{"limit":2}'` | Linux desk, live folders (501 files) | migration creates `history2.db` with every row; `Adopted 500`; new downloads saved; the oldest downloads removed to reach 500; `zz-mine-test.jpg` stays | R1–R4 |
| Copy a DuckDuckGo download into the Wallhaven folder dated 2020; `plugin run wallhaven --force --plugin-config '{"limit":1,"max_files":95}'` | Linux desk | `Adopted 100`; 5 oldest downloads removed; the copy and `.last_run` stay | R3, R4 |
| `plugin run wallhaven --force --plugin-config '{"max_files":0}'` | Linux desk | "Retention is off"; nothing removed | R5 |
| GUI: set Retention Limit to 0, then back | Linux desk | dialog once; YAML shows 0, then the new value | R6 |

## E2E Results (2026-10-03, Linux desk, dev build of this branch)

- DuckDuckGo: `history2.db` created with 6612 migrated rows; `Adopted 500
  existing downloads into retention`; 2 images saved, `Cleaning up 2 old
  images` removed the two oldest downloads (2026-07-31 15:46:07 and :18);
  `zz-mine-test.jpg` (2020, oldest file in the folder) and `.last_run` stayed.
- Wallhaven: `Adopted 100`; 5 oldest downloads removed to reach 95; the
  2020-dated copy of a DuckDuckGo download and `.last_run` stayed. The 5 test
  deletions were restored from a hard-link backup afterwards (the live limit
  is 250).
- `max_files` 0: `Retention is off (max_files 0); keeping every download.`,
  97 files before and after.

## Alternatives Considered

- Considered hash-only eligibility; rejected. It reaches across directories.
- Considered a separate Go-only path ledger next to an untouched
  `history.db`; rejected by the operator in favour of one migrated store.
- Considered matching the plugins' file names (`wallhaven-<id>`, URL-MD5
  names) for adoption; rejected. A name is not proof of origin; content
  matched against history is.
