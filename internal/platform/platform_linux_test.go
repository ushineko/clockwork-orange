//go:build linux

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestKDEScriptsReproduceThePythonTextByteForByte(t *testing.T) {
	mine := goldenFiles(t)
	g := loadKDEGolden(t, mine)
	require.Equal(t, g.Single[4], KDESingleScript(mine[0]))
	require.Equal(t, g.Multi[4], KDEMultiScript(mine[:2]))
	require.Equal(t, g.Lockscreen, KwriteconfigArgs(mine[2]))
}

func TestSetWallpaperSendsTheGoldenQdbusArgv(t *testing.T) {
	mine := goldenFiles(t)
	g := loadKDEGolden(t, mine)
	r := &FakeRunner{}
	p := New(r)
	require.NoError(t, p.SetWallpaper(context.Background(), mine[0]))
	require.Equal(t, [][]string{g.Single}, r.Calls())
}

func TestSetWallpaperMultiSendsTheGoldenQdbusArgv(t *testing.T) {
	mine := goldenFiles(t)
	g := loadKDEGolden(t, mine)
	r := &FakeRunner{}
	require.NoError(t, New(r).SetWallpaperMulti(context.Background(), mine[:2]))
	require.Equal(t, [][]string{g.Multi}, r.Calls())
}

func TestSetLockscreenSendsGoldenKwriteconfigThenBestEffortReload(t *testing.T) {
	mine := goldenFiles(t)
	g := loadKDEGolden(t, mine)
	r := &FakeRunner{Handler: func(_ context.Context, argv []string) (string, string, error) {
		if argv[0] == "qdbus6" {
			return "", "Service 'org.freedesktop.ScreenSaver' does not exist.", errors.New("exit status 1")
		}
		return "", "", nil
	}}
	require.NoError(t, New(r).SetLockscreen(context.Background(), mine[2]), "reload failure must not fail the set")
	require.Equal(t, [][]string{g.Lockscreen, g.Reload}, r.Calls())
}

func TestSetLockscreenFailsWhenKwriteconfigFails(t *testing.T) {
	mine := goldenFiles(t)
	r := &FakeRunner{Err: errors.New("exit status 1"), Stderr: "nope"}
	err := New(r).SetLockscreen(context.Background(), mine[2])
	require.ErrorContains(t, err, "kwriteconfig6 failed")
	require.ErrorContains(t, err, "nope")
	require.Len(t, r.Calls(), 1, "no reload after a failed write")
}

func TestPathsWithQuotesAndBackslashesCannotBreakOutOfTheScript(t *testing.T) {
	dir := resolvedTempDir(t)
	p := touch(t, dir, `we"ird\name.png`)
	r := &FakeRunner{}
	require.NoError(t, New(r).SetWallpaper(context.Background(), p))
	script := r.Calls()[0][4]
	want := `d.writeConfig("Image", "file://` + dir + `/we\"ird\\name.png");`
	require.Contains(t, script, want)
	require.NoError(t, New(r).SetWallpaperMulti(context.Background(), []string{p}))
	require.Contains(t, r.Calls()[1][4], `var images = ["file://`+dir+`/we\"ird\\name.png"];`)
}

func TestSetWallpaperRefusesMissingFileWithoutCallingQdbus(t *testing.T) {
	r := &FakeRunner{}
	ev, lines := captureEvents()
	err := NewWithEvents(r, ev).SetWallpaper(context.Background(), "/nonexistent/x.png")
	require.ErrorContains(t, err, "file does not exist")
	require.Empty(t, r.Calls())
	require.Contains(t, strings.Join(lines(), "\n"), "[ERROR] File does not exist: /nonexistent/x.png")
}

func TestSetWallpaperReportsMissingQdbusAndNonZeroExitWithStderr(t *testing.T) {
	mine := goldenFiles(t)
	ctx := context.Background()

	notFound := &FakeRunner{Err: fmt.Errorf("qdbus6: %w", exec.ErrNotFound)}
	require.EqualError(t, New(notFound).SetWallpaper(ctx, mine[0]), "qdbus6 not found")

	failed := &FakeRunner{Err: errors.New("exit status 1"), Stderr: "Cannot find 'org.kde.plasmashell'\n"}
	err := New(failed).SetWallpaper(ctx, mine[0])
	require.ErrorContains(t, err, "qdbus6 failed")
	require.ErrorContains(t, err, "Cannot find 'org.kde.plasmashell'")
	require.EqualError(t, New(notFound).SetWallpaperMulti(ctx, mine[:2]), "qdbus6 not found")
}

func TestSetWallpaperMultiSkipsMissingFilesAndFailsOnlyWhenNoneRemain(t *testing.T) {
	mine := goldenFiles(t)
	r := &FakeRunner{}
	ev, lines := captureEvents()
	p := NewWithEvents(r, ev)
	ctx := context.Background()

	require.NoError(t, p.SetWallpaperMulti(ctx, []string{mine[0], "/nonexistent/a.png", mine[1]}))
	script := r.Calls()[0][4]
	require.Contains(t, script, `"file://`+mine[0]+`", "file://`+mine[1]+`"`)
	require.NotContains(t, script, "nonexistent")
	require.Contains(t, strings.Join(lines(), "\n"), "[ERROR] File does not exist: /nonexistent/a.png")

	r.Reset()
	require.EqualError(t, p.SetWallpaperMulti(ctx, []string{"/nonexistent/a.png"}), "no valid image paths found")
	require.EqualError(t, p.SetWallpaperMulti(ctx, nil), "no image paths provided")
	require.Empty(t, r.Calls())
}

func TestMonitorCountParsesPlasmaOutputAndFallsBackToOne(t *testing.T) {
	ctx := context.Background()
	r := &FakeRunner{Stdout: "3\n"}
	require.Equal(t, 3, New(r).MonitorCount(ctx))
	require.Equal(t, [][]string{{"qdbus6", "org.kde.plasmashell", "/PlasmaShell",
		"org.kde.PlasmaShell.evaluateScript", "print(desktops().length)"}}, r.Calls())

	ev, lines := captureEvents()
	require.Equal(t, 1, NewWithEvents(&FakeRunner{Stdout: "undefined\n"}, ev).MonitorCount(ctx))
	require.Equal(t, 1, NewWithEvents(&FakeRunner{Err: fmt.Errorf("x: %w", exec.ErrNotFound)}, ev).MonitorCount(ctx))
	require.Equal(t, 1, NewWithEvents(&FakeRunner{Err: errors.New("exit status 1")}, ev).MonitorCount(ctx))
	joined := strings.Join(lines(), "\n")
	require.Contains(t, joined, "[DEBUG] qdbus6 not found, assuming 1 monitor")
	require.Contains(t, joined, "[DEBUG] Failed to detect monitor count")
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
