package platform

import (
	"context"
	"errors"
	"os/exec"
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
