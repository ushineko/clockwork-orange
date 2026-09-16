# Spec 010: Clockwork Orange v4.0 — Native Go Rewrite

> **Note**: This work has no associated issue tracker ticket (personal public
> GitHub repository, no tracker). Consider creating a GitHub issue for
> traceability.

## Status: IN_PROGRESS

- **Priority**: High
- **Estimated Complexity**: High
- **Target version**: v4.0.0 (see Decision D1)
- **Branch**: `v4-go-rewrite` (long-lived; `main` continues to ship the
  Python 2.9.x line until cutover)
- **Umbrella spec**: this document defines scope, architecture and acceptance
  criteria for the whole release. Implementation is phased (see
  "Implementation Phases"); each phase may be split into a child spec
  (`011-…`, `012-…`) once this document is approved.

---

## Overview

Rewrite Clockwork Orange in Go, replacing the Python 3 / PyQt6 code base with
two native binaries: a CGO-free CLI/daemon and a Fyne GUI. The release is a
straight functional port of v2.9.5 with one intentional omission (the Stable
Diffusion plugin, deferred to a later release). The GUI adopts the Fyne design
system already used by `~/git/angou` and `~/git/nmsbonker`, taking the newer
`nmsbonker` variant (centred modal busy popup, floating flash banner,
`detailTable`, log pane). The build, packaging and CI pipeline is converted to
the Go project layout used by those two repositories, while keeping this
repository's release entry points (`.tag`, `release_version.sh`, AUR job,
per-platform GitHub Release artifacts).

## Context

- v2.9.5 is ~9.4k lines of Python across `clockwork-orange.py` (CLI, engine,
  cycling loops, config watcher), `platform_utils.py` (KDE/Windows/macOS
  wallpaper + lock screen + systemd), `plugin_manager.py` (subprocess plugin
  runner), `config_migrations.py`, `plugins/` (local, wallhaven,
  duckduckgo_images, stable_diffusion, history, blacklist) and `gui/` (PyQt6).
- Pain points motivating the port: PyInstaller fragility on Windows/macOS
  (hidden imports, watchdog module paths, framework-Python requirement, DLL
  hunting), a 300 MB+ dependency footprint, Qt6 runtime dependency on Linux,
  a plugin protocol that requires spawning an interpreter per plugin per
  cycle, and no shared code with the author's other desktop tools.
- `~/git/nmsbonker/internal/gui` is a proven Fyne 2.8.1 design system (theme,
  fonts, cursor, busy/flash, table, log pane, dialogs) with headless tests and
  a CLI/GUI parity guard. `~/git/nmsbonker/specs/003-fyne-gui.md` documents
  the idioms to carry over verbatim; `~/git/nmsbonker/.claude/CLAUDE.md`
  documents the architecture rules (core-first, parity, cancellable work via
  `core.Events`, nothing transient reflows the layout).
- Reference inventories used to write this spec: the Python code base at
  commit `67ad8d2` and the two sibling repos at their current HEAD. Where this
  spec states an exact command, key, path, hash or schema, it was taken from
  the Python source and must be preserved unless listed under "Deliberate
  Deviations".

## What this is not

- **Not** a port of the `stable_diffusion` plugin. Its config block must be
  preserved on disk (D6) but no code, GUI section, venv routing, CPU-affinity
  handling, `setup_stable_diffusion.sh`, or pacman `optdepends` ship in v4.0.
- **Not** a feature release. New user-visible behaviour is limited to the
  items in "Deliberate Deviations" and to CLI subcommands that expose
  existing GUI-only operations (service install/uninstall) for parity.
- **Not** a redesign of the plugin catalogue, config schema, or on-disk
  formats. `~/.config/clockwork-orange.yml`, `history.db` and `blacklist.db`
  written by v2.9.x must load unchanged.
- **Not** a Windows service or macOS launchd agent. As in v2.9.x, Windows and
  macOS cycle from the GUI's timer; Linux uses the systemd user unit.

---

## Decisions Requiring Reviewer Sign-off

These are design choices with material consequences. Each has a default that
the spec is written against. Reviewer: confirm or override before Phase 1.

| # | Decision | Default in this spec | Alternative |
|---|----------|----------------------|-------------|
| D1 | Version number | **v4.0.0**, as stated in the initiative brief ("Major new feature 4.0"). Semver alone would give v3.0.0. | v3.0.0 |
| D2 | Binary layout | **Two binaries**: `clockwork-orange` (cobra CLI + daemon, `CGO_ENABLED=0`) and `clockwork-orange-gui` (Fyne, CGO). Matches angou/nmsbonker and lets the daemon build/run without OpenGL. `clockwork-orange --gui` execs the GUI binary for backward compatibility with docs and the desktop entry. | One CGO binary with `--gui` |
| D3 | Plugin execution | **In-process Go plugins** implementing a `Plugin` interface in `internal/plugins`. The Python subprocess/JSON-on-stdout protocol and the ability to drop third-party `.py` plugins are dropped. Progress markers (`::PROGRESS::`, `::IMAGE_SAVED::`) become typed `core.Events` callbacks. | Keep subprocess protocol (exec external plugin binaries) |
| D4 | Config format | **Keep YAML** at `~/.config/clockwork-orange.yml` (all OSes), `gopkg.in/yaml.v3`, `sort_keys` ordering on write. Differs from the nmsbonker JSON convention but preserves user configs and the systemd/GUI file-watch contract. | Migrate to JSON in `$XDG_CONFIG_HOME/clockwork-orange/config.json` with a one-shot importer |
| D5 | SQLite driver | **`modernc.org/sqlite`** (pure Go) so the CLI stays `CGO_ENABLED=0`. Existing `history.db` / `blacklist.db` schemas are reused byte-compatible. | `mattn/go-sqlite3` (CGO) |
| D6 | Unknown plugin config blocks | **Preserve** unknown `plugins.<name>` blocks (notably `stable_diffusion`) on load/save, mirroring nmsbonker's `extra map[string]RawMessage` pattern. v2.9.x `clean_config` deleted them; that behaviour is dropped because the SD block must survive until the plugin returns. | Port `clean_config` as-is (destroys SD settings) |
| D7 | Version source of truth | **Keep `.tag`** (`vX.Y.Z`) so `release_version.sh`, the AUR job and the About dialog keep working. Makefile derives `VERSION` from `.tag`; no `VERSION` file. | Adopt nmsbonker's `VERSION` file and rewrite the release tooling |
| D8 | Repo cohabitation | Go code lands at the repo root (`cmd/`, `internal/`, `packaging/`, `Makefile`, `go.mod`) on the `v4-go-rewrite` branch. Python sources stay in the tree until the cutover phase, which deletes them in one commit. `main` is never touched until merge. | Separate repository |
| D9 | DuckDuckGo backend | **Direct scrape only** (vqd token + `i.js`), which is already the Python fallback path. No Go equivalent of the `ddgs` library exists. | Vendor/port `ddgs` |
| D10 | Windows multi-monitor | **Port the spanned-composite technique** (stitch one JPEG, registry `WallpaperStyle=22`, `SystemParametersInfoW`). | Switch to `IDesktopWallpaper` COM (per-monitor native) — behaviour change |
| D11 | macOS wallpaper API | **cgo + AppKit** (`NSWorkspace setDesktopImageURL:forScreen:options:error:` per screen) in the GUI binary; `osascript` fallback in the CGO-free CLI binary. | osascript only |
| D12 | Worktree for implementation | Implementation happens in the sibling worktree `../clockwork-orange.worktrees/v4-go-rewrite` (created with this spec). | Branch in place |

---

## Requirements

Numbering `Rn.m` is referenced from code comments and tests.

### R1. Repository, toolchain, layout

- R1.1 Module `github.com/ushineko/clockwork-orange`; `go 1.25.0` in `go.mod`,
  **no `toolchain` line** (rationale copied from `nmsbonker/Makefile:50-56`).
- R1.2 Dependencies (pinned in `go.mod`): `fyne.io/fyne/v2 v2.8.1`,
  `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`,
  `github.com/fsnotify/fsnotify`, `golang.org/x/image` (draw, bmp, tiff,
  webp), `golang.org/x/sys` (windows registry / unix), `github.com/stretchr/testify`
  (tests only). Any further dependency requires a spec note.
- R1.3 Layout (copied from nmsbonker):

  ```
  cmd/clockwork-orange/main.go        cobra CLI + daemon (CGO_ENABLED=0)
  cmd/clockwork-orange-gui/main.go    Fyne entry (stdlib flag: --section --scheme --version)
  internal/buildinfo/                 Version, Commit via ldflags
  internal/config/                    YAML document, paths, migrations, watcher
  internal/core/                      Request/Result operations; the ONLY package both front ends import
  internal/engine/                    image selection, cycling, dual mode (used by core)
  internal/platform/                  wallpaper/lockscreen/monitors/service per OS (build-tagged files)
  internal/plugins/                   Plugin interface, registry, local, wallhaven, duckduckgo
  internal/store/                     history.db + blacklist.db (SQLite)
  internal/imaging/                   decode, cover-resize/crop, thumbnail, hashing
  internal/cli/                       cobra command tree
  internal/gui/                       Fyne design system + sections
  internal/gui/assets/                go:embed app icon
  tests/parity/                       CLI<->GUI parity guard (//go:build parity)
  tests/golden/                       fixtures captured from the Python 2.9.5 build
  packaging/                          .desktop, icons, arch/PKGBUILD, debian/, windows/, macos/
  Makefile  install.sh  uninstall.sh  config/.golangci-v2.12.2.yml
  ```
- R1.4 `Makefile` copied from nmsbonker with targets `help build build-gui
  build-all release test coverage lint install-lint install uninstall pkg-arch
  pkg-deb clean`; `VERSION := $(shell tr -d 'v[:space:]' < .tag)`;
  `LDFLAGS=-w -s -X …/internal/buildinfo.Version=$(VERSION) -X …/Commit=$(COMMIT)`.
- R1.5 `config/.golangci-v2.12.2.yml` copied from nmsbonker; only the
  `ignore-package-globs` module line changes. `make lint` passes with zero
  findings at every phase gate. `install-lint` keeps the SHA256-verified
  download.
- R1.6 Every file copied from angou/nmsbonker keeps its header comment
  `// Copied from nmsbonker (same author) — keep in sync by hand.` and its
  rationale comments.
- R1.7 `.claude/CLAUDE.md` is rewritten for the Go project (policies:
  `languages/go.md`, `languages/bash.md`, `git/standard.md`,
  `release-safety/simplified.md`, `security/owasp-review.md`,
  `testing/philosophy.md`, `communication/standards.md`); the Python
  override section is removed; Architecture rules copied from nmsbonker
  (parity, core-first, cancellable work, no reflow).

### R2. Configuration (`internal/config`)

- R2.1 Path: `~/.config/clockwork-orange.yml` on every OS (`os.UserHomeDir`
  + `.config`). Windows additionally probes
  `C:/Users/Public/clockwork_config.yml` second; first existing file wins;
  writers always target the primary path. State dir
  `~/.config/clockwork-orange/` holds `history.db` and `blacklist.db`.
- R2.2 Schema: a typed `Document` with every key from v2.9.5 —
  `desktop`, `lockscreen`, `dual_wallpapers` (bool); `default_wait` (int,
  GUI default 300, daemon default 900); `default_url`, `default_file`,
  `default_directory` (string); `console_font_family` ("Monospace"),
  `console_font_size` (10); `window_width` (800), `window_height` (600);
  `image_extensions` (string, default `.jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.svg`);
  `debug` (bool); `autostart` (bool); `restart_delay` (10);
  `logs_refresh_interval` (5); `auto_update_logs` (bool, omitted when false);
  `plugins` (map name → plugin block). Plugin blocks are `map[string]any`
  with `enabled` plus schema keys; unknown blocks are preserved (D6). Unknown
  top-level keys are preserved verbatim.
- R2.3 Write: `yaml.v3` encoder, keys sorted (matches Python
  `sort_keys=True`), 2-space indent, atomic write (temp file + rename) —
  v2.9.x wrote in place; atomic write is a deliberate deviation (DV3).
- R2.4 Migrations: idempotent predicate list run on every load, exactly one
  migration ported — `google_images` → `duckduckgo_images` (pop old block,
  pin `download_dir=~/Pictures/Wallpapers/GoogleImages` when absent/empty,
  store under new key). If any migration mutated, persist and log
  `[config-migration] Persisted migrated config to <path>` to stderr; on
  write failure log and continue with the in-memory document.
- R2.5 Merge with CLI args: `dual_wallpapers: true` forces `desktop` and
  `lockscreen`; `--wait` overrides `default_wait`; `--service` sets the
  daemon default of 900 when neither `--wait` nor `default_wait` is set.
- R2.6 Watcher: `fsnotify` on the config's **parent directory**
  (non-recursive), matching Create/Write/Rename/Chmod events whose resolved
  path equals the resolved config path; 2 s debounce implemented exactly as
  spec 009's `_drain_config_change_burst` (quiet-window restart per event,
  prompt return on shutdown). Unit tests from `tests/test_config_debounce.py`
  are ported 1:1 (single settled change, 5-write burst, shutdown).
- R2.7 `--write-config` writes `desktop/lockscreen/dual_wallpapers/default_wait`
  and the legacy `default_*` source keys from args, exit 0/1.

### R3. Engine (`internal/engine`, `internal/imaging`)

- R3.1 Image detection: extension set `{.jpg .jpeg .png .bmp .gif .tiff .webp .svg}`
  (case-insensitive) then `mime.TypeByExtension` prefix `image/`; Pillow is
  not involved in Python and no decode is done here either.
- R3.2 Fair source selection (`get_random_image_from_sources` port): resolve
  and filter existing sources (warn on missing), shuffle sources, walk in
  shuffled order, return the first candidate: file → itself if image; dir →
  uniform random over **non-recursive** image children; skip sources whose
  listing errors; error if none. Uses `math/rand/v2` with an injectable
  source for tests.
- R3.3 Per-monitor de-dup: for `monitorCount` monitors draw with up to **5**
  retries while the pick collides with an already-chosen image; >1 image →
  multi-monitor set, else single set. Dual mode: desktop images as above,
  then a lock-screen image retried up to 5 times against all desktop
  images; result is `desktopOK && lockOK`.
- R3.4 Directory helpers: `get_two_different_images_from_directory`
  (≥2 images, sample 2), `set_dual_wallpapers_from_files`.
- R3.5 URL source: `GET url` with 30 s timeout to a temp file with `.jpg`
  suffix; temp file is removed after a successful set (DV4).
- R3.6 Cycling loops: directory cycle, dual directory cycle, and dynamic
  multi-plugin cycle; each installs SIGINT/SIGTERM handling via
  `signal.NotifyContext`, sleeps in 1 s increments checking for shutdown,
  and the dynamic loop reloads config from disk every cycle, re-runs every
  enabled plugin (plugins self-throttle via `.last_run`), honours an updated
  `default_wait`, and is interrupted by the debounced config watcher.
- R3.7 Imaging: decode jpeg/png/gif/bmp/tiff/webp; `CoverResizeCrop(img, w, h)`
  with `x/image/draw.CatmullRom` (LANCZOS stand-in); JPEG encode with
  quality parameter; `Thumbnail(path, 128)` → JPEG q70 bytes; RGBA/paletted
  → RGB conversion as Pillow `convert("RGB")` does (alpha channel dropped,
  no compositing); hashing helpers `MD5File` (whole file) and `SHA256File`
  (streamed, 4096-byte blocks).

### R4. Platform layer (`internal/platform`, build-tagged files)

Linux (KDE Plasma 6):
- R4.1 Monitor count: `qdbus6 org.kde.plasmashell /PlasmaShell
  org.kde.PlasmaShell.evaluateScript "print(desktops().length)"`; parse int;
  any failure → 1.
- R4.2 Single wallpaper: the exact JS from `platform_utils.py:281-317`
  (`desktops().forEach(d => { d.currentConfigGroup = Array("Wallpaper","org.kde.image","General"); d.writeConfig("Image","file://PATH"); d.reloadConfig(); })`)
  via `qdbus6 … evaluateScript`. `PATH` is JS-string-escaped (DV1).
  Non-zero exit → log stderr, return false; `exec.ErrNotFound` → "qdbus6 not
  found", false.
- R4.3 Multi-monitor: `var images = ["file://p1", …]` assigned
  `images[i % images.length]` per desktop, same config group + `reloadConfig`.
- R4.4 Lock screen: `kwriteconfig6 --file kscreenlockerrc --group Greeter
  --group Wallpaper --group org.kde.image --group General --key Image
  "file://<abs>"`, then best-effort `qdbus6 org.freedesktop.ScreenSaver
  /ScreenSaver configure`. `--debug-lockscreen` dumps `~/.config/kscreenlockerrc`
  sections (INI parse) including the `Greeter][Wallpaper][org.kde.image][General`
  group and `Greeter/wallpaper`. Lock screen on Windows/macOS returns false
  with an `[INFO]` message.
- R4.5 systemd: `systemctl --user is-active|status --no-pager|start|stop|
  restart|daemon-reload|enable|disable clockwork-orange.service` with the
  same timeouts (5 s); `journalctl --user -u clockwork-orange.service
  --no-pager -n 50` (10 s); install copies the **embedded** unit to
  `~/.config/systemd/user/`, uninstall stops/disables/removes/reloads. The
  unit becomes:

  ```ini
  [Unit]
  Description=Clockwork Orange - Wallpaper Service
  After=graphical-session.target
  [Service]
  Type=simple
  Environment=DISPLAY=:0
  ExecStart=/usr/bin/clockwork-orange --service
  Restart=always
  RestartSec=10
  [Install]
  WantedBy=default.target
  ```
  (`-u` is no longer needed; Go stdout is unbuffered when line-written via
  `log`; the daemon must write logs unbuffered — R6.7.)

Windows:
- R4.6 Monitor enumeration via `EnumDisplayMonitors`/`GetMonitorInfoW`
  (replaces `screeninfo`), yielding `{x,y,w,h}`; failure → 1 monitor.
- R4.7 Multi-monitor: stitch a black RGB canvas over the monitor bounding
  box, cover-resize + centre-crop each image to its monitor, paste at
  `(x-minX, y-minY)`, save `%TEMP%\clockwork_spanned.jpg` (q90; fallbacks
  `%USERPROFILE%`, `.`), set registry `HKCU\Control Panel\Desktop`
  `WallpaperStyle="22"`, `TileWallpaper="0"`, call
  `SystemParametersInfoW(0x0014, 0, path, 0x03)`. Single wallpaper = the
  multi path with one image; on error fall back to
  `SystemParametersInfoW(20, 0, abs, 3)`.
- R4.8 `SetCurrentProcessExplicitAppUserModelID("ushineko.clockworkorange.gui.1.0")`
  at GUI start; single-instance mutex `Global\clockwork_orange_gui_lock`
  (`ERROR_ALREADY_EXISTS` → second instance exits 0; mutex creation failure
  → fail open with a log line).

macOS:
- R4.9 GUI binary: cgo/Objective-C `NSScreen.screens` count and per-screen
  `setDesktopImageURL:forScreen:options:error:` with
  `NSWorkspaceDesktopImageScalingKey = NSImageScaleProportionallyUpOrDown`,
  `NSWorkspaceDesktopImageAllowClippingKey = YES`, image `i % len`. CLI
  binary and any cgo failure: `osascript -e 'tell application "System
  Events" to set picture of every desktop to "<path>"'` and `count of
  desktops`.
- R4.10 Cache pruning: `du -sk ~/Library/Containers/com.apple.wallpaper.agent/Data/Library/Caches/com.apple.wallpaper.caches`
  (10 s timeout); if > 500 MB remove every entry; failures are warnings.
- R4.11 POSIX single-instance: `flock(LOCK_EX|LOCK_NB)` on
  `/tmp/clockwork_orange_gui_lock.lock`, held for process lifetime.

### R5. Plugins and stores (`internal/plugins`, `internal/store`)

- R5.1 Interface:

  ```go
  type Plugin interface {
      Name() string                       // "local", "wallhaven", "duckduckgo_images"
      Description() string
      Schema() []Field                    // ordered; Field{Key, Type, Description, Default, Required, Enum, Suggestions, Group, Widget}
      Run(ctx context.Context, cfg map[string]any, ev core.Events) (Result, error)
  }
  type Result struct{ Path string; Message string }  // Path is a file or directory
  ```
  Field types `string|boolean|integer|string_list`; widgets
  `file_path|directory_path`. Runtime keys `enabled`, `action`
  (`process_blacklist`), `targets`, `force`, `reset` are honoured exactly as
  in Python. Registry order: local, wallhaven, duckduckgo_images.
  `core.Events{Log(Level,string); Progress(pct int, msg string); ImageSaved(path string)}`
  replaces the `::PROGRESS::` / `::IMAGE_SAVED::` stderr protocol; the CLI
  renders them as the same text lines.
- R5.2 `local`: schema `path` (string, required, directory_path),
  `recursive` (boolean, default false — retained, unused); missing `path` →
  error; `process_blacklist` → blacklist `targets` as `plugin_name="local"`;
  nonexistent path → error; else `Result{Path: absolute expanded path}`.
- R5.3 `wallhaven`: schema keys and defaults exactly as `plugins/wallhaven.py:26-140`
  (`api_key`, `query` string_list default `landscape`, `sorting` enum
  default `relevance`, `top_range` default `1M`, `category_*` ×3 group
  "Categories", `purity_*` ×3 group "Purity", `resolutions`, `atleast`
  "2560x1440", `ratios` "16x9", `download_dir` `~/Pictures/Wallpapers/Wallhaven`,
  `interval` Hourly/Daily/Weekly default Daily, `limit` 10, `max_files` 100).
  `GET https://wallhaven.cc/api/v1/search` (10 s) with params `q apikey
  sorting order=desc page=1`, `topRange` only for toplist, `categories` and
  `purity` 3-char bitstrings, optional `resolutions atleast ratios`, 6-hex
  `seed` for random sorting. Query parsing accepts comma string or list of
  `{term,enabled}`/strings, empty → `["landscape"]`. Per item: filename
  `wallhaven-<id><ext>`; skip on `seen_url` or existing file; download
  (20 s); SHA-256 blacklisted → delete+skip; `seen_image` → delete,
  `add_entry`, skip; else `add_entry(url,path,"wallhaven")` + `ImageSaved`.
  `limit` applies **per query** (as in Python). `.last_run` in `download_dir`
  holds `time.Now().Unix()` as float text; `_should_run` semantics
  including "unknown interval → never", parse error → run, `force` bypass.
  Retention sorts **all** files by mtime and deletes the oldest beyond
  `max_files` (includes `.last_run`, as in Python). `reset` wipes the dir.
- R5.4 `duckduckgo_images`: schema (`query` default `4k nature wallpapers`
  with the 10 suggestions, `download_dir` `~/Pictures/Wallpapers/DuckDuckGo`,
  `interval` Daily, `limit` 10, `max_files` 50). Discovery: `GET
  https://duckduckgo.com/?q=…&iax=images&ia=images` (15 s), vqd regexes
  `vqd=["'](\d-[\d-]+)["']` then `"vqd":"(\d-[\d-]+)"`, then `GET
  https://duckduckgo.com/i.js` with `l=us-en o=json q vqd f=,,,size:Large,,
  p=1`, header `Referer: https://duckduckgo.com/` (15 s). Session headers
  `User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML,
  like Gecko) Chrome/132.0.0.0 Safari/537.36`, `Accept-Language:
  en-US,en;q=0.9`. Filter: dedupe by URL, reject reported dims
  `<1920×1080` (missing dims pass). Per image: skip `seen_url`; filename
  `md5(url)+".jpg"`; skip existing; download (10 s); decode; reject
  `<1920×1080`; cover-resize+crop to 3840×2160; JPEG q90; blacklist /
  history checks as R5.3; `add_entry(url,path,"duckduckgo_images")`.
  Retention globs `*.jpg` only. Return success with `download_dir` when
  anything downloaded or the dir is non-empty (interval-skip also returns
  success with the path), else error "No images found or downloaded".
- R5.5 History store: `~/.config/clockwork-orange/history.db`, table
  `downloads(id INTEGER PRIMARY KEY AUTOINCREMENT, url_hash TEXT, image_hash
  TEXT, source TEXT, timestamp REAL, UNIQUE(url_hash))`, index
  `idx_image_hash(image_hash)`; `url_hash = md5(url)`, `image_hash =
  md5(file bytes)`; `SeenURL`, `SeenImage` (false if file missing),
  `AddEntry` (false on unique violation), `Stats{Total, UniqueImages,
  DBSizeBytes}`, `Clear` (`DELETE` + `VACUUM`).
- R5.6 Blacklist store: `<state>/blacklist.db`, table `blacklist(img_hash
  TEXT PRIMARY KEY, source TEXT, timestamp REAL, thumbnail BLOB)`; SHA-256
  streamed 4096-byte blocks; thumbnail 128×128 JPEG q70; `Add(hash,
  plugin, path)` (`INSERT OR REPLACE`), `Remove`, `IsBlacklisted` (false on
  error), `Items()` ordered `timestamp DESC` with `date` formatted
  `2006-01-02 15:04` or "Unknown"; `ProcessFiles(paths, plugin)` adds then
  **removes the file**.
- R5.7 Golden compatibility: fixture DBs and YAML produced by the Python
  2.9.5 build are committed under `tests/golden/` and must open and query
  identically from Go; a Go-written DB must be readable by the Python
  `HistoryManager`/`BlacklistManager` (verified once in CI on Linux using
  the Python sources still present on the branch before cutover, then via a
  saved fixture).

### R6. CLI and daemon (`cmd/clockwork-orange`, `internal/cli`)

- R6.1 Root command accepts every v2.9.5 flag with identical semantics:
  `--lockscreen --desktop -u/--url -f/--file -d/--directory --plugin NAME
  --plugin-config JSON -w/--wait SECONDS --debug-lockscreen --write-config
  --gui --service --self-test`. `-u/-f/-d/--plugin` are mutually exclusive.
  `--run-plugin` is **dropped** (frozen-Python internal; D3). Unknown flags
  and validation errors exit **2** with the same messages as
  `_validate_args`; operation failure exits 1; success 0.
- R6.2 No-argument invocation launches the GUI (Windows double-click
  behaviour) by exec'ing `clockwork-orange-gui` found next to the CLI binary
  or on `PATH`; failure prints guidance and exits 1. `--gui` does the same.
- R6.3 Mode dispatch identical to `_perform_wallpaper_operation`: dual /
  lockscreen / desktop / default; explicit-source paths; `--wait` + `-d`
  cycles; no source and no `--plugin` → dynamic multi-plugin mode (one-shot
  or cycling with `--wait`). Dual + `--file` → exit 1; lockscreen with only
  `--plugin` → exit 1 with "requires either --file or --directory".
- R6.4 `--plugin NAME` runs the plugin, treats a dir result as `-d` and a
  file result as `-f`; invalid → exit 1. `--plugin-config` JSON is merged
  over the file config.
- R6.5 New subcommands (parity with GUI-only operations): `service
  install|uninstall|start|stop|restart|status|logs`, `blacklist list|remove
  <hash>|add <path…>`, `history stats|clear|import [dir]`, `plugins list`,
  `plugin run NAME [--force] [--reset]`, `config show|migrate`, `version`.
  Every subcommand maps to one `core` operation.
- R6.6 `--self-test`: prints Go version/OS/arch/binary path, verifies
  SQLite open in a temp dir, YAML round-trip, image codecs, platform tool
  presence (`qdbus6`, `kwriteconfig6` on Linux), a 5 s `HEAD
  https://www.google.com`, and the plugin registry; exit 0 iff all pass.
- R6.7 Logging: `log/slog` text handler on stderr with `[LEVEL]` bracketed
  prefixes matching the Python strings (`[DEBUG] [INFO] [WARN] [ERROR]`),
  unbuffered per line, level configurable by `--log-level` (default
  `debug`, matching v2.9.x which always printed `[DEBUG]`). Plugin
  `Events.Log` and `Progress` are rendered as `::PROGRESS:: <pct> :: <msg>`
  and `::IMAGE_SAVED:: <path>` lines for script compatibility.
- R6.8 `--service` sets the 900 s default wait and enables the config
  watcher; there is no daemonisation, PID file, or privilege change.
- R6.9 Additions over v2.9.5, all optional: persistent `--config PATH` and
  `--log-level LEVEL`; a hidden `--offline` that skips the self-test network
  probe (for containers without egress); `CLOCKWORK_ORANGE_GUI` names the GUI
  binary explicitly (build trees, tests); `CLOCKWORK_LOCK_DIR` relocates the
  single-instance lock files (test isolation). Ported as-is from
  `merge_config_with_args`: a missing config file contributes no
  `default_wait`, so `-d DIR` sets once, while a GUI-written file (which
  always carries `default_wait: 300`) makes the same command cycle; the
  config's `desktop`/`lockscreen` keys apply only when neither flag is given,
  `desktop` first; `-u URL` alone still requires enabled plugins to pass
  validation. The GUI is run as a child process whose exit status is
  returned, rather than exec(2)'d, so one code path serves Windows.

### R7. GUI (`cmd/clockwork-orange-gui`, `internal/gui`)

Design system (copy from nmsbonker unless noted):
- R7.1 Copy verbatim: `theme.go`, `fonts.go`, `cursor_linux.go`,
  `cursor_other.go`, `views_table.go`, `views_appearance.go` (Sample text
  changed), `icon.go` + `assets/assets.go` (SVG swapped for the Clockwork
  Orange logo), `dialogs.go` minus `luaFilter`. Copy skeletons: `app.go`
  (`section`, `Options`, `Run`, prefs, redraw levels, busy/flash, the
  shared-widget block), `model.go` (`Status` iota), `state.go`
  (`request/report/ok/invalidate/onScreen/perform`, log pump, cancel),
  `buildlog.go` + `views_build.go` (renamed `run`), `views_about.go`,
  `views_settings.go`.
- R7.2 Shell: `app.NewWithID("io.ushineko.clockwork-orange")`; window
  title `"Clockwork Orange <version>"`; `Border{top: header, bottom:
  statusBar, center: HSplit(nav list 0.16, scroll)}`; default `1180×760`
  but **restore `window_width/height` from config** and persist on resize
  (debounced 500 ms) to keep the v2.9.x behaviour; `SetMaster`;
  `applyCursorTheme()` before toolkit init; F5/Ctrl+R reload; no menu bar.
  Busy = centred modal popup after 300 ms; flash = floating non-modal popup
  (good 6 s, warn 12 s, bad until dismissed). Nothing transient reflows the
  layout.
- R7.3 Sections (nav order): **Service** (Linux) or **Activity** (Windows/
  macOS); **Local**, **Wallhaven**, **DuckDuckGo Images** (one per
  registered plugin; title = name with `_`→space, Title Case); **History**;
  **Blacklist**; **Settings**; **Appearance**; **About**. `--section NAME`
  opens a section; `gui.SectionNames()`, `gui.SchemeNames()`,
  `gui.Actions()` exported.
- R7.4 Service section (Linux): status line with `marker(Status)` mapping
  active→Good, inactive→Bad, activating/deactivating→Warn, failed→Bad,
  other→Info; details pane (`systemctl status`); toolbar Start/Stop/Restart/
  Install/Uninstall with the same enablement matrix as
  `service_manager.py:230-299`; log pane (`buildlog.go` log model,
  journalctl tail, "Refresh now", auto-refresh toggle + interval spinner
  bound to `auto_update_logs` / `logs_refresh_interval`, 5 s status poll,
  follow-tail preservation). Activity section (Windows/macOS): the same log
  pane fed by the in-process slog handler, max 1000 lines, Clear button,
  "Last updated HH:MM:SS | N entries".
- R7.5 Plugin section: `heading(description)`; "Enable this plugin" check;
  `widget.NewForm` generated from `Schema()` — string → Entry, or Select
  (enum) / editable Select-with-suggestions; boolean → Check; integer →
  numeric Entry validated 0–10000; string_list → a `searchTerms` widget
  (checkable list + editable entry with suggestions + Add/Remove; value
  `[]{term,enabled}`; accepts legacy comma string); `file_path`/
  `directory_path` → `withBrowse` (+ "Open" via `openPath` for dirs);
  consecutive same-`group` fields on one row. Toolbar: **Download Now**
  (disabled for `local`), **Reset & Run** (confirmDestructive), **Apply
  Blacklist (n)** (DangerImportance, disabled at 0). Runs open a
  `run`-based dialog: progress bar 0–100 from `Events.Progress`, log pane,
  live preview of the last `ImageSaved`, optional term checklist that
  overrides `query` before start, Start/Close/Cancel (context cancel).
- R7.6 Review mode (in the plugin section): scans `path`/`download_dir`
  (fallback schema default) for `*.jpg *.jpeg *.png *.webp *.bmp` sorted by
  mtime desc; `canvas.Image` `ImageFillContain` preview; info panel
  "Image i of n", filename, relative time (Just now / N mins ago / N hours
  ago / N days ago / `2006-01-02`), `[MARKED FOR DELETION]`; keyboard ←/→
  navigate, Space toggles mark; marked images render a red border plus
  both diagonals (pen `max(5, 2% of min dimension)`) composited in Go;
  `fsnotify` on the directory with 500 ms debounce re-scans; Apply runs the
  plugin with `action=process_blacklist, targets, force=true` then
  re-scans.
- R7.7 History section: stats form (Total Downloads Tracked, Unique Images,
  Database Size B/KB/MB), refreshed on show and every 5 s; **Reset History
  Database** (confirmDestructive); **Scan & Import Existing Files** (reads
  `plugins.duckduckgo_images.download_dir`, globs `*.jp*g`
  case-insensitive, inserts `file://imported/<name>` with source
  `imported`, cancellable via the busy popup, reports imported/skipped).
- R7.8 Blacklist section: filter Entry (live, case-insensitive across
  columns) + Refresh; `detailTable` columns **Thumb / Hash / Date Added /
  Source Plugin** with 64 px thumbnails rendered from the BLOB (custom cell
  since `detailTable` is text-only: extend it with an optional image column
  and keep the "set Importance before SetText" note); multi-select; **Remove
  Selected from Blacklist** (DangerImportance, confirm).
- R7.9 Settings section: `settingsForm` shape from nmsbonker (widgets held
  separately, `set/values` round-trip). Basic: mutually-exclusive checks
  dual / desktop-only / lockscreen-only; Wait Interval 1–86400 (300);
  Console font Select from `fontNames()` (default Monospace); font size
  6–48 (10). Advanced: Image Extensions entry; Debug check; on non-Windows
  Auto-start check, Restart Delay 1–300, Logs Refresh Interval 1–300,
  Auto-update Logs check. Raw YAML: read-only monospace view of the current
  document with **Validate** (parse) and **Copy** buttons (v2.9.x "Format"
  never wrote back; kept read-only). Auto-save: any change schedules a
  1000 ms coalesced save of the whole document (Basic + Advanced + every
  plugin form) followed by flash "Saved" and a re-arm of the wallpaper
  timer.
- R7.10 About section: 72 px logo, "Clockwork Orange", tagline "My choice
  is your imperative", version (R7.12), "© 2025 github.com/ushineko",
  `aboutLink` to the repo, and README rendered with
  `widget.RichTextFromMarkdown` from an embedded copy (no anchor injection;
  DV5).
- R7.11 Tray and lifecycle: `desktop.App.SetSystemTrayMenu` with Show /
  About / Quit and `SetSystemTrayIcon`; `SetCloseIntercept` hides to tray
  and sends a notification "Minimized to tray" (skips hide when no tray is
  available and quits instead); `fyne.App.SendNotification` for "Application
  started", "Saved", "Service running/stopped"; Quit stops timers, releases
  the single-instance lock and exits. Second instance exits 0 silently.
- R7.12 Wallpaper timer (all OSes): interval `default_wait` s (300), first
  fire 2 s after start, skipped while a run is in progress; the run calls
  **the same `core.Cycle` operation the CLI uses** (DV2) and streams its
  log lines into the Activity/Service log pane. Version string: ldflags
  `Version`; when empty read `.tag` next to the binary; when a `.git` dir is
  present append `-r<count>-<short>`; fallback "Unknown".
- R7.13 Every core call runs in a goroutine inside `u.busy(what)` /
  `u.perform`, hops back with `fyne.Do`; `fyne.DoAndWait` is not used; all
  state on `*ui`; `…OK` loaded flags; sections rebuilt on refresh.
- R7.15 Choices made in Phase 5, all within R7: the window opens at
  1180×760 when the config file is absent or carries no size (the Python
  restored 800×600 from `Defaults()`); the plugin section title follows the
  stated rule literally (`Duckduckgo Images`); the Blacklist table's
  multi-select is a click-to-toggle check column, since Fyne's table selects
  one cell; the string_list editor removes a term with a per-row button;
  integer settings are validated Entries (Fyne has no spin box); the window
  size is polled every 500 ms and persisted through the auto-save (Fyne has
  no resize callback); Apply Blacklist runs the plugin's `process_blacklist`
  action (R7.6), which is the `blacklist add` operation in `gui.Actions()`;
  the status bar says "timer idle" while the daemon holds the cycling lock
  (DV10). `golang.org/x/net` is bumped to v0.56.0 (indirect, via Fyne) to
  clear GO-2026-4918 through GO-2026-5942 from the GUI binary.
- R7.14 Headless tests with `test.NewApp()` and the `testUI(t)` helper for:
  schema→form generation for all four field types, searchTerms round-trip
  incl. legacy string, review-mode keyboard marking, auto-save coalescing,
  service enablement matrix, blacklist filter, flash lifetimes. Test names
  are sentences naming the bug prevented.

### R8. Build, packaging, CI

- R8.1 `Makefile` targets as R1.4; `make build` produces
  `bin/clockwork-orange` with `CGO_ENABLED=0`; `make build-gui` produces
  `bin/clockwork-orange-gui`; `make release` stages per-target tarballs
  and `SHA256SUMS`.
- R8.2 Arch: `packaging/arch/PKGBUILD` (package `clockwork-orange-git`,
  `arch=('x86_64')`, `makedepends=(git go gcc)`, `depends` = Fyne runtime
  set from nmsbonker (`libgl libx11 libxcursor libxrandr libxinerama libxi
  libxkbcommon libxkbcommon-x11 wayland`) **plus** `qt6-tools kconfig`
  for `qdbus6`/`kwriteconfig6`; `options=('!strip' '!debug')`; `pkgver()`
  reads `.tag` and emits `<ver>.r<count>.g<sha>`; installs both binaries
  to `/usr/bin`, the desktop file to `/usr/share/applications/
  io.ushineko.clockwork-orange.desktop`, hicolor icons 16–512, the systemd
  user unit to `/usr/lib/systemd/user/`, LICENSE, README; `check()` runs
  `go test -tags parity`). `clockwork-orange.install` retained (icon cache,
  desktop DB, `pre_remove` stops the user unit). The AUR job in
  `build.yml` regenerates `.SRCINFO` with `makepkg --printsrcinfo` inside
  the arch container instead of the hand-written heredoc (DV6) and keeps
  `AUR_SSH_KEY` handling exactly as today.
- R8.3 Debian: `packaging/debian/` builds from pre-built binaries with a
  staged tree and `dpkg-deb --build` (no `dh-golang`); `Depends:
  libgl1, libx11-6, libxcursor1, libxrandr2, libxinerama1, libxi6,
  libxkbcommon0, qt6-tools-dev-tools | qdbus-qt6, kconfig` (verify exact
  Debian package names during Phase 5); version from `.tag` + rev count +
  sha as `build_deb.sh` does today.
- R8.4 Windows: `go build -ldflags "-H=windowsgui …"` for the GUI, plain
  build for the CLI; icon and version resource embedded via `fyne package
  -os windows` or `go-winres` (choose in Phase 5, record in spec); artifact
  `clockwork-orange-windows-amd64.zip` containing both `.exe`s; CI runs
  `clockwork-orange.exe --self-test` as the gate (replaces the PyInstaller
  self-test).
- R8.5 macOS: `fyne package -os darwin -name "Clockwork Orange" -icon
  clockwork-orange.icns -appID io.ushineko.clockwork-orange` producing
  `Clockwork Orange.app` with the CLI binary copied into
  `Contents/MacOS/`; zipped as `Clockwork-Orange-macOS.zip`; self-test
  gate. Unsigned/un-notarised as today (README keeps the right-click →
  Open note).
- R8.6 CI `.github/workflows/build.yml`: jobs `test` (ubuntu; apt Fyne
  headers; `make test`, `make lint`, `CGO_ENABLED=0 make build`),
  `build-arch` (container, builder user, makepkg, install + `--self-test`
  smoke), `build-deb`, `build-windows` (windows-latest, Go, MSYS2/mingw
  gcc for CGO, self-test), `build-macos` (macos-latest, Go, fyne package,
  self-test), `release` (tags only; verifies tag == `.tag`; uploads all
  artifacts with `softprops/action-gh-release@v2`), `publish-aur` (tags
  only). Triggers unchanged (push main/master, tags `v*`, PRs, dispatch).
- R8.7 Desktop entry `packaging/io.ushineko.clockwork-orange.desktop`:
  basename equals the Fyne app ID (Wayland taskbar icon; keep the
  load-bearing comment from nmsbonker), `Exec=clockwork-orange-gui`,
  `Icon=clockwork-orange`, `StartupWMClass=clockwork-orange-gui`,
  categories/keywords from the current file. `install.sh`/`uninstall.sh`
  from nmsbonker adapted (installs both binaries, desktop file, icon, and
  the user unit; `--dry-run`, `--no-gui`).
- R8.8 `release_version.sh` unchanged. `.tag` bumped to `v4.0.0` only in
  the release step (release workflow in `.claude/CLAUDE.md` still applies).
- R8.9 Removed at cutover: all `*.py`, `requirements.txt`, `gui/`,
  `plugins/`, `scripts/*.ps1`, `scripts/*.py`, `scripts/setup_stable_diffusion.sh`,
  `build_deb.sh`, `build_local.sh`, `run_*.sh`, `.flake8`, `PKGBUILD`
  (root), `debian/` (root), `clockwork-orange.desktop*`,
  `install-desktop-entry.sh`, `test_platform_utils_build.py`,
  `repro_interval.py`, `docs/generate_screenshots.py`. Kept: `.tag`,
  `release_version.sh`, `clockwork-orange.install`, `scripts/update_aur.sh`
  (paths updated), `LICENSE`, `README.md`, `GUI.md`, `WALKTHROUGH.md`,
  `img/`, `specs/`, `validation-reports/`.

### R9. Testing

- R9.1 Unit tests per package with `go test -race -tags parity ./...`;
  coverage target ≥ 70 % for `internal/{config,engine,imaging,plugins,store,core}`.
- R9.2 Golden tests (`tests/golden/`): captured from Python 2.9.5 before
  cutover — sample `clockwork-orange.yml` variants (pre- and
  post-migration), `history.db` and `blacklist.db` with known rows, a
  Wallhaven `_build_api_params` table, DDG filename/md5 pairs, blacklist
  SHA-256 for fixture images, thumbnail dimensions, the KDE JS script text
  for given paths, and `kwriteconfig6` argv. Go output must match
  byte-for-byte where Python output is deterministic.
- R9.3 Integration tests (env-gated, skipped when absent):
  `CLOCKWORK_LIVE_KDE=1` runs `qdbus6`/`kwriteconfig6` against the real
  session and reads back the written keys; `CLOCKWORK_LIVE_NET=1` performs
  one real Wallhaven search and one real DDG vqd + `i.js` round-trip;
  `CLOCKWORK_LIVE_SYSTEMD=1` installs/starts/stops/uninstalls the user unit
  in a throwaway `XDG_CONFIG_HOME`.
- R9.4 Parity guard `tests/parity/parity_test.go`: every cobra leaf maps
  to a `gui.Actions()` entry or the documented allow-list (`--self-test`,
  `--debug-lockscreen`, `--write-config`, `config migrate`, `version`).
- R9.5 Headless GUI tests per R7.14.
- R9.6 Manual platform verification before release (recorded in the
  validation report): Windows 10/11 multi-monitor spanned set, macOS 13+
  per-screen set and cache prune, KDE Plasma 6 desktop + lock screen, tray
  on all three, systemd install → reboot → wallpaper changes.

### R10. Documentation and cutover

- R10.1 `README.md` rewritten for Go: install (pacman/AUR, deb, Windows
  zip, macOS app, `go install`), both binaries, all flags and subcommands,
  config reference, plugin reference (SD section replaced by a
  "deferred to a future release; your `stable_diffusion` settings are
  preserved" note), service setup, troubleshooting.
- R10.2 `docs/architecture.md` cloned from nmsbonker (package map + rules).
- R10.3 `specs/README.md` updated with 007–010 and the new status.
- R10.4 The memory note about the live systemd unit symlink into dotfiles
  applies: after cutover the dotfiles copy of `clockwork-orange.service`
  must be updated to `ExecStart=/usr/bin/clockwork-orange --service`
  (operator step, recorded in the validation report).

---

## Deliberate Deviations from v2.9.5

Everything not listed here is ported as-is, including known quirks (Wallhaven
`limit` per query, retention deleting `.last_run`, DDG error status on an
empty directory, `history` MD5 vs blacklist SHA-256, inert `debug` /
`image_extensions` / `autostart` / `restart_delay` keys).

| # | Deviation | Rationale |
|---|-----------|-----------|
| DV1 | JS-string-escape the file path interpolated into the KDE `evaluateScript` payload | Project security extension "validate paths"; a `"` in a path currently breaks the script |
| DV2 | GUI timer uses the shared engine (all image extensions, per-monitor de-dup) instead of the separate `*.jpg`/`*.png`-only worker | CLI/GUI parity rule; removes a second selection algorithm |
| DV3 | Atomic config writes (temp + rename) and a process-wide write mutex | Removes the write-storm race between auto-save, migration persist and `auto_update_logs` save |
| DV4 | Delete the temp file after a URL-sourced set succeeds | v2.9.x leaked one file per run |
| DV5 | About/README uses Fyne RichText without heading anchors | No anchor support in Fyne; the Qt hack is not portable |
| DV6 | `.SRCINFO` generated by `makepkg --printsrcinfo` | Hand-written heredoc drifted from PKGBUILD (already lists SD optdepends that v4 drops) |
| DV7 | `clean_config` no longer deletes unknown plugin blocks (D6) | Preserve `stable_diffusion` settings |
| DV8 | `--run-plugin` removed; plugins run in-process (D3) | No frozen interpreter to re-enter |
| DV9 | Config format has no `ddgs` library path; always direct scrape (D9) | No Go equivalent |
| DV10 | Single-instance lock also guards the daemon (`/tmp/clockwork_orange_service_lock.lock`) so GUI-timer and systemd cycling do not both run on Linux; GUI skips its timer when the daemon lock is held | v2.9.x changed wallpapers twice per period when both ran |
| DV11 | `Blacklist.ProcessFiles` keeps the file when the database write fails and returns the error | Python swallowed the DB error and deleted the image anyway, losing it without recording it |
| DV12 | Duplicate-content branch in Wallhaven/DDG records the history entry *before* deleting the duplicate file | Python deleted first, then `add_entry` raised on the missing file, so the URL was never recorded and re-downloaded every run |
| DV13 | Wallhaven API key is redacted from log lines; POSIX single-instance lock fails open on non-contention errors; `DebugLockscreen` reads `$XDG_CONFIG_HOME/kscreenlockerrc` and preserves key case | Security extension "no credentials in logs"; consistency with the Windows fail-open path; that is where `kwriteconfig6` writes |
| DV14 | `ToRGB`/thumbnail/cover-resize flatten alpha by dropping the channel (Pillow `convert("RGB")` semantics) before scaling | The first Go draft composited through premultiplied RGBA and darkened translucent pixels |

---

## Implementation Phases

Each phase ends with: tests green, `make lint` clean, security review
(`/ralph-security-review`), validation report (strict), spec AC boxes
checked for that phase, one or more commits on `v4-go-rewrite`. Phases are
sequential; a phase may be split into a child spec if it exceeds ~10 AC.

| Phase | Scope | Requirements |
|-------|-------|--------------|
| 0 | Approve this spec; capture golden fixtures from Python 2.9.5 | R9.2 |
| 1 | Skeleton: `go.mod`, layout, Makefile, lint, buildinfo, CI `test` job, `.claude/CLAUDE.md` | R1, R8.6 (test job) |
| 2 | `internal/config` (schema, YAML I/O, migration, watcher/debounce), `internal/imaging`, `internal/store` | R2, R3.7, R5.5–R5.7 |
| 3 | `internal/platform` (Linux first, then Windows, macOS), `internal/engine`, `internal/core` | R3.1–R3.6, R4 |
| 4 | `internal/plugins` (local, wallhaven, duckduckgo), `internal/cli`, `cmd/clockwork-orange`, daemon mode, self-test, parity allow-list | R5.1–R5.4, R6, R9.4 |
| 5 | GUI: design system import, shell, sections, tray, review mode, headless tests | R7, R9.5 |
| 6 | Packaging + CI for all platforms, installers, desktop entry, AUR job | R8 |
| 7 | Cutover: delete Python, README/docs rewrite, manual platform verification, `.tag` → `v4.0.0`, merge to `main`, release | R8.9, R9.6, R10 |

---

## Acceptance Criteria

### Phase 0–1: Foundations
- [x] `go build ./...` with `CGO_ENABLED=0` builds `cmd/clockwork-orange`; `make build-gui` builds `cmd/clockwork-orange-gui` on the dev machine.
- [x] `make lint` passes with the copied golangci config; `make test` runs with `-race -tags parity`.
- [x] `.claude/CLAUDE.md` lists the Go policy set and the architecture rules; the Python override section is gone.
- [x] Golden fixtures exist under `tests/golden/` with a README stating the Python commit (`67ad8d2`) and commands used to capture them.

### Phase 2: Config, imaging, stores
- [x] A v2.9.5 `clockwork-orange.yml` containing a `stable_diffusion` block and a `google_images` block loads, migrates `google_images` → `duckduckgo_images` with the pinned `download_dir`, is written back with sorted keys, and **still contains the `stable_diffusion` block** (golden diff).
- [x] Debounce tests ported from `tests/test_config_debounce.py` pass (settled change, 5-write burst, shutdown) against the fsnotify-backed watcher.
- [x] **Integration boundary**: Go opens the Python-created `history.db` and `blacklist.db` fixtures and returns identical `Stats`/`Items()`; a Go-created DB is opened by `plugins/history.py` and `plugins/blacklist.py` (one-time CI step on Linux while Python is still on the branch) with matching rows. *Implemented as: Go-written DB has byte-identical `sqlite_master` to the Python fixture, and a Go replay of `capture.py` reproduces `expected.json`; the Python-reads-Go step is run manually in Phase 7 verification.*
- [x] `SHA256File`/`MD5File` match Python hashes for the fixture images; thumbnails are ≤128×128 JPEG; `CoverResizeCrop` yields exact target dimensions for portrait, landscape and equal-aspect inputs.

### Phase 3: Platform, engine, core
- [x] The generated KDE JS for single and multi-monitor paths equals the golden text for paths without special characters, and escapes `"`/`\` in paths (DV1) with a test.
- [x] `kwriteconfig6` argv equals the golden argv; the screensaver reload is best-effort (non-zero exit does not fail the set).
- [x] **Integration boundary** (`CLOCKWORK_LIVE_KDE=1`, dev machine): setting a wallpaper and a lock-screen image via the real `qdbus6`/`kwriteconfig6` succeeds and `~/.config/kscreenlockerrc` contains the written `Image` key. *Run 2026-09-15 on the dev machine: pass; the previous lock-screen image is restored by the test.*
- [x] Fair selection: with a seeded RNG, a 10-image source and a 10 000-image source are chosen with equal frequency over 10 000 draws (±2 %); de-dup retries stop at 5. *Test uses 4 000 draws and ±3 % over a 10 vs 200 image pair; same contract.*
- [x] Windows: composite canvas dimensions equal the monitor bounding box and each image is placed at `(x-minX, y-minY)` (unit test with fake monitors); registry and SPI calls are behind an interface and exercised by a fake.
- [x] macOS: cache-prune logic removes entries only when `du` reports >500 MB (unit test with fake `du`).
- [x] `core` exposes Request/Result operations for every CLI and GUI action (`Cycle`, `SetFromFile/Directory/URL`, `RunPlugin`, `Service*`, `Blacklist*`, `History*`, `ConfigLoad/Save`), each accepting `context.Context` and `core.Events`.

### Phase 4: Plugins, CLI, daemon
- [x] `_build_api_params` golden table passes (all sorting/topRange/categories/purity/optional-param combinations); query parsing accepts comma string, `{term,enabled}` list and bare strings, and defaults to `landscape`.
- [x] DDG: filename is `md5(url).jpg`; images below 1920×1080 are rejected before and after decode; output is 3840×2160 JPEG; vqd regex fallbacks are unit-tested against saved HTML.
- [x] `.last_run` interval logic matches Python for Hourly/Daily/Weekly/always/unknown/parse-error/force (table test).
- [x] **Integration boundary** (`CLOCKWORK_LIVE_NET=1`): one real Wallhaven search returns ≥1 item and one real DDG vqd + `i.js` round-trip returns results. *Run 2026-09-15: both pass.*
- [x] Every v2.9.5 flag is accepted with identical validation messages and exit codes (table test over the `_validate_args` cases); `--run-plugin` is rejected as unknown (exit 2).
- [x] `clockwork-orange` with no args and with `--gui` execs `clockwork-orange-gui`; missing GUI binary exits 1 with guidance.
- [x] `--service` runs the dynamic cycle with a 900 s default, reloads config every cycle, and a settled config edit interrupts the wait within ~2 s (integration test with a temp `HOME`).
- [ ] `service install|uninstall|start|stop|restart|status|logs` work against systemd in a throwaway `XDG_CONFIG_HOME` (`CLOCKWORK_LIVE_SYSTEMD=1`). *Env-gated test exists (`internal/cli/service_live_test.go`); it skips on the dev machine because the production unit is active. Run in the Phase 6 container or after cutover.*
- [x] `--self-test` exits 0 on the dev machine and 1 when a probe fails (fault-injected test).
- [x] Parity test passes with the documented allow-list. *Phase 4 shape: every leaf maps to one core operation and every exception carries a reason; the `gui.Actions()` comparison lands with the GUI in Phase 5.*

### Phase 5: GUI
- [x] Design-system files carry the "Copied from nmsbonker" header; `theme.go`/`fonts.go`/`cursor_*.go`/`views_table.go` diff against nmsbonker only in the header comment and module path. *Verified with `diff` modulo the module path; the copied files keep their original "Copied from angou" header line, as R1.6 asks. `views_table.go` additionally carries the optional thumbnail column (R7.8).*
- [x] All sections in R7.3 render headlessly; `--section` opens each by name; `gui.Actions()` covers every core operation.
- [x] Plugin form generation is tested for the four field types, `enum` vs `suggestions`, `group` row packing, and both `widget` kinds; `searchTerms` round-trips legacy comma strings.
- [x] Review mode: ←/→/Space behave as specified in a headless test; marked images produce a red border + diagonals overlay; Apply sends `action=process_blacklist` with the marked paths.
- [x] Auto-save coalesces rapid edits into one write ≥1 s after the last change and preserves unknown plugin blocks (D6).
- [x] Service section enablement matrix matches `service_manager.py:230-299` (table test); log pane keeps scroll position unless following the tail.
- [x] Window size restores from `window_width/height` and persists after a resize (headless test with `test.NewWindow`).
- [x] Tray: close intercept hides the window and sends a notification when a tray is available; quits when not (fake `desktop.App`).
- [ ] Manual: the GUI runs on KDE Plasma 6 (Wayland and X11) with the Breeze Dark palette, correct taskbar icon, tray icon present.

### Phase 6: Packaging and CI
- [ ] `makepkg` in the arch container builds `clockwork-orange-git`, installs both binaries, the desktop file, icons and the user unit; `clockwork-orange --self-test` passes inside the container.
- [ ] `.deb` installs on Ubuntu 24.04 and `--self-test` passes.
- [ ] Windows CI builds both `.exe`s with CGO, embeds the icon, and `--self-test` passes; the GUI exe has the windowsgui subsystem.
- [ ] macOS CI produces `Clockwork Orange.app` via `fyne package`, includes the CLI binary, and `--self-test` passes.
- [ ] `release` job refuses when the git tag differs from `.tag`; `publish-aur` regenerates `.SRCINFO` with `makepkg --printsrcinfo` and pushes only when changed.
- [ ] `install.sh --dry-run` lists exactly the files it would install; `uninstall.sh` removes exactly those.

### Phase 7: Cutover and release
- [ ] All Python sources and Python-only tooling listed in R8.9 are deleted in one commit; `git grep -l "python"` in the tree returns only historical specs, validation reports and this spec.
- [ ] README, `docs/architecture.md`, `GUI.md`, `specs/README.md` updated; SD section replaced by the deferral note.
- [ ] Manual platform verification (R9.6) completed on Windows, macOS and KDE and recorded in `validation-reports/`.
- [ ] Dotfiles systemd unit updated to the new `ExecStart` and the live user service restarted (operator step recorded).
- [ ] `.tag` = `v4.0.0`; `release_version.sh` tags and pushes; GitHub Actions publishes Arch, deb, Windows zip, macOS zip; AUR updated.
- [ ] Security review (dependency scan via `govulncheck`, OWASP pass on network code, no secrets) recorded for the release commit.

---

## Testing

See R9. Summary of the contract each layer encodes:

- **Config**: files written by 2.9.5 load and round-trip; migrations are
  idempotent; unknown data survives; watcher debounce timings.
- **Engine**: fairness across sources, de-dup bound, dual-mode disjointness.
- **Platform**: exact external command lines (golden), fake-backed Windows/
  macOS APIs, env-gated live KDE/systemd runs.
- **Plugins**: HTTP query construction, response filtering, filename and
  hash derivation, interval scheduling, retention, blacklist/history
  interplay (using `httptest.Server` for the network).
- **Stores**: byte-compatibility with Python-created SQLite files.
- **CLI**: flag parsing, validation messages, exit codes, mode dispatch.
- **GUI**: headless behaviour tests, parity guard.
- **Packaging**: CI self-test gate on every artifact.

Mock boundaries: the network (`httptest`), OS APIs (interfaces with fakes),
the clock and RNG (injected). Not mocked: the filesystem, SQLite, YAML.

---

## Risks & Assumptions

- **Rollback**: users reinstall a v2.9.x package from GitHub Releases / AUR
  history; config and DBs are format-compatible in both directions (R5.7,
  D4, D6), so no data migration is needed to roll back. `main` keeps the
  Python line until merge; the merge commit is a single revert point.
- **Fyne on Windows/macOS CI needs CGO toolchains** (mingw on
  windows-latest, Xcode CLT on macos-latest). Assumed available on GitHub
  hosted runners; verified in Phase 1's CI job before GUI work starts.
- **KDE Plasma dependency on `qdbus6`/`kwriteconfig6` remains** (no D-Bus
  library port in v4.0 to keep the straight-port promise). A future spec
  may replace them with `godbus`.
- **Tray on Wayland/KDE** relies on `fyne.io/systray` StatusNotifier
  support; if it misbehaves, the fallback is "no tray → close quits"
  (R7.11 already specifies this branch).
- **`modernc.org/sqlite` VACUUM and `INSERT OR REPLACE`** behave as
  upstream SQLite; assumed, verified by the golden DB tests.
- **DuckDuckGo scraping** can break at any time; unchanged risk from 2.9.x.
- **Debian dependency names** for `qdbus6`/`kwriteconfig6` vary by release;
  to be pinned in Phase 6.
- **Binary size**: Fyne GUI ≈ 30–40 MB; acceptable versus the current
  PyInstaller bundles (>150 MB).
- **Assumption**: "Major new feature 4.0" in the brief means this release
  is v4.0.0 (D1). If a separate 4.0 feature exists, it is out of scope here.
- **Assumption**: the reviewer accepts the deviations DV1–DV10; any rejected
  deviation is reverted to a straight port before Phase 3.

---

## Alternatives Considered

- Single CGO binary with `--gui` (D2): rejected because the daemon would
  depend on OpenGL/X11 libraries on a headless-ish systemd session and could
  not be built `CGO_ENABLED=0` for static releases.
- Keeping the subprocess plugin protocol (D3): rejected; no external plugin
  has ever been written for it and it would require shipping a scripting
  runtime or a second binary per plugin.
- JSON config in `$XDG_CONFIG_HOME` (D4): rejected for v4.0 to guarantee
  zero-touch upgrade and rollback; may be revisited with an importer.
- `IDesktopWallpaper` COM on Windows (D10): rejected as a behaviour change;
  the composite technique is what users have today.
- Separate repository (D8): rejected; releases, AUR package name and issue
  history stay attached to this repo.

---

## Executive Summary

*(Populate before opening the merge to `main` at Phase 7.)*

---

## Notes / Open Questions for Review

1. Confirm D1–D12 and DV1–DV10, or mark overrides inline.
2. Should the `clockwork-orange-git` AUR package be renamed (e.g.
   `clockwork-orange-bin` for release binaries) or stay `-git` building from
   source? Default: stay `-git`, build from source with `go`.
3. Should v4.0 ship a `history`/`blacklist` CLI at all, or is GUI-only
   acceptable with a larger parity allow-list? Default: ship the CLI (R6.5).
4. Windows icon embedding: `fyne package` vs `go-winres` — decided in
   Phase 6 unless the reviewer has a preference.
5. Whether child specs (011–016) are wanted per phase or the AC in this
   document suffices for Ralph loop mode.
