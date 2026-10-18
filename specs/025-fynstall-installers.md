# Spec 025: Installers from fynstall

**Issue**: #44
**Related**: ushineko/fynstall#55 (staging for package managers)

## Status: INCOMPLETE

- **Priority**: Medium
- **Estimated Complexity**: Medium
- **Branch**: `feat/fynstall-installer`

## Executive Summary

`make installer` renders `packaging/fynstall/fynstall.yaml.in` with the
version and target, stages the payload beside it, and builds the Linux
installers with a pinned fynstall (v0.1.1). CI builds the Linux installers
and the Windows installer, attaches them to releases, and puts the Linux CLI
installer through an install, a run and an uninstall in a temp HOME. The
service stays the program's own. AUR packaging is unchanged; the way to
move it onto the same config is filed as ushineko/fynstall#55. Reviewers
should look at `packaging/fynstall/` and the two CI changes first.

## Context

clockwork-orange is installed four ways today, each listing the same files
on its own:
- `install.sh` builds from a checkout and copies into `~/.local`;
- `packaging/arch/PKGBUILD` and its AUR copy;
- `packaging/debian/build.sh`;
- a Windows zip of two `.exe` files, which has no install step, no Start
  Menu entry and no uninstall.

fynstall (`github.com/ushineko/fynstall`, v0.1.1) builds a single
self-contained installer from a `fynstall.yaml`. The installer has a wizard
and a command line, plus launcher entries, icons, `PATH` links and an
uninstaller that puts things back, on Linux and Windows. macOS is not
supported yet.

fynstall installs into `~/.local` or `/usr/local` and keeps its own record.
A distribution package has to be built into `$pkgdir/usr`, owned by the
package manager, with no install record and no actions run at build time.
So fynstall cannot replace the PKGBUILD as it stands. The proposed fynstall
feature in the AUR section below closes that gap.

## Requirements

- R1 `packaging/fynstall/fynstall.yaml.in` describes the program:
  - app ID `io.ushineko.clockwork-orange`, name, publisher, and the 512 px
    icon;
  - the CLI and the GUI as the payload, plus README and LICENSE;
  - a `path_links` entry for both programs;
  - one desktop entry with the fields of
    `packaging/io.ushineko.clockwork-orange.desktop` (generic name, keywords,
    category, startup notify; the WM class defaults to the app ID);
  - scopes `[user, system]`.

  `packaging/fynstall/stage.sh` fills in the version (fynstall has no
  version flag) and the one target being built, and stages the payload
  beside the rendered config. The repository therefore carries no file that
  a build rewrites.

  Changed from the draft:
  - `keep_on_uninstall` is left out. fynstall forbids `{home}` in a config
    that offers system scope, and the config lives at the literal
    `~/.config` on every OS. The uninstaller never touches it anyway,
    because the installer did not create it.
  - `linux/arm64` is left out: the GUI needs cgo and cannot be cross-built
    from the amd64 runner.
- R2 The systemd user unit is **not** a fynstall `service` action. The
  program installs its own unit (`clockwork-orange service install`, the
  GUI's Install button), and that stays the only path. Two owners of one
  unit file would fight: fynstall would back up and restore a unit the
  program had rewritten. The installer's finish page and README say where
  the button is.
- R3 `make installer` builds `dist/clockwork-orange-<ver>-linux-amd64-installer`
  and the `-cli-installer` (with their uninstallers). The fynstall builder is
  pinned in the Makefile (`FYNSTALL_VERSION`, run with `go run …@v0.1.1`),
  the same pattern as the linter pin. The Windows CI step reads the same
  variable.
- R4 CI:
  - `build-linux-installer` builds both installers, then runs the CLI one
    with `--yes --dir <tmp>` and a temp HOME. It runs the installed
    `clockwork-orange version`, checks the link and the launcher entry, runs
    `<dir>/uninstall --yes`, and checks that all three are gone. It runs
    `version`, not `--self-test`, because the runner has no KDE tools; the
    deb job skips the self-test for the same reason;
  - `build-windows` additionally builds the Windows installer, beside the
    zip;
  - `release` attaches every installer.
- R5 `install.sh` and `uninstall.sh` keep working for a build from a
  checkout. Their headers point to `make installer` as the way to install a
  release.
- R6 The installed GUI's icon resolves on Wayland: fynstall installs the
  icon under the app ID, and the desktop entry's `Icon=` is the app ID.
  This differs from the packages' `clockwork-orange.png`. Nothing in the
  program depends on the icon's name, so both may coexist.
- R7 A fynstall install and an AUR or deb package of the same version on one
  machine: the package's `/usr/bin` and the fynstall link in `~/.local/bin`
  can both exist. Document which comes first on `PATH`; do not try to
  detect it.

## AUR: proposed fynstall feature (filed upstream, not built here)

`fynstall stage --layout fhs --prefix /usr --root "$pkgdir" [--hooks pacman|deb]`
would lay the payload and integration out as a package wants it, from the
same `fynstall.yaml`:

- the payload as the program expects it, with `path_links` as real files or
  links in `/usr/bin`;
- desktop entries in `/usr/share/applications`;
- icons in `/usr/share/icons/hicolor`;
- a `service` action as a unit file in `/usr/lib/systemd/user` (or
  `/usr/lib/systemd/system`), installed but not enabled, as packages do;
- `config_file`, `run` and `migrate` actions cannot be written as files.
  With `--hooks` they become the package manager's install scripts
  (`.install` for pacman, `postinst`/`prerm` for deb). Without it, the
  stage refuses them and names them;
- no receipt, no install index, no uninstaller: the package manager owns
  removal.

The PKGBUILD's `package()` would shrink to one `fynstall stage` call. The
deb, rpm and flatpak builds would follow the same pattern, so the file list
would live in one place. Until fynstall has it, the PKGBUILDs stay as they
are.

## Acceptance Criteria

- [x] R1 `fynstall validate` passes: locally, and as a step of
      `make installer` and of the Windows CI build.
- [x] R2 The config has no `service` action, and a local install's output
      lists none.
- [x] R3 `make installer` produced both Linux installers locally.
- [x] R4 Local run of the CI job's steps, with a temp HOME:
      - the CLI installer installed 13 files, the two links, the launcher
        entry and 7 icon sizes;
      - the installed `--self-test` passed (this machine has the KDE tools);
      - `uninstall --yes` removed all of it.

      The launcher entry passes `desktop-file-validate`, and quotes an
      install path with a space.
- [x] R4 CI on PR #46:
      - `build-linux-installer` passed: install, run, uninstall;
      - `build-windows` built the Windows wizard installer, the CLI-only
        installer and their uninstallers (artifact `windows-installer`).
- [ ] R4 The installers are attached to a release. This happens at the next
      tag.
- [x] R5 `./install.sh --dry-run` runs unchanged; only the header comment
      differs.
- [ ] R6 Live, Plasma 6 Wayland: after a wizard install the launcher entry
      shows the icon, and the running window's taskbar icon is the app's.
      This is the human local test: the wizard install puts links in
      `~/.local/bin` over the `install.sh` binaries, which fynstall backs up
      and restores on uninstall.
- [ ] Live, Windows 11: per-user wizard install, Start Menu entry starts the
      GUI, Settings > Apps uninstall removes it and leaves
      `%APPDATA%\clockwork-orange.yml`.
- [x] R7 README: the installer section says which copy runs when a package
      is also installed.
- [x] Upstream issue filed on ushineko/fynstall for `stage` (#55).

## Test Strategy

- No Go code changes, so no new unit tests. The contract is "an installer
  installs a working program and the uninstaller removes it". The CI job in
  R4 checks that against a real install in a temp directory, not a mock.

## Risks & Assumptions

- fynstall is early (0.1.1). Pinning a released builder bounds the risk; a
  bad installer is replaced by the zip or the AUR package, which are
  unchanged.
- fynstall needs Go 1.26 to build the builder. CI already resolves Go via
  `go-version-file` plus `check-latest`, which is newer.
- The full installer needs cgo and builds only for its own platform: Linux
  on the Ubuntu runner, Windows on the Windows runner with MSYS2's gcc.
- macOS keeps the signed `.app` zip; fynstall has no macOS support yet.
- Rollback: delete `fynstall.yaml`, the Makefile target and the CI job. No
  existing artefact changes.

## E2E Test Plan

- Linux: download the release's `-installer`, run it from Dolphin
  (wizard) and from a terminal (`--yes`). Each time, start the GUI from the
  launcher, install the service from the GUI, and see a wallpaper change.
  Then run `~/.local/share/io.ushineko.clockwork-orange/uninstall`: the
  binaries, entry and icons are gone, and the config and databases remain.
  (R1, R2, R6)
- Windows 11: as in the acceptance criteria. (R4)

### Prod visibility

Desktop application: no production service. The release page lists the
installers, and GitHub download counts show uptake.
