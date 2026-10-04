# Spec 023: Retention only touches what the app downloaded, and 0 turns it off

**Issue**: #38

## Status: INCOMPLETE

- **Priority**: High
- **Estimated Complexity**: Low
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

## Requirements

- **R1 — History decides what retention may delete.** A file in `download_dir`
  is eligible for retention only if its MD5 is an `image_hash` in `history.db`,
  from any source (2.9.x rows say `duckduckgo_images` and `google_images`;
  matching on the hash alone keeps them eligible). Ineligible files are never
  deleted by retention and do not count toward `max_files`.
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
- **R7 — No on-disk format change.** `history.db`'s schema and DDL text are
  untouched; 2.9.x still reads and writes it.

### Deliberate Deviation

Spec 010's table gains **DV16**: retention deletes only files recorded in
history, and `max_files` ≤ 0 disables it. 2.9.5 deleted any file in the folder
(including `.last_run`) and treated 0 as "delete all". Reason: data loss
reported in #1.

## Acceptance Criteria

- [ ] With a real `history.db` and a real temp folder: a run with `max_files`
      2 over 3 recorded downloads and 2 unrecorded files (one older than every
      download) deletes the one oldest download and neither unrecorded file
      (R1, R2).
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
- [ ] The history schema golden test passes unchanged (R7).
- [ ] Spec 010 carries DV16, and its R5.3 retention sentence points to it.
- [ ] `make test` passes, `make lint` reports 0 issues, `govulncheck` is clean.

## Risks & Assumptions

- **Clearing history makes existing downloads ineligible.** After "Clear
  history", files downloaded before are no longer known, so retention will not
  delete them and they accumulate. This is the safe direction: the app cannot
  tell them from the user's own files. Changing the Clear History
  confirmation text to say so is out of scope; the PR calls it out.
- **Hashing cost.** Each run with retention on hashes every file matching the
  glob, once. At the default limits that is tens of files of a few MB each,
  well under a second; a folder of thousands of user photos costs more, but
  only reads them.
- **Identical bytes.** A user file byte-identical to a downloaded image is
  eligible. It is, in content, the downloaded image.
- **Assumption: the dialog belongs to the 0 transition only.** Setting a
  positive limit shows no dialog; the label (R6) covers that direction.
- **Assumption: negative values behave like 0.** The GUI validator already
  rejects them; only a hand-edited YAML or `--plugin-config` can supply one.
- **Out of scope:** resolving a relative `download_dir` against the working
  directory (audit finding, separate issue if wanted); `reset`, which empties
  the folder only after an explicit confirmation.
- **Rollback**: revert the commit. No on-disk change.

## E2E Test Plan

| Step | Environment | Expected | Covers |
|---|---|---|---|
| Copy 3 personal JPEGs (older mtimes) into a scratch folder; set it as DuckDuckGo's `download_dir` with `max_files` 2; `clockwork-orange plugin run duckduckgo_images --force` | Linux desk, installed build | downloads arrive; the 3 personal JPEGs remain; the folder holds those 3 plus at most 2 downloads | R1, R2 |
| Set `max_files` 0 in the GUI | Linux desk | warning dialog appears once; the saved YAML shows `max_files: 0` | R4 |
| Force another run | Linux desk | log shows retention disabled; no file deleted | R3 |
| Clear the Retention Limit box, change the interval, wait for autosave | Linux desk | YAML keeps the earlier `max_files` | R5 |

## Alternatives Considered

- Considered adding a `path` column to `downloads`; rejected. It changes the
  DDL text that 2.9.x compatibility and the golden test pin, and the content
  hash already identifies a download.
- Considered matching the plugins' own file names (`wallhaven-<id>`, MD5 names
  for DuckDuckGo) instead of history; rejected. A name is not proof of origin,
  and the issue asks for history as the source of truth.
