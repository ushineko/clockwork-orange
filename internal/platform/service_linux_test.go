//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsActiveTrustsStdoutEvenWhenSystemctlExitsNonZero(t *testing.T) {
	// `systemctl is-active` exits 3 for an inactive unit; the state is still
	// on stdout and must not be read as "unknown".
	r := &FakeRunner{Stdout: "inactive\n", Err: errors.New("exit status 3")}
	s := NewService(r)
	require.Equal(t, StateInactive, s.IsActive(context.Background()))
	require.Equal(t, [][]string{{"systemctl", "--user", "is-active", "clockwork-orange.service"}}, r.Calls())

	require.Equal(t, StateActive, NewService(&FakeRunner{Stdout: "active\n"}).IsActive(context.Background()))
	require.Equal(t, StateFailed, NewService(&FakeRunner{Stdout: "failed\n", Err: errors.New("exit status 3")}).IsActive(context.Background()))
}

func TestIsActiveIsUnknownWhenSystemctlProducesNothing(t *testing.T) {
	notFound := &FakeRunner{Err: fmt.Errorf("systemctl: %w", exec.ErrNotFound)}
	require.Equal(t, StateUnknown, NewService(notFound).IsActive(context.Background()))
	require.Equal(t, StateUnknown, NewService(&FakeRunner{Stdout: "  \n"}).IsActive(context.Background()))
}

func TestSystemctlAndJournalctlCallsCarryTheirTimeouts(t *testing.T) {
	var deadlines []time.Duration
	r := &FakeRunner{Handler: func(ctx context.Context, argv []string) (string, string, error) {
		dl, ok := ctx.Deadline()
		require.True(t, ok, "%v must run with a deadline", argv)
		deadlines = append(deadlines, time.Until(dl))
		return "", "", nil
	}}
	s := NewService(r)
	ctx := context.Background()
	s.IsActive(ctx)
	s.StatusDetails(ctx)
	require.NoError(t, s.Start(ctx))
	require.NoError(t, s.Stop(ctx))
	require.NoError(t, s.Restart(ctx))
	s.Logs(ctx, 50)
	require.Len(t, deadlines, 6)
	for _, d := range deadlines[:5] {
		require.InDelta(t, systemctlTimeout, d, float64(time.Second))
	}
	require.InDelta(t, journalctlTimeout, deadlines[5], float64(time.Second))
}

func TestStatusDetailsReturnsStdoutForInactiveUnitsAndErrorTextOtherwise(t *testing.T) {
	status := "○ clockwork-orange.service - Clockwork Orange\n     Active: inactive (dead)\n"
	r := &FakeRunner{Stdout: status, Err: errors.New("exit status 3")}
	s := NewService(r)
	require.Equal(t, status, s.StatusDetails(context.Background()))
	require.Equal(t, [][]string{{"systemctl", "--user", "status", "clockwork-orange.service", "--no-pager"}}, r.Calls())

	missing := NewService(&FakeRunner{Err: fmt.Errorf("systemctl: %w", exec.ErrNotFound)})
	require.Contains(t, missing.StatusDetails(context.Background()), "executable file not found")

	withStderr := NewService(&FakeRunner{Err: errors.New("exit status 4"), Stderr: "Unit clockwork-orange.service could not be found.\n"})
	require.Equal(t, "exit status 4\nUnit clockwork-orange.service could not be found.", withStderr.StatusDetails(context.Background()))
}

func TestStartStopRestartWrapFailuresWithStderr(t *testing.T) {
	r := &FakeRunner{Err: errors.New("exit status 5"), Stderr: "Failed to start: Unit not found.\n"}
	s := NewService(r)
	ctx := context.Background()
	for _, op := range []struct {
		name string
		fn   func(context.Context) error
	}{{"start", s.Start}, {"stop", s.Stop}, {"restart", s.Restart}} {
		err := op.fn(ctx)
		require.ErrorContains(t, err, "systemctl --user "+op.name+" clockwork-orange.service")
		require.ErrorContains(t, err, "Unit not found")
	}
	require.Equal(t, [][]string{
		{"systemctl", "--user", "start", "clockwork-orange.service"},
		{"systemctl", "--user", "stop", "clockwork-orange.service"},
		{"systemctl", "--user", "restart", "clockwork-orange.service"},
	}, r.Calls())
}

func TestInstallWritesEmbeddedUnitUnderXDGConfigHomeThenReloadsAndEnables(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	r := &FakeRunner{}
	require.NoError(t, NewService(r).Install(context.Background()))

	got, err := os.ReadFile(filepath.Join(dir, "systemd", "user", "clockwork-orange.service"))
	require.NoError(t, err)
	exe, err := os.Executable()
	require.NoError(t, err)
	require.Equal(t, string(UnitFileFor(exe)), string(got), "ExecStart names the binary that installed the unit")
	require.NotContains(t, string(got), "/usr/bin/clockwork-orange", "the test binary is not in /usr/bin")
	require.Equal(t, [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "clockwork-orange.service"},
	}, r.Calls())
}

func TestInstallFallsBackToHomeConfigWhenXDGConfigHomeIsUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	require.NoError(t, NewService(&FakeRunner{}).Install(context.Background()))
	require.FileExists(t, filepath.Join(home, ".config", "systemd", "user", "clockwork-orange.service"))
}

func TestInstallReportsEnableFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := &FakeRunner{Handler: func(_ context.Context, argv []string) (string, string, error) {
		if argv[2] == "enable" {
			return "", "Failed to enable unit\n", errors.New("exit status 1")
		}
		return "", "", nil
	}}
	err := NewService(r).Install(context.Background())
	require.ErrorContains(t, err, "systemctl --user enable clockwork-orange.service")
	require.ErrorContains(t, err, "Failed to enable unit")
}

func TestUninstallIgnoresStopAndDisableFailuresRemovesUnitAndReloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	unit := filepath.Join(dir, "systemd", "user", "clockwork-orange.service")
	require.NoError(t, os.MkdirAll(filepath.Dir(unit), 0o750))
	require.NoError(t, os.WriteFile(unit, UnitFile, 0o600))

	r := &FakeRunner{Handler: func(_ context.Context, argv []string) (string, string, error) {
		switch argv[2] {
		case "stop", "disable":
			return "", "not loaded", errors.New("exit status 5")
		}
		return "", "", nil
	}}
	require.NoError(t, NewService(r).Uninstall(context.Background()))
	require.NoFileExists(t, unit)
	require.Equal(t, [][]string{
		{"systemctl", "--user", "stop", "clockwork-orange.service"},
		{"systemctl", "--user", "disable", "clockwork-orange.service"},
		{"systemctl", "--user", "daemon-reload"},
	}, r.Calls())
}

func TestUninstallSucceedsWhenUnitFileIsAlreadyGone(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, NewService(&FakeRunner{}).Uninstall(context.Background()))
}

func TestLogsUsesJournalctlWithRequestedLineCount(t *testing.T) {
	r := &FakeRunner{Stdout: "Sep 15 10:00:00 host clockwork-orange[1]: [DEBUG] cycle\n"}
	s := NewService(r)
	out := s.Logs(context.Background(), 50)
	require.Equal(t, r.Stdout, out)
	require.Equal(t, [][]string{{"journalctl", "--user", "-u", "clockwork-orange.service", "--no-pager", "-n", "50"}}, r.Calls())

	failing := NewService(&FakeRunner{Err: fmt.Errorf("journalctl: %w", exec.ErrNotFound)})
	require.True(t, strings.HasPrefix(failing.Logs(context.Background(), 10), "Error retrieving logs: "))
}

func TestLinuxServiceNameIsTheUnitName(t *testing.T) {
	require.Equal(t, "clockwork-orange.service", NewService(&FakeRunner{}).Name())
}

// --- live systemd (opt-in) ---

func TestLiveSystemdInstallThenUninstallRoundTripsTheUnitFile(t *testing.T) {
	if os.Getenv("CLOCKWORK_LIVE_SYSTEMD") != "1" {
		t.Skip("set CLOCKWORK_LIVE_SYSTEMD=1 to run against the real user systemd")
	}
	ctx := context.Background()
	live := NewService(ExecRunner{})
	if live.IsActive(ctx) == StateActive {
		t.Skip("clockwork-orange.service is active on this machine; not disturbing it")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", ServiceName)); err == nil {
		t.Skip("a real clockwork-orange.service unit is installed; Uninstall would disable it")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	unit := filepath.Join(dir, "systemd", "user", ServiceName)

	// The user manager resolves units from its own environment, not from a
	// redirected XDG_CONFIG_HOME, so `enable` may legitimately report that
	// the unit does not exist. The file write and daemon-reload are what this
	// test pins; the enable outcome is logged.
	if err := live.Install(ctx); err != nil {
		t.Logf("Install returned %v (expected when the manager cannot see %s)", err, dir)
	}
	require.FileExists(t, unit)
	require.NoError(t, live.Uninstall(ctx))
	require.NoFileExists(t, unit)
}
