## Validation Report: Sign the macOS app bundle in CI

**Date**: 2026-09-19
**Commit**: (pre-commit)
**Status**: PASSED
**Spec**: none (release-build bug fix — no dedicated spec file)
**Issue**: #24

### Problem

The v4.3.0 macOS bundle will not launch on macOS 27.0 (26A428, arm64). macOS
rejects it before any of our code runs, and Gatekeeper's "Open Anyway" is never
offered.

```console
$ codesign --verify --strict --verbose=2 "/Applications/Clockwork Orange.app"
/Applications/Clockwork Orange.app: code has no resources but signature indicates they must be present
```

Both Mach-Os carried `Identifier=a.out` with `flags=0x20002(adhoc,linker-signed)`,
`Sealed Resources=none`, and the bundle had no `Contents/_CodeSignature/`.

The signature is **invalid**, not merely unnotarized. Note this is a packaging
defect, *not* the cause of the launch failure — see the correction below.
On arm64 every Mach-O must be signed, so with nothing signing it the linker
applies an implicit ad-hoc signature covering the executable only —
`Identifier=a.out` is the linker's default, not our `--app-id`. A bundle
signature must additionally seal `Contents/Resources` via
`_CodeSignature/CodeResources`; that file was absent, so the signature
contradicts itself.

#### Correction (post-CI, same day)

An earlier version of this report claimed the invalid seal was why the app would
not launch, and that repairing it would make Gatekeeper's "Open Anyway"
available. Both are wrong, established by isolating the two variables instead of
changing them together:

| condition | launches? |
|---|---|
| broken signature **+** quarantine | no — "damaged and can't be opened" |
| broken signature, **no** quarantine | **yes** |
| valid ad-hoc signature **+** quarantine | no — `spctl -a`: `rejected`, rc=3 |
| valid ad-hoc signature, no quarantine | yes |

Quarantine is the discriminator; the seal is not. The bundle seal is checked by
`codesign --verify` and by Gatekeeper, but is not enforced at launch — the kernel
requires only that the *executable* carry a valid signature, which `linker-signed`
already satisfies.

"Open Anyway" waives notarization for a **Developer ID** signature, where there is
a Team ID to attribute. An ad-hoc bundle has `TeamIdentifier=not set` and is
refused outright, which surfaces as "damaged".

This change is therefore packaging correctness and a prerequisite for any future
notarization. It does not fix the reported launch failure; documenting the
quarantine workaround does (#24).

Root cause is a regression from the v4 Go port. The 2.9.x build re-signed the
bundle in `scripts/build_macos.sh`
(`validation-reports/2026-08-13-macos-broken-release-build.md`). That script no
longer exists; packaging moved into `.github/workflows/build.yml` via
`fyne package` and the re-sign was not carried across. The workflow also copies
the CLI into `Contents/MacOS/` *after* packaging, which would invalidate a
resource seal even if `fyne package` had written one.

CI did not catch it because the macOS job runs `--self-test` by invoking the
binary inside the bundle directly. Executing a Mach-O does not exercise bundle
signature validation — only LaunchServices opening the `.app` does.

### Changes

Build-infrastructure only — no application source touched.

- `README.md`, `WALKTHROUGH.md`: replace the stale "first launch is
  right-click, Open" instruction — that route was removed by Apple and never
  applied to ad-hoc signatures — with the `xattr -dr com.apple.quarantine` step
  and an explanation that "damaged" is Gatekeeper declining, not a bad download.
  This is the actual fix for #24.
- `.github/workflows/build.yml`, `build-macos` job: two new steps between
  bundle assembly and `Self-test`.
  - **Sign the app bundle** — `codesign --force --sign -` on the nested CLI
    first, then on the bundle with an explicit
    `--identifier io.ushineko.clockwork-orange`. Order matters: signing the
    nested binary after the bundle would break the seal the bundle just wrote.
    No `--deep` (deprecated, and there is no nested framework here).
  - **Verify the signature** — fails the build on `codesign --verify --strict`,
    a missing `CodeResources`, an unstamped identifier, an unsealed bundle, or
    a lingering `linker-signed` flag.

Signing is placed before `Self-test` so the self-test exercises the bytes that
actually ship.

### Phase 3: Tests / Verification

- No application code changed, so `make test` / `make lint` are unaffected by
  this diff and were not re-run for it.
- The signing commands were validated against the real broken artifact on the
  affected OS (macOS 27.0, arm64) before being written into CI. Applied to
  `/Applications/Clockwork Orange.app`:

  | check | before | after |
  |---|---|---|
  | `codesign --verify --strict` | `code has no resources...` | `valid on disk`, `satisfies its Designated Requirement`, rc=0 |
  | `Identifier` | `a.out` | `io.ushineko.clockwork-orange` |
  | `flags` | `0x20002(adhoc,linker-signed)` | `0x2(adhoc)` |
  | `Sealed Resources` | `none` | `version=2 rules=13 files=2` |
  | `_CodeSignature/CodeResources` | absent | present (2666 bytes) |

  The app launches after the change. The verify step's assertions are written
  against this observed-good output.
- Workflow validated locally: YAML parses (`yaml.safe_load`), step order is
  Build → Sign → Verify → Self-test → Zip → upload, and both new `run:` scripts
  pass `bash -n`.
- End-to-end confirmation is the CI run on this PR, which is the only place
  `fyne package` output can be signed and verified as a unit.
- Status: PASSED

### Phase 4: Code Quality

- No duplication: signing appears once, in the only place a macOS bundle is
  produced. There is no local `make package-macos` target to keep in sync.
- The verify step reports which assertion failed and dumps the signature, so a
  future regression names itself rather than surfacing as a red X.
- Scratch output goes to `$RUNNER_TEMP`, not the workspace, so it cannot reach
  the artifact.
- Status: PASSED

### Phase 5: Security Review

- Ad-hoc signing (`--sign -`) needs no identity, keychain or secret. No new
  credentials enter CI and no workflow permissions change.
- This does **not** notarize the app, and does not change what a user sees on a
  downloaded zip: Gatekeeper still rejects an ad-hoc bundle ("damaged"), and no
  override is offered. Verified with `spctl -a -vvv -t exec` against the signed
  artifact: `rejected`, rc=3. Notarization has been ruled out for now; the
  supported answer is the documented `xattr -dr com.apple.quarantine` step,
  added to README.md and WALKTHROUGH.md in this branch.
- No dependency changes, so `govulncheck` surface is unchanged.
- Status: PASSED

### Phase 5.5: Release Safety

- Rollback: revert the commit. The two steps are additive — removing them
  returns the previous (broken) bundle, and nothing else in the job depends on
  them.
- No change to artifact name, layout or the release job.
- Status: PASSED
