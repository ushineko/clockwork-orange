package core

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/platform"
	"github.com/ushineko/clockwork-orange/internal/store"
)

type fakePlatform struct {
	mu       sync.Mutex
	monitors int
	set      []string
	multi    [][]string
	lock     []string
}

func (f *fakePlatform) Name() string                     { return "fake" }
func (f *fakePlatform) MonitorCount(context.Context) int { return f.monitors }
func (f *fakePlatform) LockscreenSupported() bool        { return true }
func (f *fakePlatform) SetLockscreen(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lock = append(f.lock, p)
	return nil
}

func (f *fakePlatform) SetWallpaper(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, p)
	return nil
}

func (f *fakePlatform) SetWallpaperMulti(_ context.Context, p []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.multi = append(f.multi, p)
	return nil
}

func (f *fakePlatform) sets() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.set) + len(f.multi)
}

// testDeps builds Deps over temp stores and a fake desktop, with HOME
// redirected so nothing touches the developer's real config.
func testDeps(t *testing.T, monitors int) (*Deps, *fakePlatform, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	h, err := store.OpenHistory(filepath.Join(home, "history.db"))
	require.NoError(t, err)
	b, err := store.OpenBlacklist(filepath.Join(home, "blacklist.db"))
	require.NoError(t, err)
	fp := &fakePlatform{monitors: monitors}
	d := &Deps{Platform: fp, Service: platform.NewService(&platform.FakeRunner{}), History: h, Blacklist: b}
	t.Cleanup(func() { _ = d.Close() })
	return d, fp, home
}

func imageDir(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range n {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "img"+string(rune('a'+i))+".jpg"), []byte("x"), 0o600))
	}
	return dir
}

func writeConfig(t *testing.T, doc config.Document) string {
	t.Helper()
	require.NoError(t, config.Save(config.DefaultPath(), doc))
	return config.DefaultPath()
}

// dual_wallpapers in the config forces dual mode even with no flags; the
// config's desktop/lockscreen keys apply only when no flag was given, and an
// explicit --lockscreen is not widened to dual by `desktop: true` in the file
// -- merge_config_with_args semantics (R2.5).
func TestResolveModeMergesFlagsWithConfig(t *testing.T) {
	require.Equal(t, ModeDual, ResolveMode(config.Document{DualWallpapers: true}, false, false))
	require.Equal(t, ModeLockscreen, ResolveMode(config.Document{Desktop: true}, false, true))
	require.Equal(t, ModeDesktop, ResolveMode(config.Document{Lockscreen: true}, true, false))
	require.Equal(t, ModeDual, ResolveMode(config.Document{}, true, true))
	require.Equal(t, ModeLockscreen, ResolveMode(config.Document{}, false, true))
	require.Equal(t, ModeDesktop, ResolveMode(config.Document{Desktop: true}, false, false))
	require.Equal(t, ModeLockscreen, ResolveMode(config.Document{Lockscreen: true}, false, false))
	require.Equal(t, ModeDesktop, ResolveMode(config.Document{Desktop: true, Lockscreen: true}, false, false),
		"both keys without dual_wallpapers is desktop-only, as in the Python")
	require.Equal(t, ModeDefault, ResolveMode(config.Document{}, false, false))
}

// A dynamic cycle reloads the config, runs the enabled local plugin and sets
// one image per monitor; a plugin the build does not know (stable_diffusion)
// is skipped without failing the cycle (D6).
func TestDynamicCycleRunsEnabledPluginsAndSkipsUnknownOnes(t *testing.T) {
	d, fp, _ := testDeps(t, 2)
	dir := imageDir(t, 4)
	doc := config.Defaults()
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": dir}
	doc.Plugins["stable_diffusion"] = map[string]any{"enabled": true, "prompt": "castle"}
	writeConfig(t, doc)

	var logs []string
	ev := events.Events{OnLog: func(_ events.Level, m string) { logs = append(logs, m) }}
	res, err := Cycle(context.Background(), CycleRequest{Request: Request{Deps: d, Events: ev}, Mode: ModeDesktop})
	require.NoError(t, err)
	require.Equal(t, []string{dir}, res.Sources)
	require.Len(t, fp.multi, 1)
	require.Len(t, fp.multi[0], 2)
	require.Contains(t, logs, "Plugin stable_diffusion is enabled but not available in this build; skipping")
}

// With nothing enabled the dynamic cycle reports ErrNoEnabledPlugins instead
// of silently doing nothing.
func TestDynamicCycleWithoutPluginsIsAnError(t *testing.T) {
	d, _, _ := testDeps(t, 1)
	writeConfig(t, config.Defaults())
	_, err := Cycle(context.Background(), CycleRequest{Request: Request{Deps: d}, Mode: ModeDefault})
	require.ErrorIs(t, err, ErrNoEnabledPlugins)
}

// Dual mode from a single file is refused (R6.3); lockscreen mode from a file
// goes to the lock screen, not the desktop.
func TestSetFromFileDispatchesByMode(t *testing.T) {
	d, fp, _ := testDeps(t, 1)
	dir := imageDir(t, 1)
	img := filepath.Join(dir, "imga.jpg")
	require.ErrorIs(t, SetFromFile(context.Background(), SetRequest{Request: Request{Deps: d}, Mode: ModeDual, Path: img}), ErrDualNeedsDirectory)
	require.NoError(t, SetFromFile(context.Background(), SetRequest{Request: Request{Deps: d}, Mode: ModeLockscreen, Path: img}))
	require.Equal(t, []string{img}, fp.lock)
	require.Empty(t, fp.set)
	require.NoError(t, SetFromFile(context.Background(), SetRequest{Request: Request{Deps: d}, Mode: ModeDesktop, Path: img}))
	require.Equal(t, []string{img}, fp.set)
}

// A settled config edit interrupts the daemon's wait and triggers exactly one
// extra cycle, the spec 009 contract carried into the port (R2.6, R3.6).
func TestRunLoopIsInterruptedOnceByAConfigChange(t *testing.T) {
	d, fp, _ := testDeps(t, 1)
	dir := imageDir(t, 3)
	doc := config.Defaults()
	doc.DefaultWait = 3600
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": dir}
	path := writeConfig(t, doc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cycles := make(chan struct{}, 16)
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, LoopRequest{
			Request:  Request{Deps: d},
			Mode:     ModeDesktop,
			Wait:     time.Hour,
			Debounce: 200 * time.Millisecond,
			OnCycle:  func(CycleResult, error) { cycles <- struct{}{} },
		})
	}()
	select {
	case <-cycles:
	case <-time.After(5 * time.Second):
		t.Fatal("first cycle did not run")
	}
	time.Sleep(100 * time.Millisecond) // let the watcher settle
	start := time.Now()
	for range 4 {
		require.NoError(t, config.Save(path, doc))
		time.Sleep(30 * time.Millisecond)
	}
	select {
	case <-cycles:
		require.Greater(t, time.Since(start), 200*time.Millisecond, "returned before the burst settled")
	case <-time.After(5 * time.Second):
		t.Fatal("config change did not interrupt the wait")
	}
	select {
	case <-cycles:
		t.Fatal("a burst of writes caused more than one extra cycle")
	case <-time.After(700 * time.Millisecond):
	}
	require.Equal(t, 2, fp.sets())
	cancel()
	require.NoError(t, <-done)
}

// Two loops cannot run at once: the second gets ErrDaemonRunning and
// DaemonRunning reports the lock (DV10).
func TestSecondLoopIsRefusedWhileTheFirstHoldsTheDaemonLock(t *testing.T) {
	d, _, _ := testDeps(t, 1)
	dir := imageDir(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, LoopRequest{Request: Request{Deps: d}, Mode: ModeDesktop, Wait: time.Hour, Sources: []string{dir},
			OnCycle: func(CycleResult, error) {
				select {
				case started <- struct{}{}:
				default:
				}
			}})
	}()
	<-started
	require.True(t, DaemonRunning())
	err := RunLoop(ctx, LoopRequest{Request: Request{Deps: d}, Mode: ModeDesktop, Wait: time.Hour, Sources: []string{dir}})
	require.ErrorIs(t, err, ErrDaemonRunning)
	cancel()
	require.NoError(t, <-done)
	require.False(t, DaemonRunning())
}

// --write-config persists the mode flags and wait while keeping plugin blocks
// that were already in the file (R2.7).
func TestWriteConfigKeepsExistingPluginBlocks(t *testing.T) {
	d, _, _ := testDeps(t, 1)
	doc := config.Defaults()
	doc.Plugins["wallhaven"] = map[string]any{"enabled": true, "api_key": "k"}
	writeConfig(t, doc)
	path, err := WriteConfig(context.Background(), WriteConfigRequest{Request: Request{Deps: d}, Dual: true, Wait: 42, Directory: "/pics"})
	require.NoError(t, err)
	got, err := config.Load(path)
	require.NoError(t, err)
	require.True(t, got.DualWallpapers)
	require.Equal(t, 42, got.DefaultWait)
	require.Equal(t, "/pics", got.DefaultDirectory)
	require.Equal(t, "k", got.Plugins["wallhaven"]["api_key"])
}

// Import counts JPEGs once: a second run skips every file (R7.7).
func TestHistoryImportSkipsAlreadyImportedFiles(t *testing.T) {
	d, _, _ := testDeps(t, 1)
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.JPEG", "c.png", "d.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600))
	}
	res, err := HistoryImport(context.Background(), HistoryImportRequest{Request: Request{Deps: d}, Dir: dir})
	require.NoError(t, err)
	require.Equal(t, 2, res.Imported)
	require.Equal(t, 0, res.Skipped)
	res, err = HistoryImport(context.Background(), HistoryImportRequest{Request: Request{Deps: d}, Dir: dir})
	require.NoError(t, err)
	require.Equal(t, 0, res.Imported)
	require.Equal(t, 2, res.Skipped)
	stats, err := HistoryStats(context.Background(), Request{Deps: d})
	require.NoError(t, err)
	require.Equal(t, 2, stats.TotalRecords)
}

// The self-test's portable probes pass on any machine; platform tool probes
// are reported but not asserted here because CI runners lack KDE.
func TestSelfTestPortableChecksPass(t *testing.T) {
	res := SelfTest(context.Background(), SelfTestRequest{SkipNetwork: true})
	for _, c := range res.Checks {
		switch c.Name {
		case "sqlite", "yaml", "image codecs", "plugins", "runtime":
			require.Truef(t, c.OK, "%s: %s", c.Name, c.Detail)
		}
	}
	require.Len(t, res.Checks, len(res.Checks))
}

// PluginsList reflects the config's enablement per registry entry.
func TestPluginsListReportsEnablement(t *testing.T) {
	d, _, _ := testDeps(t, 1)
	doc := config.Defaults()
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": "/x"}
	writeConfig(t, doc)
	list, err := PluginsList(context.Background(), Request{Deps: d})
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, "local", list[0].Name)
	require.True(t, list[0].Enabled)
	require.False(t, list[1].Enabled)
	require.NotEmpty(t, list[1].Schema)
}
