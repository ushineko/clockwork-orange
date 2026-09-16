## Validation Report: Spec 010 Phase 4 — CLI, Daemon, Self-Test, Parity Guard

**Date**: 2026-09-15
**Spec**: specs/010-go-rewrite-v4.md (Phase 4: R6, R9.4; plugin AC from R5 verified)
**Branch**: v4-go-rewrite
**Status**: PASSED (one AC deferred, see Phase 6.5)

### Summary

Adds `internal/cli` (cobra command tree over `internal/core`) and wires
`cmd/clockwork-orange`. The root command is the v2.9.5 argparse surface flag
for flag, with `_validate_args` messages and exit codes (usage 2, operation
failure 1); `--run-plugin` is rejected as unknown (DV8). New subcommands
`service`, `blacklist`, `history`, `plugins`, `plugin run`, `config`,
`version` give the CLI every GUI operation (R6.5). Bare invocation and
`--gui` run the `clockwork-orange-gui` binary and return its exit status.
`--service` runs `core.RunLoop` with the 900 s default and the config
watcher. Logging is a `log/slog` handler printing `[LEVEL] msg` to stderr;
plugin progress renders as the `::PROGRESS::` / `::IMAGE_SAVED::` markers.
`--self-test` prints one line per probe and exits 0 iff all pass.

Parity fixes in `core`: `ResolveMode` now follows `merge_config_with_args`
(config `desktop`/`lockscreen` apply only when neither flag is given,
`desktop` first); `AvailablePluginNames()` lists the registry without
opening the stores. `platform.TryLock` honours `CLOCKWORK_LOCK_DIR` so test
binaries do not contend for the daemon lock in `/tmp`.

Repository fix: `.gitignore` ignored `clockwork-orange/` unanchored, which
had silently excluded `cmd/clockwork-orange/main.go` from every earlier
commit on this branch. Anchored to the repo root; the file is now tracked.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): 9 packages ok, 247 test
  functions, 5 env-gated skips.
- Coverage: `internal/cli` 90.4 % (target ≥ 70 %, R9.1). `internal/core`
  55.0 %, unchanged from Phase 3 and below target; the GUI phase exercises
  the remaining service/store paths, revisit then.
- `internal/cli` tests run the command tree end to end over a fake desktop,
  a fake `systemctl` and temp SQLite files with `HOME` redirected: the
  `_validate_args` table (10 cases, exit 2, message text), file/directory/
  URL/plugin dispatch per mode, dual-with-file refusal, dynamic
  multi-plugin mode, `--service` 900 s default and config-edit interruption
  (integration test with a temp `HOME`), second-daemon refusal,
  `--write-config`, `--self-test` pass and fault-injected fail (PATH
  emptied), GUI hand-off via a stand-in script (exit code propagated) and
  the missing-GUI guidance, every subcommand, and `config migrate` against
  the Python golden pre-migration file (compared as parsed YAML; the
  stable_diffusion block survives).
- Live tests run on the dev machine this pass: `CLOCKWORK_LIVE_KDE=1`
  (wallpaper + lock screen via real `qdbus6`/`kwriteconfig6`, lock-screen
  image restored afterwards) pass; `CLOCKWORK_LIVE_NET=1` (Wallhaven search,
  DDG vqd + i.js) pass. `CLOCKWORK_LIVE_SYSTEMD=1` skipped by design: the
  production `clockwork-orange.service` is active on this machine.
- Manual smoke run of the built binary: exit codes 0/1/2 as specified;
  `service status` read the live unit; `--self-test` all probes OK.
- `go build ./...` (`CGO_ENABLED=0`) and `go vet -tags parity ./...`: OK.

### Phase 4: Code Quality

- `make lint`: 0 issues (golangci-lint v2.12.2, nmsbonker config).
- One dispatch function replaces the four near-identical Python mode
  handlers; the unreachable dual-mode checks in `_validate_args` are
  documented rather than ported.
- `gofmt` clean.

### Phase 5: Security Review

- `govulncheck -mode binary` (v1.1.4, DB vuln.go.dev) on the
  `CGO_ENABLED=0` `clockwork-orange` binary built with go1.27.1: no
  vulnerabilities found. Source mode is unusable on this machine (the
  linter-pinned go1.26 govulncheck cannot parse the go1.27 standard
  library), which is why the binary was scanned.
- OWASP pass on the new code: no shell is invoked anywhere; the GUI child
  is either the sibling binary, a PATH lookup of a fixed name, or an
  explicit operator override (`CLOCKWORK_ORANGE_GUI`), argv fixed. Command
  line JSON (`--plugin-config`) is decoded with `encoding/json` into a map
  and merged over the config block; it never reaches a shell or a query.
  User paths flow to `core`, which expands and validates them. Log output
  carries no credentials (Wallhaven key redaction, DV13, is upstream of the
  CLI). No secrets in changed files.

### Phase 5.5: Release Safety

- Rollback: revert the commit on `v4-go-rewrite`; `main` untouched.
- Additive: no Python file modified; packaging unchanged.

### Phase 6.5: Spec Reconciliation

Phase 4 acceptance criteria checked with two annotations (parity guard is
the Phase 4 shape until `gui.Actions()` exists; live-net run recorded). The
Phase 3 `CLOCKWORK_LIVE_KDE` criterion is now checked (run this pass).
Deferred: the `CLOCKWORK_LIVE_SYSTEMD` criterion — the env-gated test exists
in `internal/cli/service_live_test.go` but skips while the production unit
is active; to be run in the Phase 6 container or after cutover. Spec status
remains IN_PROGRESS (Phases 5–7 pending). R6.9 added to record the optional
additions (`--config`, `--log-level`, hidden `--offline`, two environment
variables) and the ported merge semantics.
