# Spec 023: Retention only touches what the app downloaded, and 0 turns it off

**Issue**: #38

## Status: INCOMPLETE

- **Priority**: High
- **Estimated Complexity**: Medium
- **Branch**: `fix/retention-history`

## Executive Summary

*(Filled in before the PR opens.)*

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

### How history identifies a download

`downloads` holds `url_hash`, `image_hash` (MD5 of the file as saved), `source`
and `timestamp`. It holds no path. Both plugins call `AddEntry` after the file
is in its final form (Wallhaven after trimming bars), so the MD5 of a file on
disk equals its `image_hash` for as long as nobody edits it.
`History.SeenImage(path)` already performs exactly that lookup.

A hash alone does not say *where* the download went. A downloaded wallpaper
the user copies into their own photo folder has the same hash, and if that
folder is (or later becomes) a plugin's `download_dir`, a hash-only check
would make the copy eligible. Retention must not reach across directories, so
the location has to be recorded too.

`history.db` cannot carry it: the schema golden test compares every
`sqlite_master` row against the 2.9.x fixture, so even an extra table breaks
the compatibility contract. The location goes in a separate Go-only store.

## Requirements

- **R1 — A download ledger records where each download went.**
  `config.StateDir()/downloads.db` (Go-only; 2.9.x never opens it) holds one
  row per saved download: absolute cleaned path, `image_hash` (the same MD5
  history stores), plugin name, timestamp; the path is the key. A plugin
  writes the row in the same step as its `AddEntry` for a saved image, and
  never for a duplicate or blacklisted file it removes. "Clear history" also
  clears the ledger.
- **R1a — Retention is exact to the file.** A file is eligible only if all
  three hold: its absolute path is in the ledger under the running plugin's
  name; its current MD5 equals the ledger's `image_hash`; and that hash is in
  `history.db`. History stays the source of truth for *what* was downloaded,
  the ledger adds *where*. A copy elsewhere, a file another plugin downloaded,
  an edited file, and anything the user put in the folder are never eligible,
  are never deleted by retention, and do not count toward `max_files`.
- **R1b — Ledger rows for missing files are pruned** during retention, so the
  ledger does not grow without bound when the user deletes downloads by hand.
- **R2 — Keep the newest `max_files` downloads.** Among eligible files, the
  oldest by mtime are removed until `max_files` remain. Ordering and logging
  are otherwise unchanged.
- **R3 — 0 disables retention.** `max_files` ≤ 0 means retention is off:
  nothing is hashed or deleted, and the run logs one info line saying
  retention is disabled and downloads are kept. The CLI and the GUI's run log
  both show it, since both render the same event stream.
- **R4 — Warning dialog in the GUI.** When the Retention Limit of a plugin
  changes from a positive value to 0, the GUI shows a dialog explaining that
  retention is now off, that downloads in the folder will accumulate without
  limit, and that a positive number restores it (deleting only images the app
  downloaded, oldest first). The dialog informs; it does not ask to undo. It
  is not shown when a form loads with 0 already saved.
- **R5 — An empty box is not 0.** An empty or unparseable integer field keeps
  its last valid value when the form's values are read, so editing another
  field cannot save a 0 the user did not type.
- **R6 — Labels say what 0 means.** The `max_files` field description reads
  "Retention Limit (0 = keep all)" for both plugins.
- **R7 — `history.db` is untouched.** Its schema and DDL text do not change;
  2.9.x still reads and writes it. `downloads.db` is new and additive.
- **R8 — Downloads from before this change are left alone.** They have no
  ledger row, so retention never deletes them. They stay until the user
  removes them.

### Deliberate Deviation

Spec 010's table gains **DV16**: retention deletes only files recorded in
both the download ledger and history at that exact path, and `max_files` ≤ 0
disables it. 2.9.5 deleted any file in the folder
(including `.last_run`) and treated 0 as "delete all". Reason: data loss
reported in #1.

## Acceptance Criteria

- [ ] With a real `history.db`, a real `downloads.db` and a real temp folder:
      a run with `max_files` 2 over 3 recorded downloads and 2 unrecorded
      files (one older than every download) deletes the one oldest download
      and neither unrecorded file (R1a, R2).
- [ ] No leak between directories: a byte-identical copy of a recorded
      download, placed in a second plugin's `download_dir`, is not deleted by
      that plugin's retention, and does not count toward its limit (R1a).
- [ ] A file Wallhaven downloaded is never eligible for DuckDuckGo's retention
      when both share a folder, and vice versa (R1a).
- [ ] Saving an image writes its ledger row; a duplicate or blacklisted file
      leaves none (R1).
- [ ] A ledger row whose file is gone is removed by the next retention pass
      (R1b); "Clear history" empties the ledger (R1).
- [ ] A file in history but with no ledger row (a pre-023 download) is not
      deleted (R8).
- [ ] Wallhaven's `.last_run`, and a non-image file in its folder, survive
      retention (R1).
- [ ] A downloaded file whose bytes were changed after download is not deleted
      (R1).
- [ ] `max_files` 0 and -1: no file is deleted, and the run emits the
      "retention disabled" info line (R3).
- [ ] A run that downloads nothing (fake server returns 503) deletes no
      unrecorded file (R1).
- [ ] GUI: clearing the Retention Limit box and then editing another field
      leaves the saved `max_files` at its previous value (R5, `test` driver).
- [ ] GUI: changing Retention Limit from 50 to 0 opens the warning dialog once;
      loading a form whose saved value is already 0 does not (R4, `test`
      driver).
- [ ] Both plugin schemas describe `max_files` as "Retention Limit (0 = keep
      all)" (R6).
- [ ] The history schema golden test passes unchanged, and 2.9.x fixtures
      open with no `downloads.db` present (R7).
- [ ] Spec 010 carries DV16, and its R5.3 retention sentence points to it.
- [ ] `make test` passes, `make lint` reports 0 issues, `govulncheck` is clean.

## Risks & Assumptions

- **Clearing history makes existing downloads ineligible.** After "Clear
  history", files downloaded before are no longer known, so retention will not
  delete them and they accumulate. This is the safe direction: the app cannot
  tell them from the user's own files. Changing the Clear History
  confirmation text to say so is out of scope; the PR calls it out.
- **Pre-023 downloads accumulate once (R8).** Existing installs keep every
  image downloaded before the upgrade until the user deletes it. Adopting
  them automatically would mean trusting a filename again; R8 picks safety.
- **2.9.x and 4.x sharing one machine.** Downloads made by 2.9.x get no ledger
  row, so 4.x retention leaves them alone (R8 again).
- **Hashing cost.** Each run with retention on hashes only the files that have
  a ledger row, once. At the default limits that is tens of files of a few MB each,
  well under a second; a folder of thousands of user photos costs more, but
  only reads them.
- **Assumption: the dialog belongs to the 0 transition only.** Setting a
  positive limit shows no dialog; the label (R6) covers that direction.
- **Assumption: negative values behave like 0.** The GUI validator already
  rejects them; only a hand-edited YAML or `--plugin-config` can supply one.
- **Out of scope:** resolving a relative `download_dir` against the working
  directory (audit finding, separate issue if wanted); `reset`, which empties
  the folder only after an explicit confirmation.
- **Rollback**: revert the commit. A leftover `downloads.db` is ignored by
  earlier versions and can be deleted; `history.db` is unchanged.

## E2E Test Plan

| Step | Environment | Expected | Covers |
|---|---|---|---|
| Copy 3 personal JPEGs (older mtimes) into a scratch folder; set it as DuckDuckGo's `download_dir` with `max_files` 2; `clockwork-orange plugin run duckduckgo_images --force` | Linux desk, installed build | downloads arrive; the 3 personal JPEGs remain; the folder holds those 3 plus at most 2 downloads | R1, R2 |
| Copy one of those downloads into Wallhaven's `download_dir` (`max_files` 1) and force a Wallhaven run | Linux desk | the copy survives | R1a |
| Set `max_files` 0 in the GUI | Linux desk | warning dialog appears once; the saved YAML shows `max_files: 0` | R4 |
| Force another run | Linux desk | log shows retention disabled; no file deleted | R3 |
| Clear the Retention Limit box, change the interval, wait for autosave | Linux desk | YAML keeps the earlier `max_files` | R5 |

## Alternatives Considered

- Considered hash-only eligibility (any file whose MD5 is in history);
  rejected. It reaches across directories: a copy of a download in another
  plugin's folder would be eligible.
- Considered a `path` column or an extra table in `history.db`; rejected. The
  golden test pins every `sqlite_master` row to the 2.9.x fixture.
- Considered a manifest file inside each `download_dir`; rejected. It puts a
  program file in the user's folder (Wallhaven's `*` glob would count it), and
  a user copying the folder copies the manifest with it.
- Considered matching the plugins' own file names (`wallhaven-<id>`, MD5 names
  for DuckDuckGo) instead of history; rejected. A name is not proof of origin,
  and the issue asks for history as the source of truth.
