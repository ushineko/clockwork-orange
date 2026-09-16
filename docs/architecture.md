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
Operations: `LoadConfig`, `SaveConfig`, `Cycle` (one dynamic multi-plugin cycle: collect enabled plugin sources, set per mode), `RunDaemon` (cycling loop with watcher), `SetFromFile/Directory/URL`, `RunPlugin`, `ServiceStatus/Start/Stop/Restart/Install/Uninstall/Logs`, `BlacklistList/Remove/Add`, `HistoryStats/Clear/Import`, `PluginsList`, `SelfTest`, `DebugLockscreen`, `WriteConfig`.
Each request embeds `core.Request{ConfigPath string; Events events.Events}`.

## Testing conventions

- Unit tests beside the code; goldens under `tests/golden/` (read via a
  `testdata` helper that finds the repo root by walking up to `go.mod`).
- External processes behind `platform.Runner`; network behind `*http.Client`
  with `httptest.Server`; clock injectable.
- Live tests gated by `CLOCKWORK_LIVE_KDE`, `CLOCKWORK_LIVE_NET`,
  `CLOCKWORK_LIVE_SYSTEMD`.
- Test names are sentences naming the bug they prevent.
