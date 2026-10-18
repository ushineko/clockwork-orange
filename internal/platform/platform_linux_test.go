//go:build linux

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// kdeGolden is tests/golden/platform/kde_commands.json.
type kdeGolden struct {
	Paths      []string `json:"paths"`
	Single     []string `json:"single"`
	Multi      []string `json:"multi"`
	Lockscreen []string `json:"lockscreen"`
	Reload     []string `json:"reload"`
}

// loadKDEGolden reads the golden and rewrites the recorded absolute paths to
// the caller's files, as tests/golden/README.md prescribes.
func loadKDEGolden(t *testing.T, mine []string) kdeGolden {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "tests", "golden", "platform", "kde_commands.json"))
	require.NoError(t, err)
	var g kdeGolden
	require.NoError(t, json.Unmarshal(raw, &g))
	require.Len(t, mine, len(g.Paths))
	subst := func(argv []string) []string {
		out := make([]string, len(argv))
		for i, a := range argv {
			for j, rec := range g.Paths {
				a = strings.ReplaceAll(a, rec, mine[j])
			}
			out[i] = a
		}
		return out
	}
	g.Single, g.Multi, g.Lockscreen, g.Reload = subst(g.Single), subst(g.Multi), subst(g.Lockscreen), subst(g.Reload)
	return g
}

func goldenFiles(t *testing.T) []string {
	t.Helper()
	dir := resolvedTempDir(t)
	return []string{
		touch(t, dir, "landscape_1920x1080.png"),
		touch(t, dir, "portrait_1080x1920.png"),
		touch(t, dir, "square_600x600.png"),
	}
}

// fakePlasma is plasmashell with a fixed set of screens: it records every
// SetImage and answers Image from what was set.
type fakePlasma struct {
	mu       sync.Mutex
	screens  int
	images   map[int]string
	sets     []string // "screen=path"
	reloads  int
	err      error
	changes  chan int
	reloadEr error
}

func newFakePlasma(screens int) *fakePlasma {
	return &fakePlasma{screens: screens, images: map[int]string{}, changes: make(chan int, 4)}
}

func (f *fakePlasma) SetImage(_ context.Context, screen int, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sets = append(f.sets, fmt.Sprintf("%d=%s", screen, path))
	if screen < f.screens {
		f.images[screen] = path
	}
	return nil
}

func (f *fakePlasma) Image(_ context.Context, screen int) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", false, f.err
	}
	if screen >= f.screens {
		return "", false, nil
	}
	return f.images[screen], true, nil
}

func (f *fakePlasma) Changes(context.Context) (<-chan int, error) { return f.changes, nil }

func (f *fakePlasma) ReloadScreenSaver(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reloads++
	return f.reloadEr
}

func (f *fakePlasma) setCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sets...)
}

func onPlasma(r Runner, pl Plasma) (*LinuxPlatform, func() []string) {
	ev, lines := captureEvents()
	return &LinuxPlatform{Runner: r, Events: ev, Plasma: pl}, lines
}

// The kwriteconfig6 argv is still the 2.9.5 one, byte for byte.
func TestKwriteconfigArgvIsThePythonOne(t *testing.T) {
	mine := goldenFiles(t)
	g := loadKDEGolden(t, mine)
	require.Equal(t, g.Lockscreen, KwriteconfigArgs(mine[2]))
}

// R1: one image goes to every screen, one setWallpaper per screen; no
// process is started.
func TestSetWallpaperSetsEveryScreenOverDBus(t *testing.T) {
	mine := goldenFiles(t)
	r := &FakeRunner{}
	pl := newFakePlasma(2)
	p, _ := onPlasma(r, pl)
	require.NoError(t, p.SetWallpaper(context.Background(), mine[0]))
	require.Equal(t, []string{"0=" + mine[0], "1=" + mine[0]}, pl.setCalls())
	require.Empty(t, r.Calls(), "no qdbus6")
}

// R1: screen i gets paths[i % len]; missing files are skipped first.
func TestSetWallpaperMultiAssignsImagesByScreen(t *testing.T) {
	mine := goldenFiles(t)
	pl := newFakePlasma(3)
	p, lines := onPlasma(&FakeRunner{}, pl)
	ctx := context.Background()
	require.NoError(t, p.SetWallpaperMulti(ctx, []string{mine[0], "/nonexistent/a.png", mine[1]}))
	require.Equal(t, []string{"0=" + mine[0], "1=" + mine[1], "2=" + mine[0]}, pl.setCalls())
	require.Contains(t, strings.Join(lines(), "\n"), "[ERROR] File does not exist: /nonexistent/a.png")

	require.EqualError(t, p.SetWallpaperMulti(ctx, []string{"/nonexistent/a.png"}), "no valid image paths found")
	require.EqualError(t, p.SetWallpaperMulti(ctx, nil), "no image paths provided")
}

// A path with quotes and backslashes goes to Plasma as it is: there is no
// script left for it to break out of (DV18).
func TestAPathIsPassedVerbatim(t *testing.T) {
	dir := resolvedTempDir(t)
	weird := touch(t, dir, `we"ird\name.png`)
	pl := newFakePlasma(1)
	p, _ := onPlasma(&FakeRunner{}, pl)
	require.NoError(t, p.SetWallpaper(context.Background(), weird))
	require.Equal(t, []string{"0=" + weird}, pl.setCalls())
}

func TestSetWallpaperRefusesMissingFileWithoutCallingPlasma(t *testing.T) {
	pl := newFakePlasma(1)
	p, lines := onPlasma(&FakeRunner{}, pl)
	err := p.SetWallpaper(context.Background(), "/nonexistent/x.png")
	require.ErrorContains(t, err, "file does not exist")
	require.Empty(t, pl.setCalls())
	require.Contains(t, strings.Join(lines(), "\n"), "[ERROR] File does not exist: /nonexistent/x.png")
}

func TestSetWallpaperReportsAPlasmaFailure(t *testing.T) {
	mine := goldenFiles(t)
	pl := newFakePlasma(1)
	p, _ := onPlasma(&FakeRunner{}, pl)
	require.NoError(t, p.SetWallpaper(context.Background(), mine[0]))
	pl.err = errors.New("org.freedesktop.DBus.Error.ServiceUnknown")
	require.ErrorContains(t, p.SetWallpaperOn(context.Background(), 0, mine[0]), "ServiceUnknown")
}

// R2: screens are counted by asking each for its wallpaper until one has
// none; no screen or no bus is one monitor.
func TestMonitorCountProbesScreens(t *testing.T) {
	ctx := context.Background()
	p, _ := onPlasma(&FakeRunner{}, newFakePlasma(2))
	require.Equal(t, 2, p.MonitorCount(ctx))
	p, _ = onPlasma(&FakeRunner{}, newFakePlasma(40))
	require.Equal(t, maxScreens, p.MonitorCount(ctx))
	p, _ = onPlasma(&FakeRunner{}, newFakePlasma(0))
	require.Equal(t, 1, p.MonitorCount(ctx))
	down := newFakePlasma(2)
	down.err = errors.New("no session bus")
	p, lines := onPlasma(&FakeRunner{}, down)
	require.Equal(t, 1, p.MonitorCount(ctx))
	require.Contains(t, strings.Join(lines(), "\n"), "[DEBUG] Failed to detect monitor count")
}

// R3: one screen is set and the others are not touched.
func TestSetWallpaperOnTouchesOneScreen(t *testing.T) {
	mine := goldenFiles(t)
	pl := newFakePlasma(2)
	p, _ := onPlasma(&FakeRunner{}, pl)
	require.NoError(t, p.SetWallpaperOn(context.Background(), 1, mine[1]))
	require.Equal(t, []string{"1=" + mine[1]}, pl.setCalls())
}

// R4: a wallpaperChanged signal arrives as the screen and its image.
func TestWatchWallpapersReportsTheChangedScreensImage(t *testing.T) {
	pl := newFakePlasma(2)
	pl.images[1] = "/pics/new.jpg"
	p, _ := onPlasma(&FakeRunner{}, pl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := p.WatchWallpapers(ctx)
	require.NoError(t, err)
	pl.changes <- 1
	require.Equal(t, ScreenImage{Screen: 1, Path: "/pics/new.jpg"}, <-ch)
}

func TestPathFromImageURLDecodesWhatKDEWrites(t *testing.T) {
	require.Equal(t, "/a/b c.jpg", PathFromImageURL("file:///a/b%20c.jpg"))
	require.Equal(t, "/a/b.jpg", PathFromImageURL("file:///a/b.jpg"))
	require.Equal(t, "/plain", PathFromImageURL("/plain"))
}

// R5: the lock screen is kwriteconfig6, then the reload over D-Bus, whose
// failure does not fail the set.
func TestSetLockscreenWritesTheKeyThenReloadsOverDBus(t *testing.T) {
	mine := goldenFiles(t)
	g := loadKDEGolden(t, mine)
	r := &FakeRunner{}
	pl := newFakePlasma(1)
	pl.reloadEr = errors.New("ScreenSaver not running")
	p, _ := onPlasma(r, pl)
	require.NoError(t, p.SetLockscreen(context.Background(), mine[2]), "reload failure must not fail the set")
	require.Equal(t, [][]string{g.Lockscreen}, r.Calls())
	require.Equal(t, 1, pl.reloads)
}

func TestSetLockscreenFailsWhenKwriteconfigFails(t *testing.T) {
	mine := goldenFiles(t)
	r := &FakeRunner{Err: errors.New("exit status 1"), Stderr: "nope"}
	pl := newFakePlasma(1)
	p, _ := onPlasma(r, pl)
	err := p.SetLockscreen(context.Background(), mine[2])
	require.ErrorContains(t, err, "kwriteconfig6 failed")
	require.ErrorContains(t, err, "nope")
	require.Zero(t, pl.reloads, "no reload after a failed write")
}

func TestDebugLockscreenDumpsSectionsAndFindsTheNestedImageKey(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, "kscreenlockerrc")
	require.NoError(t, os.WriteFile(rc, []byte(`[Daemon]
Autolock=false
LockOnResume=false
lockonresume=true

[Greeter]
wallpaper=org.kde.image

[Greeter][Wallpaper][org.kde.image][General]
Image=file:///home/u/Pictures/a.png
`), 0o600))
	var buf bytes.Buffer
	require.NoError(t, debugLockscreenFile(&buf, rc))
	out := buf.String()
	require.Contains(t, out, "[DEBUG] Configuration sections found:\n")
	require.Contains(t, out, "[DEBUG]   - Daemon\n[DEBUG]     Autolock = false\n")
	require.Contains(t, out, "[DEBUG]     lockonresume = true\n", "duplicate-keys-by-case must not abort the dump")
	require.Contains(t, out, "[DEBUG]   - Greeter][Wallpaper][org.kde.image][General\n")
	require.Contains(t, out, "[DEBUG] Current lock screen wallpaper: file:///home/u/Pictures/a.png\n")
	require.Contains(t, out, "[DEBUG] Main Greeter wallpaper: org.kde.image\n")
}

func TestDebugLockscreenReportsMissingSectionsKeysAndFile(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, "kscreenlockerrc")
	require.NoError(t, os.WriteFile(rc, []byte("[Greeter][Wallpaper][org.kde.image][General]\nimage=file:///x.png\n"), 0o600))
	var buf bytes.Buffer
	require.NoError(t, debugLockscreenFile(&buf, rc))
	require.Contains(t, buf.String(), "[DEBUG] Current lock screen wallpaper: file:///x.png\n", "lowercase image key is honoured")
	require.Contains(t, buf.String(), "[DEBUG] No wallpaper key found in Greeter section\n")

	require.NoError(t, os.WriteFile(rc, []byte("[Daemon]\nTimeout=0\n"), 0o600))
	buf.Reset()
	require.NoError(t, debugLockscreenFile(&buf, rc))
	require.Contains(t, buf.String(), "[DEBUG] Wallpaper section Greeter][Wallpaper][org.kde.image][General not found\n")

	buf.Reset()
	require.NoError(t, debugLockscreenFile(&buf, filepath.Join(dir, "absent")))
	require.Contains(t, buf.String(), "[DEBUG] Configuration file does not exist: ")
}

func TestDebugLockscreenReadsTheFileKwriteconfigWrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kscreenlockerrc"),
		[]byte("[Greeter][Wallpaper][org.kde.image][General]\nImage=file:///y.png\n"), 0o600))
	var buf bytes.Buffer
	require.NoError(t, DebugLockscreen(&buf))
	require.Contains(t, buf.String(), "Current lock screen wallpaper: file:///y.png")
}

func TestPortedCleanupHelperIssuesTheSameDeleteCommandsAsThePython(t *testing.T) {
	r := &FakeRunner{}
	ev, _ := captureEvents()
	cleanLockscreenConfig(context.Background(), r, ev)
	calls := r.Calls()
	require.Len(t, calls, 5)
	require.Equal(t, []string{"kwriteconfig6", "--file", "kscreenlockerrc", "--group", "Greeter", "--group", "Wallpaper",
		"--group", "org.kde.image", "--group", "General", "--key", "image", "--delete"}, calls[0])
	require.Equal(t, []string{"kwriteconfig6", "--file", "kscreenlockerrc", "--group", "Greeter", "--key", "wallpaper", "--delete"}, calls[1])
	for i, key := range []string{"lockonresume", "timeout", "autolock"} {
		require.Equal(t, []string{"kwriteconfig6", "--file", "kscreenlockerrc", "--group", "Daemon", "--key", key, "--delete"}, calls[2+i])
	}
}

func TestPortedReloadHelperTriesEveryServiceMethodPairAndReportsAnySuccess(t *testing.T) {
	r := &FakeRunner{Handler: func(_ context.Context, argv []string) (string, string, error) {
		if argv[1] == "org.kde.screensaver" && argv[3] == "configure" {
			return "", "", nil
		}
		return "", "", errors.New("exit status 1")
	}}
	ev, lines := captureEvents()
	require.True(t, reloadScreensaverConfig(context.Background(), r, ev))
	require.Len(t, r.Calls(), 4)
	require.Contains(t, strings.Join(lines(), "\n"), "Successfully called configure on org.kde.screensaver")

	ev2, lines2 := captureEvents()
	require.False(t, reloadScreensaverConfig(context.Background(), &FakeRunner{Err: errors.New("x")}, ev2))
	require.Contains(t, strings.Join(lines2(), "\n"), "[WARN] All attempts to reload screen saver configuration failed")
}

// --- live KDE (opt-in) ---

func TestLiveKDESetsWallpaperAndLockscreenThroughRealTools(t *testing.T) {
	if os.Getenv("CLOCKWORK_LIVE_KDE") != "1" {
		t.Skip("set CLOCKWORK_LIVE_KDE=1 to run against the real Plasma session")
	}
	ctx := context.Background()
	fixture := filepath.Join(repoRoot(t), "tests", "golden", "images", "landscape_1920x1080.png")
	ev, lines := captureEvents()
	p := NewWithEvents(ExecRunner{}, ev)

	// Remember the current lock-screen image so the developer's setting is
	// restored afterwards; the desktop wallpaper is left on the fixture.
	rcPath := filepath.Join(configHome(), "kscreenlockerrc")
	previous := currentLockscreenImage(t, rcPath)
	t.Cleanup(func() {
		argv := KwriteconfigArgs("")
		if previous == "" {
			argv = append(argv[:len(argv)-1], "--delete")
		} else {
			argv[len(argv)-1] = previous
		}
		_, _, _ = ExecRunner{}.Run(ctx, argv[0], argv[1:]...)
	})

	require.NoError(t, p.SetWallpaper(ctx, fixture), strings.Join(lines(), "\n"))
	require.GreaterOrEqual(t, p.MonitorCount(ctx), 1)
	require.NoError(t, p.SetLockscreen(ctx, fixture), strings.Join(lines(), "\n"))
	require.Equal(t, "file://"+fixture, currentLockscreenImage(t, rcPath))
}

func currentLockscreenImage(t *testing.T, rcPath string) string {
	t.Helper()
	f, err := os.Open(rcPath)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	sections, err := parseKDERC(f)
	require.NoError(t, err)
	for _, s := range sections {
		if s.Name == lockscreenWallpaperSection {
			v, _ := s.lookup("Image")
			return v
		}
	}
	return ""
}
