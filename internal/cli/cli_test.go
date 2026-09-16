package cli_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/ushineko/clockwork-orange/internal/cli"
	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/platform"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// fakePlatform records what the CLI asked the desktop to do.
type fakePlatform struct {
	mu   sync.Mutex
	set  []string
	lock []string
}

func (*fakePlatform) Name() string                     { return "fake" }
func (*fakePlatform) MonitorCount(context.Context) int { return 1 }
func (*fakePlatform) LockscreenSupported() bool        { return true }
func (f *fakePlatform) SetWallpaper(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, p)
	return nil
}

func (f *fakePlatform) SetWallpaperMulti(_ context.Context, p []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, p...)
	return nil
}

func (f *fakePlatform) SetLockscreen(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lock = append(f.lock, p)
	return nil
}

func (f *fakePlatform) sets() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.set)
}

func (f *fakePlatform) locks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.lock)
}

// harness is a CLI over a fake desktop, a fake systemctl and temp databases,
// with HOME redirected so nothing touches the developer's real config.
type harness struct {
	t      *testing.T
	home   string
	fp     *fakePlatform
	runner *platform.FakeRunner
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLOCKWORK_LOCK_DIR", home)
	t.Setenv("CLOCKWORK_ORANGE_GUI", "")
	return &harness{t: t, home: home, fp: &fakePlatform{}, runner: &platform.FakeRunner{}}
}

// deps opens the harness databases; Execute closes them, so each run gets
// its own handles over the same files.
func (h *harness) deps() *core.Deps {
	h.t.Helper()
	hist, err := store.OpenHistory(filepath.Join(h.home, "history.db"))
	require.NoError(h.t, err)
	bl, err := store.OpenBlacklist(filepath.Join(h.home, "blacklist.db"))
	require.NoError(h.t, err)
	return &core.Deps{Platform: h.fp, Service: platform.NewService(h.runner), History: hist, Blacklist: bl}
}

type result struct {
	code           int
	stdout, stderr string
}

func (h *harness) run(args ...string) result {
	return h.runCtx(context.Background(), args...)
}

func (h *harness) runCtx(ctx context.Context, args ...string) result {
	var out, errb bytes.Buffer
	code := cli.NewApp(cli.Options{Deps: h.deps()}).Execute(ctx, args, &out, &errb)
	return result{code: code, stdout: out.String(), stderr: errb.String()}
}

func (h *harness) writeConfig(doc config.Document) string {
	h.t.Helper()
	path := config.DefaultPath()
	require.NoError(h.t, config.Save(path, doc))
	return path
}

func yamlMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, yaml.Unmarshal(b, &m))
	return m
}

func imageDir(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range n {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "img"+string(rune('a'+i))+".jpg"), []byte("x"), 0o600))
	}
	return dir
}

func localPluginConfig(dir string) config.Document {
	doc := config.Document{Plugins: map[string]map[string]any{}}
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": dir}
	return doc
}

// --- R6.1: argparse-compatible validation, exit 2 with the same messages ---

// The _validate_args cases and argparse's own parse errors all exit 2 with
// the Python message text, so scripts that matched on them keep working.
func TestValidationErrorsExitTwoWithThePythonMessages(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 2)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no source and no enabled plugins", []string{"--debug-lockscreen"},
			"Operation requires either --file, --directory, --plugin, or enabled plugins in config"},
		{"negative wait", []string{"-d", dir, "-w", "-3"}, "--wait must be a positive integer"},
		{"zero wait with nothing to fall back on", []string{"-d", dir, "-w", "0"}, "--wait must be a positive integer"},
		{"url with lockscreen", []string{"--lockscreen", "-u", "http://x/", "-d", dir},
			"argument -d/--directory: not allowed with argument -u/--url"},
		{"two sources", []string{"-f", "a.jpg", "-u", "http://x/"}, "argument -f/--file: not allowed with argument -u/--url"},
		{"unknown plugin", []string{"--plugin", "nope"},
			"argument --plugin: invalid choice: 'nope' (choose from 'local', 'wallhaven', 'duckduckgo_images')"},
		{"--run-plugin was dropped (DV8)", []string{"--run-plugin", "local"}, "unknown flag: --run-plugin"},
		{"unknown positional", []string{"frob"}, `unknown command "frob"`},
		{"non-integer wait", []string{"-d", dir, "-w", "soon"}, `invalid argument "soon" for "-w, --wait" flag`},
		{"bad log level", []string{"-d", dir, "--log-level", "loud"}, "--log-level must be debug, info, warn or error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := h.run(tc.args...)
			require.Equal(t, cli.ExitUsage, res.code, res.stderr)
			require.Contains(t, res.stderr, tc.want)
			require.Zero(t, h.fp.sets(), "a usage error must not touch the desktop")
		})
	}
}

// --url is rejected with --lockscreen and in dual mode, including when the
// lock-screen flag comes from the config rather than the command line.
func TestURLIsRejectedForTheLockScreenHoweverTheModeWasChosen(t *testing.T) {
	h := newHarness(t)
	doc := localPluginConfig(imageDir(t, 1))
	doc.Lockscreen = true
	h.writeConfig(doc)
	for _, args := range [][]string{
		{"-u", "http://x/", "--lockscreen"},
		{"-u", "http://x/", "--desktop", "--lockscreen"},
		{"-u", "http://x/"}, // lockscreen: true in the file
	} {
		res := h.run(args...)
		require.Equal(t, cli.ExitUsage, res.code, res.stderr)
		require.Contains(t, res.stderr, "--url cannot be used with --lockscreen (lock screen requires local files)")
	}
}

// --- R6.3: mode dispatch ---

func TestFileModeSetsTheDesktopOrTheLockScreenAndRefusesDual(t *testing.T) {
	h := newHarness(t)
	img := filepath.Join(imageDir(t, 1), "imga.jpg")

	res := h.run("-f", img)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 1, h.fp.sets())
	require.Contains(t, res.stderr, "[DEBUG] Wallpaper set successfully")

	res = h.run("--lockscreen", "-f", img)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 1, h.fp.locks())
	require.Contains(t, res.stderr, "[DEBUG] Lock screen set successfully")

	res = h.run("--desktop", "--lockscreen", "-f", img)
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "dual wallpaper mode with single file not supported")
	require.Contains(t, res.stderr, "failed to set both desktop and lock screen")
	require.Equal(t, 1, h.fp.sets(), "the refused dual set must not change anything")
}

// Without a config file there is no default_wait, so -d sets exactly once.
// (With the GUI's default_wait: 300 in the file the same command cycles, as
// it did in 2.9.x.)
func TestDirectoryModeSetsOnceWhenNoWaitApplies(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 3)
	res := h.run("-d", dir)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 1, h.fp.sets())
	require.NotContains(t, res.stderr, "continuous mode")

	res = h.run("--desktop", "--lockscreen", "-d", dir)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 2, h.fp.sets())
	require.Equal(t, 1, h.fp.locks())
}

// default_wait in the config turns a plain -d into cycling (merge_config_with_args).
func TestConfigDefaultWaitMakesDirectoryModeCycleUntilInterrupted(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 3)
	h.writeConfig(config.Document{DefaultWait: 1})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan result, 1)
	go func() { done <- h.runCtx(ctx, "-d", dir) }()
	require.Eventually(t, func() bool { return h.fp.sets() >= 2 }, 10*time.Second, 50*time.Millisecond)
	cancel()
	res := <-done
	require.Equal(t, cli.ExitOK, res.code, "an interrupted cycle is a clean exit")
	require.Contains(t, res.stderr, "Set default wait interval from config: 1")
	require.Contains(t, res.stderr, "continuous mode with 1 second intervals")
}

func TestURLModeDownloadsThenSets(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("not really a jpeg, the fake desktop does not decode"))
	}))
	defer srv.Close()
	h.writeConfig(localPluginConfig(imageDir(t, 1))) // -u alone needs enabled plugins to pass _validate_args
	res := h.run("-u", srv.URL+"/img.jpg")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 1, h.fp.sets())
	require.Contains(t, res.stderr, "URL mode: "+srv.URL)
}

// --- R6.4: --plugin ---

func TestPluginFlagTreatsADirectoryResultAsDirectoryAndAFileResultAsFile(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 2)

	res := h.run("--plugin", "local", "--plugin-config", `{"path": "`+dir+`"}`)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stderr, "Plugin resolved to directory")
	require.Equal(t, 1, h.fp.sets())

	res = h.run("--lockscreen", "--plugin", "local", "--plugin-config", `{"path": "`+filepath.Join(dir, "imga.jpg")+`"}`)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stderr, "Plugin resolved to file")
	require.Equal(t, 1, h.fp.locks())
}

func TestPluginFailuresExitOne(t *testing.T) {
	h := newHarness(t)
	res := h.run("--plugin", "local", "--plugin-config", `{not json`)
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "invalid JSON in --plugin-config")

	res = h.run("--plugin", "local")
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "plugin execution failed: Missing 'path' in configuration")

	res = h.run("--plugin", "local", "--plugin-config", `{"path": "/nonexistent/dir"}`)
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "Path not found")
	require.Zero(t, h.fp.sets())
}

// --- R3.6 / R6.3: dynamic multi-plugin mode ---

func TestEnabledPluginsSupplyTheSourcesWhenNoFlagNamesOne(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 3)
	h.writeConfig(localPluginConfig(dir))

	res := h.run("--desktop")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 1, h.fp.sets())
	require.Contains(t, res.stderr, "[DEBUG] Desktop set successfully")

	res = h.run("--lockscreen")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 1, h.fp.locks())

	// Default mode needs some operation flag or the bare invocation starts
	// the GUI; --plugin-config without --plugin is inert, as in the Python.
	res = h.run("--plugin-config", "{}")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, 2, h.fp.sets())
}

// A plugin that is enabled but fails leaves no sources: the Python printed
// "No source specified and no plugins enabled" in default mode and "No
// enabled plugins found for X mode" otherwise, and exited 1 either way.
func TestNoUsableSourceExitsOne(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(localPluginConfig("/nonexistent/wallpapers"))

	res := h.run("--plugin-config", "{}")
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "no source specified and no plugins enabled; use --plugin, --file, --directory, or --url")

	res = h.run("--desktop")
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "no enabled plugins found for Desktop mode")
	require.Contains(t, res.stderr, "failed to set desktop")
}

// --- R6.8: --service ---

// --service runs the dynamic cycle with a 900 s default, reloads the config
// every cycle, and a settled config edit interrupts the wait (spec 009 carried
// into the port).
func TestServiceModeDefaultsTo900SecondsAndReactsToAConfigEdit(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 3)
	doc := localPluginConfig(dir)
	doc.Desktop = true
	path := h.writeConfig(doc)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan result, 1)
	go func() { done <- h.runCtx(ctx, "--service") }()
	require.Eventually(t, func() bool { return h.fp.sets() == 1 }, 10*time.Second, 50*time.Millisecond, "first cycle")

	time.Sleep(300 * time.Millisecond) // let the watcher settle
	doc.Plugins["local"]["path"] = imageDir(t, 2)
	require.NoError(t, config.Save(path, doc))
	require.Eventually(t, func() bool { return h.fp.sets() == 2 }, 10*time.Second, 50*time.Millisecond,
		"a settled config change must interrupt the 900 s wait")
	cancel()
	res := <-done
	require.Equal(t, cli.ExitOK, res.code)
	require.Contains(t, res.stderr, "Service mode: defaulting to 900s wait interval")
	require.Contains(t, res.stderr, "with 15m0s interval")
	require.Contains(t, res.stderr, "Config change detected, interrupting wait cycle")
}

func TestASecondDaemonIsRefused(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 2)
	doc := localPluginConfig(dir)
	h.writeConfig(doc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan result, 1)
	go func() { done <- h.runCtx(ctx, "--service") }()
	require.Eventually(t, func() bool { return h.fp.sets() == 1 }, 10*time.Second, 50*time.Millisecond)

	res := h.run("--service")
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "another clockwork-orange daemon is already running")
	cancel()
	require.Equal(t, cli.ExitOK, (<-done).code)
}

// --- R2.7: --write-config ---

func TestWriteConfigPersistsTheModeFlagsWaitAndSource(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 1)
	res := h.run("--desktop", "--lockscreen", "-d", dir, "-w", "300", "--write-config")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stderr, "Configuration file written successfully")
	require.Zero(t, h.fp.sets(), "--write-config exits without setting anything")

	got, err := config.Load(config.DefaultPath())
	require.NoError(t, err)
	require.True(t, got.DualWallpapers)
	require.True(t, got.Desktop)
	require.True(t, got.Lockscreen)
	require.Equal(t, 300, got.DefaultWait)
	require.Equal(t, dir, got.DefaultDirectory)
}

// --- R6.6: --self-test ---

func TestSelfTestExitsZeroWhenEveryProbePassesAndOneWhenOneFails(t *testing.T) {
	h := newHarness(t)
	res := h.run("--self-test", "--offline")
	require.Contains(t, res.stdout, "[OK]   sqlite")
	_, noQdbus := exec.LookPath("qdbus6")
	_, noKwrite := exec.LookPath("kwriteconfig6")
	if runtime.GOOS == "linux" && (noQdbus != nil || noKwrite != nil) {
		// A CI runner without Plasma: the KDE tool probes fail by design,
		// which is the second half of this test, not a bug in the first.
		require.Equal(t, cli.ExitFailure, res.code, res.stdout)
		require.Contains(t, res.stdout, "All passed: false")
		return
	}
	require.Equal(t, cli.ExitOK, res.code, res.stdout)
	require.Contains(t, res.stdout, "All passed: true")

	if runtime.GOOS != "linux" {
		t.Skip("the fault-injected probe is the KDE tool lookup, Linux only")
	}
	t.Setenv("PATH", t.TempDir()) // qdbus6/kwriteconfig6/systemctl vanish
	res = h.run("--self-test", "--offline")
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stdout, "[FAIL] qdbus6")
	require.Contains(t, res.stdout, "All passed: false")
	require.Empty(t, res.stderr, "the verdict is on stdout; nothing else to say")
}

// --- R6.2: the GUI hand-off ---

func TestBareInvocationAndGUIFlagRunTheGUIBinaryAndReturnItsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the stand-in GUI")
	}
	h := newHarness(t)
	script := filepath.Join(t.TempDir(), "clockwork-orange-gui")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho gui-ran\nexit 7\n"), 0o700))
	t.Setenv("CLOCKWORK_ORANGE_GUI", script)

	res := h.run()
	require.Equal(t, 7, res.code)
	require.Contains(t, res.stdout, "gui-ran")

	res = h.run("--gui")
	require.Equal(t, 7, res.code)

	res = h.run("--log-level", "info")
	require.Equal(t, 7, res.code, "the shared flags alone do not name an operation")
}

func TestMissingGUIBinaryExitsOneWithGuidance(t *testing.T) {
	h := newHarness(t)
	t.Setenv("PATH", t.TempDir())
	res := h.run()
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, "the graphical interface was not found")
	require.Contains(t, res.stderr, "clockwork-orange-gui")
	require.Contains(t, res.stderr, "--file, --directory, --url or --plugin")
}

// --- R6.5: subcommands ---

func TestBlacklistAddListRemoveRoundTrip(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 2)
	a, b := filepath.Join(dir, "imga.jpg"), filepath.Join(dir, "imgb.jpg")
	require.NoError(t, os.WriteFile(b, []byte("different"), 0o600))

	res := h.run("blacklist", "list")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "The blacklist is empty.")

	res = h.run("blacklist", "add", "--plugin", "local", a, b)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "Blacklisted and removed 2 file(s)")
	require.NoFileExists(t, a)
	require.NoFileExists(t, b)

	res = h.run("blacklist", "list")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	lines := strings.Split(strings.TrimSpace(res.stdout), "\n")
	require.Len(t, lines, 3, "header plus two rows:\n%s", res.stdout)
	require.Contains(t, lines[0], "HASH")
	require.Contains(t, lines[1], "local")
	hash := strings.Fields(lines[1])[0]
	require.Len(t, hash, 64, "SHA-256 hex")

	res = h.run("blacklist", "remove", hash)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	res = h.run("blacklist", "list")
	require.Len(t, strings.Split(strings.TrimSpace(res.stdout), "\n"), 2)

	res = h.run("blacklist", "remove")
	require.Equal(t, cli.ExitUsage, res.code, "a missing argument is a usage error")
}

func TestHistoryImportStatsAndClear(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 3)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o600))

	res := h.run("history", "import", dir)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "3 imported, 0 skipped")

	res = h.run("history", "import", dir)
	require.Contains(t, res.stdout, "0 imported, 3 skipped")

	res = h.run("history", "stats")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "Total downloads tracked: 3")

	res = h.run("history", "clear")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	res = h.run("history", "stats")
	require.Contains(t, res.stdout, "Total downloads tracked: 0")

	res = h.run("history", "import", "one", "two")
	require.Equal(t, cli.ExitUsage, res.code)
}

func TestPluginsListShowsEnablement(t *testing.T) {
	h := newHarness(t)
	h.writeConfig(localPluginConfig(imageDir(t, 1)))
	res := h.run("plugins", "list")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	lines := strings.Split(strings.TrimSpace(res.stdout), "\n")
	require.Len(t, lines, 4)
	require.Regexp(t, `^local\s+yes\s+`, lines[1])
	require.Regexp(t, `^wallhaven\s+no\s+`, lines[2])
	require.Regexp(t, `^duckduckgo_images\s+no\s+`, lines[3])
}

func TestPluginRunPrintsThePathAndFailsOnUnknownPlugin(t *testing.T) {
	h := newHarness(t)
	dir := imageDir(t, 1)
	res := h.run("plugin", "run", "local", "--plugin-config", `{"path": "`+dir+`"}`)
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Equal(t, dir, strings.TrimSpace(res.stdout))
	require.Zero(t, h.fp.sets(), "plugin run downloads; it does not set a wallpaper")

	res = h.run("plugin", "run", "nope")
	require.Equal(t, cli.ExitFailure, res.code)
	require.Contains(t, res.stderr, `unknown plugin "nope"`)

	res = h.run("plugin", "run", "local", "--plugin-config", "nope")
	require.Equal(t, cli.ExitUsage, res.code)
}

// `config migrate` on the 2.9.5 golden pre-migration file yields the golden
// post-migration file: the stable_diffusion block survives (D6).
func TestConfigMigrateRewritesTheGoldenPreMigrationFile(t *testing.T) {
	h := newHarness(t)
	pre, err := os.ReadFile(filepath.Join("..", "..", "tests", "golden", "config", "pre_migration.yml"))
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join("..", "..", "tests", "golden", "config", "post_migration.yml"))
	require.NoError(t, err)
	// The pinned download_dir embeds the $HOME the Python capture ran under.
	want = bytes.ReplaceAll(want, []byte("/home/nverenin"), []byte(h.home))
	path := config.DefaultPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, pre, 0o600))

	res := h.run("config", "migrate")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "Configuration at "+path+" is current")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	// Semantic comparison: PyYAML wrote indentless block sequences, yaml.v3
	// indents them. The config package pins the Go writer's own bytes.
	require.Equal(t, yamlMap(t, want), yamlMap(t, got))

	res = h.run("config", "show")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "# "+path+"\n")
	require.Contains(t, res.stdout, "duckduckgo_images:")
	require.Contains(t, res.stdout, "stable_diffusion:")
}

func TestConfigCommandsOnAMissingFileReportTheDefaults(t *testing.T) {
	h := newHarness(t)
	res := h.run("config", "show")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "does not exist; these are the defaults")
	require.Contains(t, res.stdout, "default_wait: 300")
	res = h.run("config", "migrate")
	require.Contains(t, res.stdout, "nothing to migrate")
}

func TestServiceCommandsDriveSystemctl(t *testing.T) {
	h := newHarness(t)
	h.runner.Handler = func(_ context.Context, argv []string) (string, string, error) {
		if len(argv) > 2 && argv[2] == "is-active" {
			return "active\n", "", nil
		}
		return "● clockwork-orange.service - fake\n", "", nil
	}
	res := h.run("service", "status")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "Service: clockwork-orange.service")
	require.Contains(t, res.stdout, "State:   active")
	require.Contains(t, res.stdout, "fake")

	for _, verb := range []string{"start", "stop", "restart"} {
		h.runner.Reset()
		res = h.run("service", verb)
		require.Equal(t, cli.ExitOK, res.code, res.stderr)
		require.Contains(t, res.stdout, "Service "+past(verb))
		var seen bool
		for _, c := range h.runner.Calls() {
			if c[0] == "systemctl" && strings.Contains(strings.Join(c, " "), verb+" clockwork-orange.service") {
				seen = true
			}
		}
		require.True(t, seen, "%s must reach systemctl: %v", verb, h.runner.Calls())
	}

	res = h.run("service", "logs", "-n", "5")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "fake")

	res = h.run("service", "frob")
	require.Equal(t, cli.ExitUsage, res.code)
}

func past(verb string) string {
	if verb == "stop" {
		return "stopped"
	}
	return verb + "ed"
}

func TestVersionCommandAndFlag(t *testing.T) {
	h := newHarness(t)
	res := h.run("version")
	require.Equal(t, cli.ExitOK, res.code)
	// "Unknown" under `go test`: no ldflags and no .tag beside the test binary.
	require.Regexp(t, `^clockwork-orange \S+ \(\S+\)\n$`, res.stdout)
	res = h.run("--version")
	require.Equal(t, cli.ExitOK, res.code)
	require.Contains(t, res.stdout, "clockwork-orange version ")
}
