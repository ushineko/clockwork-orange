## Validation Report: Spec 023, retention only touches what the app downloaded
**Date**: 2026-10-03 23:30
**Branch**: `fix/retention-history`
**Status**: PASSED (one manual GUI criterion open: the warning dialog)

### Phase 3: Tests
- `make test` (`go test -race -tags parity ./...`): all packages pass.
- New or changed tests: `TestCleanupDeletesOnlyRecordedDownloadsOldestFirst`,
  `TestCleanupIgnoresCopiesAndEditsOfDownloads`,
  `TestCleanupWithLimitZeroOrLessDeletesNothing`,
  `TestLegacyHistoryIsMigratedOnceWithEveryRow`,
  `TestPythonWrittenHistoryDBReadsIdenticallyFromGoAfterMigration`,
  `TestAdoptGrandfathersPathlessDownloadsOncePerFolder`,
  `TestIntFieldReadsTheLastValidNumberWhenEmpty`; the DuckDuckGo and
  Wallhaven retention tests now assert that unrecorded files survive.
- Removed: `TestGoWrittenHistoryDBHasByteIdenticalSchemaToPython` (history is
  no longer 2.9.x-compatible by decision; replaced by the migration test).
- Contract quality: tests assert which files remain on disk against a real
  SQLite file, not call sequences.
- Live E2E on the desk with populated folders: recorded in the spec.

### Phase 4: Code Quality
- `make lint`: 0 issues (golangci-lint v2.12.2, cache cleaned of a deleted
  worktree's entries first).
- Dead code: none left. `cleanupOldFiles` dropped an unreachable clamp.
- Duplication: `AddEntry`/`AddImported` share `add`.

### Phase 5: Security Review
- `govulncheck ./...` (govulncheck@v1.8.0): no vulnerabilities found.
- SQL: every new statement is parameterised; `ATTACH DATABASE ?` takes the
  legacy path as a bound value.
- Paths: retention deletes only paths recorded by the app whose content still
  matches (narrows the previous delete set); `filepath.Abs` normalises before
  storing and comparing without resolving symlinks, so a symlink's own path
  is never a recorded download and is never deleted by retention.
- Secrets: none added; no credentials logged.

### Phase 5.5: Release Safety
- Rollback: reinstall the previous release. `history.db` is not modified by
  the migration, so the old version reads it as before (missing only
  downloads made since). Deleting `history2.db` reruns the migration.
- Backup taken before the live test: config, DBs and hard-link copies of both
  download folders in `~/clockwork-orange-backup-spec023-20261003-2316/`.
