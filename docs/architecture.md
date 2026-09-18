# Architecture (spec 010)

Two binaries over one core. `cmd/clockwork-orange` (cobra CLI + daemon,
`CGO_ENABLED=0`) and `cmd/clockwork-orange-gui` (Fyne) import only
`internal/core`, which orchestrates the packages below. Nothing outside
`internal/core` and the front ends may import `internal/core`.

```
cmd/clockwork-orange ─┐
                      ├─► internal/core ─► engine, plugins, store, platform, config, imaging, events
cmd/clockwork-orange-gui ┘
```

Dependency direction (leaf first): `events`, `buildinfo` → `imaging` →
`config` → `store` → `platform` → `plugins` → `engine` → `core` → `cli`/`gui`.
A package may import anything to its left, never to its right.

## Package contracts

These signatures are the integration contract between concurrently developed
packages. Change them only by editing this file in the same commit.

### `internal/events` (exists)

`Level` (`LevelDebug|Info|Warn|Error`, `String()` → `"[DEBUG]"` …),
`Events{OnLog, OnProgress, OnImageSaved}` with methods `Log`, `Logf`, `Debugf`,
`Infof`, `Warnf`, `Errorf`, `Progress(pct int, msg string)`, `ImageSaved(path)`.

### `internal/imaging` (core exists; hashing/thumbnail to add)

```go
func Decode(r io.Reader) (image.Image, string, error)
func DecodeFile(path string) (image.Image, string, error)
func ToRGB(src image.Image) *image.RGBA
func CoverResizeCrop(src image.Image, w, h int) *image.RGBA
func EncodeJPEG(w io.Writer, img image.Image, quality int) error
func SaveJPEG(path string, img image.Image, quality int) error
// to add (R3.7, R5.6):
func Thumbnail(path string, maxSide int, quality int) ([]byte, error) // Pillow thumbnail((n,n)) semantics: fit within, keep aspect, never upscale; JPEG
func MD5File(path string) (string, error)     // whole file, hex
func SHA256File(path string) (string, error)  // streamed 4096-byte blocks, hex
func IsImageFile(path string) bool            // R3.1: extension set then mime prefix "image/"
var ImageExtensions = []string{".jpg", ".jpeg", ".png", ".bmp", ".gif", ".tiff", ".webp", ".svg"}
```

### `internal/config`

```go
const FileName, StateDirName
func HomeDir() string; func Dir() string; func DefaultPath() string; func StateDir() string
func CandidatePaths() []string; func ExpandPath(string) string; func MkdirAll(string) error

type Document struct {
    Desktop, Lockscreen, DualWallpapers bool
    DefaultWait int               // 0 = unset
    DefaultURL, DefaultFile, DefaultDirectory string
    ConsoleFontFamily string; ConsoleFontSize int
    WindowWidth, WindowHeight int
    ImageExtensions string; Debug, Autostart bool
    RestartDelay, LogsRefreshInterval int; AutoUpdateLogs bool
    Plugins map[string]map[string]any   // every plugin block, known or not; preserved verbatim
    Extra map[string]any                // unknown top-level keys, preserved verbatim
}
const DefaultWaitGUI = 300; const DefaultWaitService = 900
func Defaults() Document                       // the GUI defaults (font Monospace/10, 800x600, extensions list, wait 300, restart 10, logs 5)
func Load(path string) (Document, error)       // yaml.v3 → Document, runs Migrate, persists if mutated (R2.4); missing file → Defaults(), nil
func LoadDefault() (Document, string, error)   // probes CandidatePaths(); returns the path used ("" if none)
func Save(path string, d Document) error       // sorted keys, 2-space indent, atomic temp+rename, process-wide mutex (DV3); omits AutoUpdateLogs when false; omits zero-value optional keys the way the Python writer did not emit them (only write keys that were set or are non-default)
func Marshal(d Document) ([]byte, error)       // the bytes Save writes
func Parse(b []byte) (Document, error)         // no migration, no I/O
func Migrate(raw map[string]any) (changed bool) // idempotent migration list; currently google_images → duckduckgo_images (R2.4)
func (d Document) PluginEnabled(name string) bool
func (d Document) EnabledPlugins() []string     // sorted
func (d *Document) SetPlugin(name string, block map[string]any)

// Watcher (R2.6)
const DebounceSeconds = 2.0
type Watcher struct{ ... }
func Watch(ctx context.Context, path string) (*Watcher, error)  // fsnotify on parent dir, non-recursive; parent is created if absent
func (w *Watcher) Changed() <-chan struct{}                     // one token per raw event that matched the config path (Create/Write/Rename/Chmod, resolved path equality; Rename also checks the new name)
func (w *Watcher) Close() error
func DrainBurst(ctx context.Context, changed <-chan struct{}, debounce time.Duration) // spec 009 semantics: returns when quiet for `debounce` after the last event, or ctx done
func WaitForNextCycle(ctx context.Context, wait time.Duration, changed <-chan struct{}, debounce time.Duration) (interrupted bool) // 1 s ticks; on event: DrainBurst then return true; returns false when wait elapses or ctx done
```

### `internal/store`

```go
type History struct{ ... }
func OpenHistory(path string) (*History, error)          // creates schema if absent (R5.5)
func OpenDefaultHistory() (*History, error)              // config.StateDir()/history.db, MkdirAll first
func (h *History) Close() error
func (h *History) SeenURL(url string) (bool, error)
func (h *History) SeenImage(path string) (bool, error)   // false,nil if file missing
func (h *History) AddEntry(url, path, source string) (added bool, err error) // false,nil on duplicate url_hash
type HistoryStats struct{ TotalRecords, UniqueImages int; DBSizeBytes int64 }
func (h *History) Stats() (HistoryStats, error)
func (h *History) Clear() error                          // DELETE + VACUUM

type Blacklist struct{ ... }
func OpenBlacklist(path string) (*Blacklist, error)      // path is the .db file (R5.6)
func OpenDefaultBlacklist() (*Blacklist, error)
func (b *Blacklist) Close() error
func (b *Blacklist) Add(hash, plugin, filePath string) error // if filePath exists: compute SHA-256 when hash=="" and thumbnail; INSERT OR REPLACE; timestamp now
func (b *Blacklist) Remove(hash string) error
func (b *Blacklist) IsBlacklisted(hash string) bool      // false on any error
type BlacklistItem struct{ Hash, Source string; Timestamp float64; Thumbnail []byte; Date string }
func (b *Blacklist) Items() ([]BlacklistItem, error)     // timestamp DESC; Date "2006-01-02 15:04" local or "Unknown"
func (b *Blacklist) ProcessFiles(paths []string, plugin string, ev events.Events) (int, error) // add then os.Remove each existing file; logs "Blacklisted and removed: <name>"
var Now = time.Now                                       // injectable clock
```

### `internal/platform`

```go
type Monitor struct{ X, Y, W, H int }
type Runner interface { Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error) } // exec wrapper; ExecRunner default; FakeRunner in tests
type Platform interface {
    Name() string                                   // "linux" | "windows" | "darwin"
    MonitorCount(ctx) int                           // failure → 1
    SetWallpaper(ctx, path string) error
    SetWallpaperMulti(ctx, paths []string) error
    SetLockscreen(ctx, path string) error           // ErrUnsupported on windows/darwin
    LockscreenSupported() bool
}
func New(r Runner) Platform                          // host platform
var ErrUnsupported = errors.New("not supported on this platform")
func JSStringEscape(s string) string                 // DV1: escapes \ and " and control chars for a double-quoted JS literal
// Linux-only helpers (linux build tag):
func KDESingleScript(path string) string; func KDEMultiScript(paths []string) string   // exact golden text modulo escaping
func KwriteconfigArgs(path string) []string
// Service control (all OSes; non-Linux returns the Python message strings)
type ServiceState string // "active" "inactive" "activating" "deactivating" "failed" "unknown"
type Service interface {
    Name() string; IsActive(ctx) ServiceState; StatusDetails(ctx) string
    Start(ctx) error; Stop(ctx) error; Restart(ctx) error
    Install(ctx) error; Uninstall(ctx) error; Logs(ctx, lines int) string
}
func NewService(r Runner) Service
var UnitFile []byte // go:embed clockwork-orange.service (R4.5 text)
// Single instance (R4.8, R4.11, DV10)
type Lock interface{ Release() }
func TryLock(id string) (Lock, bool)                 // id e.g. "clockwork_orange_gui_lock"; false when held elsewhere; fail-open (true) on creation errors with a log line
// Windows extras (windows build tag): SetAppUserModelID(id string); stitch helpers behind an interface with fakes
// macOS extras (darwin): PruneWallpaperCache(ctx, r Runner, maxMB int) error
```

### `internal/plugins`

```go
type FieldType string // "string" "boolean" "integer" "string_list"
type Widget string    // "" "file_path" "directory_path"
type Field struct{ Key string; Type FieldType; Description string; Default any; Required bool; Enum []string; Suggestions []string; Group string; Widget Widget }
type Result struct{ Path, Message string }
type Plugin interface {
    Name() string; Description() string; Schema() []Field
    Run(ctx context.Context, cfg map[string]any, ev events.Events) (Result, error)
}
type Deps struct{ History *store.History; Blacklist *store.Blacklist; HTTP *http.Client; Now func() time.Time }
func Registry(d Deps) []Plugin           // order: local, wallhaven, duckduckgo_images
func Lookup(reg []Plugin, name string) (Plugin, bool)
func Names(reg []Plugin) []string
type Term struct{ Term string; Enabled bool }
func ParseTerms(v any) []Term             // string (comma), []any of map/string; used by both plugins and the GUI
func ParseQueries(v any, fallback string) []string  // Wallhaven _parse_queries semantics
func ShouldRun(dir string, interval string, now time.Time) bool // .last_run logic (R5.3), interval compared lower-cased
func UpdateLastRun(dir string, now time.Time) error
```

### `internal/engine`

```go
var Rand *rand.Rand // math/rand/v2, injectable
func GatherValidSources(paths []string, ev events.Events) []string
func RandomImageFromSources(sources []string, ev events.Events) (string, error)      // R3.2
func PickDistinct(sources []string, n int, ev events.Events) ([]string, error)       // R3.3: n draws, 5 retries each vs already chosen
func PickLockscreenDistinct(sources []string, avoid []string, ev events.Events) (string, error)
func DownloadToTemp(ctx, client *http.Client, url string) (path string, cleanup func(), err error) // R3.5, 30 s
type Setter interface { MonitorCount(ctx) int; SetWallpaper(ctx, string) error; SetWallpaperMulti(ctx, []string) error; SetLockscreen(ctx, string) error } // satisfied by platform.Platform
func SetRandomFromSources(ctx, s Setter, sources []string, ev events.Events) error   // per-monitor de-dup
func SetDualFromSources(ctx, s Setter, sources []string, ev events.Events) error
```

### `internal/core`

Request/Result structs per operation; every function `func Op(ctx context.Context, req OpRequest) (OpResult, error)`.
Operations: `LoadConfig`, `SaveConfig`, `ResolveMode` (merge_config_with_args), `Cycle` (one dynamic multi-plugin cycle: collect enabled plugin sources, set per mode), `RunLoop` (cycling loop with watcher and the daemon lock), `DaemonRunning`, `SetFromFile/Directory/URL`, `CollectSources`, `RunPlugin`, `PluginsList`, `PluginNames`, `AvailablePluginNames`, `ServiceStatus/Start/Stop/Restart/Install/Uninstall/Logs`, `BlacklistList/Remove/Add`, `HistoryStats/Clear/Import`, `SelfTest`, `DebugLockscreen`, `WriteConfig`, `Version`.
Each request embeds `core.Request{ConfigPath string; Events events.Events}`.

### `internal/cli`

cobra tree over `core`. The root command is the v2.9.5 argparse surface
(`--lockscreen --desktop -u -f -d --plugin --plugin-config -w
--debug-lockscreen --write-config --gui --service --self-test`); subcommands
`service`, `blacklist`, `history`, `plugins`, `plugin`, `config`, `version`
each render one core operation. `App.Execute(ctx, args, stdout, stderr) int`
maps errors to exit codes: `UsageError` → 2, `ExitError` → its code
(the GUI child's status, or a self-test that already printed its verdict),
anything else → 1. Log lines go to stderr through a `log/slog` handler that
prints `[LEVEL] msg`; plugin progress renders as the `::PROGRESS::` and
`::IMAGE_SAVED::` marker lines. A bare invocation or `--gui` runs the
`clockwork-orange-gui` binary found beside the CLI, on PATH, or at
`$CLOCKWORK_ORANGE_GUI`.

### `internal/gui`

Fyne front end over `core`, design system copied from nmsbonker (`theme.go`,
`fonts.go`, `cursor_*.go`, `views_table.go`, `views_appearance.go`,
`dialogs.go`). `Run(Options)` opens the window; `SectionNames()`,
`SchemeNames()` and `Actions()` are computable before an app exists.
Sections: Service (Linux) or Activity, one per registered plugin, History,
Blacklist, Settings, Appearance, About. State lives on `*ui`, written only on
the UI thread; every core call runs through `perform`/a loader with the busy
popup up and hops back with `fyne.Do`. Edits write into `ui.doc` and
`scheduleSave` coalesces them into one `core.SaveConfig` 1 s after the last
change. `wallpaperTimer` runs `core.Cycle` on `default_wait` and idles while
`core.DaemonRunning()` (DV10). `reviewModel` is the plugin section's image
review (scan, ←/→/Space, red overlay, fsnotify rescans, `process_blacklist`
on Apply).

#### Design language

Sections are assembled from a fixed vocabulary rather than from raw Fyne
widgets, so that every section states the same kind of thing the same way.
Reach for the component before writing a new arrangement of labels; a shape
that appears in a second section belongs here.

| Component | Where | Use it for |
|-----------|-------|------------|
| `heading(title, blurb)` | `app.go` | The sentence at the top of a section saying what it is |
| `card(title, body…)` | `app.go` | A titled block of facts with a rule under the title |
| `note(text, Status)` | `app.go` | A marked, wrapped caveat inside a section |
| `wrapped(text)` | `app.go` | A paragraph that must reflow rather than run off the edge |
| `statusText` / `marker` | `app.go` | Ranking a value or a row from the active scheme's roles, never a hard-coded colour |
| `detailTable` | `views_table.go` | Anything the CLI would print as a fixed-width table, with an optional thumbnail column |
| `logPane` | `logpane.go` | A stream of lines arriving while the window is open: fixed height, follows the tail unless the reader scrolled up |
| `markdownPane` | `markdownpane.go` | A Markdown document that can outgrow the window |
| `codePanel` | `markdownpane.go` | A block of commands or code inside such a document |

Two rules bind the vocabulary. Colour comes from the scheme through `Status`,
so every component stays legible in all five schemes. And nothing transient
reflows the interface: results and progress float over the content as popups,
`logPane` is a fixed height, and `markdownPane` reserves each block's measured
height whether or not that block is currently rendered.

`markdownPane` exists because the About README scrolled in fits and bursts
(spec 011). Two causes: Fyne's RichText lays out and repaints every segment it
holds on each refresh, and Fyne draws a Markdown code block inside a
horizontal scroll, which takes the wheel from the section and spends it
sideways. The pane splits the document on blank lines, measures each block
once per width, keeps only the blocks within half a viewport of the screen in
the widget tree, and draws code with `codePanel`, which wraps rather than
scrolls. It is not scrollable itself: a section puts it inside its own scroll
and hands that scroll to `follow`, and `ui.detach` releases it when the
section is replaced.

That is the rule the vocabulary adds here: one scroll per section. A widget
that scrolls inside a scrolling section stops the page wherever the pointer
happens to rest, so a component either fills the space it is given
(`markdownPane`, `codePanel`) or is a fixed height with its own scrollbar the
reader can aim at (`logPane`, the Service section's details pane).

## Testing conventions

- Unit tests beside the code; goldens under `tests/golden/` (read via a
  `testdata` helper that finds the repo root by walking up to `go.mod`).
- External processes behind `platform.Runner`; network behind `*http.Client`
  with `httptest.Server`; clock injectable.
- Live tests gated by `CLOCKWORK_LIVE_KDE`, `CLOCKWORK_LIVE_NET`,
  `CLOCKWORK_LIVE_SYSTEMD`.
- Test names are sentences naming the bug they prevent.
