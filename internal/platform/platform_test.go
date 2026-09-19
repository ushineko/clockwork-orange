package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJSStringEscapeLeavesPlainPathsUntouchedSoGoldenScriptsStayByteEqual(t *testing.T) {
	for _, p := range []string{
		"/home/user/Pictures/wall paper.png",
		"C:/Users/x/img.jpg",
		"/tmp/ünïcödé/🍊.png",
	} {
		require.Equal(t, p, JSStringEscape(p))
	}
}

func TestJSStringEscapeEscapesQuoteBackslashAndControlCharacters(t *testing.T) {
	require.Equal(t, `a\"b`, JSStringEscape(`a"b`))
	require.Equal(t, `a\\b`, JSStringEscape(`a\b`))
	require.Equal(t, `a\nb\rc\td`, JSStringEscape("a\nb\rc\td"))
	require.Equal(t, `x\u0001y\u001by`, JSStringEscape("x\x01y\x1by"))
	// A path crafted to break out of the JS string literal cannot.
	escaped := JSStringEscape(`/tmp/x"); evil(); ("`)
	require.NotContains(t, strings.ReplaceAll(escaped, `\"`, ""), `"`)
}

func TestExecRunnerReturnsStdoutStderrAndExitErrorSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	stdout, stderr, err := ExecRunner{}.Run(context.Background(), "sh", "-c", "echo out; echo err >&2; exit 3")
	require.Equal(t, "out\n", stdout)
	require.Equal(t, "err\n", stderr)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 3, exitErr.ExitCode())
}

func TestExecRunnerReportsMissingProgramAsErrNotFound(t *testing.T) {
	_, _, err := ExecRunner{}.Run(context.Background(), "clockwork-definitely-missing-binary-xyz")
	require.ErrorIs(t, err, exec.ErrNotFound)
}

func TestExecRunnerHonoursContextDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := ExecRunner{}.Run(ctx, "sleep", "5")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 3*time.Second)
}

func TestFakeRunnerRecordsArgvAndAnswersFromHandler(t *testing.T) {
	f := &FakeRunner{Handler: func(_ context.Context, argv []string) (string, string, error) {
		if argv[0] == "boom" {
			return "", "bad", errors.New("exit status 1")
		}
		return "ok", "", nil
	}}
	out, _, err := f.Run(context.Background(), "x", "a", "b")
	require.NoError(t, err)
	require.Equal(t, "ok", out)
	_, stderr, err := f.Run(context.Background(), "boom")
	require.Error(t, err)
	require.Equal(t, "bad", stderr)
	require.Equal(t, [][]string{{"x", "a", "b"}, {"boom"}}, f.Calls())
	f.Reset()
	require.Empty(t, f.Calls())
}

func TestUnitFileIsTheR45TextWithNoPythonInterpreter(t *testing.T) {
	unit := string(UnitFile)
	require.Contains(t, unit, "ExecStart=/usr/bin/clockwork-orange --service\n")
	require.NotContains(t, unit, "python")
	require.NotContains(t, unit, " -u ")
	for _, line := range []string{
		"[Unit]", "Description=Clockwork Orange - Wallpaper Service", "After=graphical-session.target",
		"[Service]", "Type=simple", "Environment=DISPLAY=:0", "Restart=always", "RestartSec=10",
		"[Install]", "WantedBy=default.target",
	} {
		require.Contains(t, unit, line+"\n")
	}
}

// A unit installed by a binary outside /usr/bin names that binary; the
// packaged path yields the shipped file unchanged.
func TestUnitFileForRewritesExecStartOnly(t *testing.T) {
	require.Equal(t, UnitFile, UnitFileFor("/usr/bin/clockwork-orange"))
	local := string(UnitFileFor("/home/me/.local/bin/clockwork-orange"))
	require.Contains(t, local, "ExecStart=/home/me/.local/bin/clockwork-orange --service\n")
	require.NotContains(t, local, "/usr/bin/")
	require.Equal(t, len(UnitFile)+len("/home/me/.local/bin")-len("/usr/bin"), len(local), "nothing else changes")
}

func TestNewReturnsTheHostPlatformNamedAfterGOOS(t *testing.T) {
	p := New(&FakeRunner{})
	require.Equal(t, runtime.GOOS, p.Name())
	if runtime.GOOS == "linux" {
		require.True(t, p.LockscreenSupported())
	} else {
		require.False(t, p.LockscreenSupported())
		require.ErrorIs(t, p.SetLockscreen(context.Background(), "x.png"), ErrUnsupported)
	}
}

func TestServiceOnEveryOSReportsAStateTheGUIKnows(t *testing.T) {
	s := NewService(&FakeRunner{Stdout: "inactive\n", Err: errors.New("exit status 3")})
	require.Equal(t, StateInactive, s.IsActive(context.Background()))
}

/*
The unit never names the window (spec 015).

Install takes the unit's ExecStart from the running binary, and the window can
install the service too. Before this fix it wrote
ExecStart=/usr/bin/clockwork-orange-gui --service; that binary does not take
--service, exits 2 on the unknown flag, and Restart=always started it again ten
seconds later for as long as the unit stayed enabled.
*/
func TestTheDaemonBesideTheWindowIsTheDaemon(t *testing.T) {
	cases := []struct {
		exe   string
		want  string
		isGUI bool
	}{
		{"/usr/bin/clockwork-orange-gui", "/usr/bin/clockwork-orange", true},
		{"/home/me/.local/bin/clockwork-orange-gui", "/home/me/.local/bin/clockwork-orange", true},
		{`C:\Program Files\co\clockwork-orange-gui.exe`, `C:\Program Files\co\clockwork-orange.exe`, true},
		{"/usr/bin/clockwork-orange", "", false},
		{"/home/me/.local/bin/clockwork-orange", "", false},
		// "-gui" in a parent directory is not the window: only the basename counts.
		{"/opt/clockwork-orange-gui/bin/clockwork-orange", "", false},
		// The test binary, which is what the Install tests run as.
		{"/tmp/go-build/platform.test", "", false},
	}
	for _, c := range cases {
		got, isGUI := daemonBeside(filepath.FromSlash(c.exe))
		require.Equalf(t, c.isGUI, isGUI, "exe %q", c.exe)
		if !c.isGUI {
			continue
		}
		require.Equalf(t, filepath.FromSlash(c.want), got, "exe %q", c.exe)
	}
}

// Whatever DaemonExecutable answers, it is never the window: that is the
// property the unit depends on.
func TestDaemonExecutableIsNeverTheWindow(t *testing.T) {
	exe := DaemonExecutable()
	require.NotEmpty(t, exe)
	_, isGUI := daemonBeside(exe)
	require.Falsef(t, isGUI, "the unit would run the window: %s", exe)
	require.NotContains(t, string(UnitFileFor(exe)), "clockwork-orange-gui")
}

// A unit that cannot start gives up rather than restarting every ten seconds
// for as long as it is enabled (spec 015 R3).
func TestTheUnitBoundsItsRestarts(t *testing.T) {
	unit := string(UnitFile)
	require.Contains(t, unit, "StartLimitIntervalSec=120")
	require.Contains(t, unit, "StartLimitBurst=5")
	require.Contains(t, unit, "RestartSec=10")
	limit := strings.Index(unit, "StartLimitIntervalSec")
	service := strings.Index(unit, "[Service]")
	require.Positive(t, limit)
	require.Less(t, limit, service, "StartLimit* are [Unit] directives, not [Service] ones")
}

/*
The window installing the service writes a unit for the daemon beside it
(spec 015 R1).

The window and the daemon are installed together -- by the packages into
/usr/bin, by install.sh into ~/.local/bin -- so the sibling is where to look
first. With no sibling and nothing on $PATH, the packaged path is the answer:
wrong only on a machine where the daemon is somewhere else entirely, and a
great deal less wrong than a unit naming the window.
*/
func TestTheWindowInstallsAUnitForTheDaemonBesideIt(t *testing.T) {
	dir := t.TempDir()
	gui := filepath.Join(dir, "clockwork-orange-gui")
	daemon := filepath.Join(dir, "clockwork-orange")
	require.NoError(t, os.WriteFile(gui, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(daemon, []byte("#!/bin/sh\n"), 0o755))

	require.Equal(t, daemon, daemonFor(gui), "the sibling daemon")
	require.Contains(t, string(UnitFileFor(daemonFor(gui))), "ExecStart="+daemon+" --service")

	// No sibling: $PATH, then the packaged path. Emptying $PATH leaves the
	// last resort, which must still not be the window.
	lonely := filepath.Join(t.TempDir(), "clockwork-orange-gui")
	require.NoError(t, os.WriteFile(lonely, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", "")
	require.Equal(t, packagedDaemon, daemonFor(lonely))
	_, isGUI := daemonBeside(daemonFor(lonely))
	require.False(t, isGUI)
}
