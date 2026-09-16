## Validation Report: Spec 010 — Full Install Test on the Dev Machine

**Date**: 2026-09-15
**Spec**: specs/010-go-rewrite-v4.md (R8.7 install scripts, R4.5 service, R9.6 partial)
**Branch**: v4-go-rewrite
**Status**: PASSED

### Summary

First real installation of the Go port on the developer's KDE Plasma 6
machine (CachyOS, 3 monitors, Wayland). `install.sh` built and placed both
binaries, the desktop entry and the icon under `~/.local`; the installed CLI
passed `--self-test`; `clockwork-orange service install` rewrote the live
systemd user unit and `service restart` replaced the Python 2.9.5 daemon
(running since 2026-09-06) with the Go one, which completed its first dual
cycle at once: three monitors set through `qdbus6`, the lock-screen image
written to `~/.config/kscreenlockerrc` through `kwriteconfig6`, the two
network plugins skipped on their hourly interval, and the unknown
`stable_diffusion` block preserved and skipped (D6).

One defect found and fixed: the embedded unit named
`/usr/bin/clockwork-orange`, which does not exist on an `install.sh`
install. `Install` now writes `ExecStart` for the binary that ran it
(`platform.UnitFileFor`; R8.7a), matching the Python's
`service_install(base_path)`. The packaged unit is unchanged.

### What was run

| Step | Result |
|------|--------|
| `install.sh --dry-run` | lists the four files (AC R8.7) |
| `install.sh` | built CLI (`CGO_ENABLED=0`) and GUI, installed to `~/.local/bin`, `~/.local/share/applications`, `~/.local/share/icons/hicolor/512x512/apps` |
| `desktop-file-validate io.ushineko.clockwork-orange.desktop` | valid |
| `clockwork-orange --self-test` (installed binary) | all 9 probes OK, exit 0 |
| `clockwork-orange service install` | unit written with `ExecStart=/home/…/.local/bin/clockwork-orange --service`, daemon-reload, enabled |
| `clockwork-orange service restart` | Python daemon stopped cleanly (signal 15), Go daemon active; RSS 9.3 MB (Python: 6.1 GB, 9.6 GB peak) |
| First cycle (journal) | dual mode from config, `default_wait` 180 s, 3 monitors, wallpapers + lock screen set, `Waiting 3m0s` |
| `~/.config/kscreenlockerrc` | `Image=file:///…/GoogleImages/eb3ffb8b….jpg`, the file the log named |
| `clockwork-orange-gui` (installed, 8 s) | ran and exited cleanly on timeout |
| `gtk-launch io.ushineko.clockwork-orange` | started `~/.local/bin/clockwork-orange-gui`; killed after 6 s |
| `uninstall.sh --dry-run` | lists exactly the four installed files (AC R8.7); not run for real |
| `make pkg-arch` | see the Arch package section below |

Backups taken before the switch, in `~/clockwork-orange-v4-backup-20260915-2225/`:
the previous unit file, `clockwork-orange.yml`, `history.db`, `blacklist.db`.

### Rollback

```
cp ~/clockwork-orange-v4-backup-20260915-2225/clockwork-orange.service ~/.config/systemd/user/
systemctl --user daemon-reload && systemctl --user restart clockwork-orange
./uninstall.sh
```

The databases and config were not modified by the switch itself; the Go
daemon writes the same schema the Python reads, so no restore is needed to
go back.

### Arch package (`make pkg-arch`, local makepkg)

First run failed in `check()`: the core loop tests took the daemon lock in
`/tmp`, which the now-running Go daemon holds, so `RunLoop` was refused and
one test hung until the 10 min timeout. Fixed by isolating `CLOCKWORK_LOCK_DIR`
in the core test harness (the CLI and GUI harnesses already did) and by
guarding the waiting test with a timeout. Second run: `check()` green,
package `clockwork-orange-git-2.9.5.r189.g98631db-1-x86_64.pkg.tar.zst`
(18.1 MB, 43.5 MB installed) with both binaries, the desktop entry, seven
hicolor icons, the user unit (`ExecStart=/usr/bin/clockwork-orange --service`),
LICENSE and README; `depend` list as R8.2. The CLI extracted from the
package passes `--self-test --offline`. `pacman -U` was not run (needs
sudo); the AUR path is exercised by CI in Phase 6. makepkg rewrites
`pkgver=` in the PKGBUILD in place; that edit was reverted.

### Not covered

- The dotfiles copy of the unit (`~/git/dotfiles/hosts/*/.config/systemd/user/`)
  still carries the AUR variant (`/usr/bin/clockwork-orange`); it is updated
  at cutover. The live unit is now a plain file written by the Go binary,
  not a symlink into dotfiles.
- The network plugins did not run in this window (hourly interval not
  elapsed); a full plugin cycle under the Go daemon is confirmed by the
  journal over the next hour.
- Windows and macOS (R9.6) remain untested.
