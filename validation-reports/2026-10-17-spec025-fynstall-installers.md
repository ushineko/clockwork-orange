## Validation Report: Spec 025, installers from fynstall

**Date**: 2026-10-17
**Spec**: `specs/025-fynstall-installers.md` (#44)
**Status**: PASSED (CI and manual criteria open; see "Not verified")

### Summary

New files:
- `packaging/fynstall/fynstall.yaml.in` and `stage.sh`.

Changed:
- a `make installer` target, with fynstall pinned at v0.1.1;
- a `build-linux-installer` CI job;
- a Windows installer step in `build-windows`;
- the installers attached to releases;
- the README in both copies, and the header comment of `install.sh`.

No Go source changes.

### Phase 3: Tests

- `make test`: all packages pass. `internal/gui` renders the README copy
  that changed.
- Local run of what the CI job does, with a temp HOME:
  - the CLI installer installed the binaries, the links, the launcher entry
    and 7 icon sizes;
  - the installed `--self-test` passed;
  - the uninstaller removed everything.

  The launcher entry passes `desktop-file-validate`.
- No new unit tests: there is no Go change. The contract ("installs a
  working program, removes it cleanly") is checked against a real install,
  not a mock.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- The fynstall version has one source, the Makefile; the Windows step reads
  it.
- `stage.sh` is `set -euo pipefail` and quotes every path.

### Phase 5: Security Review

- `govulncheck ./...`: no vulnerabilities found (no Go change).
- The builder is pinned to a release tag, not `@latest`. It is fetched
  through the Go module proxy and checksum database.
- No secrets. The CI changes use no new secret.

### Phase 5.5: Release Safety

- Additive: the zip, the AUR package, the deb and `install.sh` are
  unchanged.
- Rollback: delete `packaging/fynstall/`, the Makefile target and the two
  CI changes.

### Not verified

- The CI job and the Windows build on GitHub: pending the PR's run.
- A wizard install on Plasma, and on Windows 11, by hand.

### Overall

**PASSED.**
