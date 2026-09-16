//go:build linux

package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/cli"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/platform"
)

// The service subcommands against the real user systemd, in a throwaway
// XDG_CONFIG_HOME (spec 010 R9.3, AC "service install|uninstall|...").
// Opt-in, and it steps aside when a real clockwork-orange unit is installed
// or running on this machine.
func TestLiveSystemdServiceSubcommands(t *testing.T) {
	if os.Getenv("CLOCKWORK_LIVE_SYSTEMD") != "1" {
		t.Skip("set CLOCKWORK_LIVE_SYSTEMD=1 to run against the real user systemd")
	}
	ctx := context.Background()
	live := platform.NewService(platform.ExecRunner{})
	if live.IsActive(ctx) == platform.StateActive {
		t.Skip("clockwork-orange.service is active on this machine; not disturbing it")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", platform.ServiceName)); err == nil {
		t.Skip("a real clockwork-orange.service unit is installed; uninstall would disable it")
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	unit := filepath.Join(dir, "systemd", "user", platform.ServiceName)
	run := func(args ...string) result {
		var out, errb strings.Builder
		code := cli.NewApp(cli.Options{Deps: &core.Deps{Service: live}}).Execute(ctx, args, &out, &errb)
		return result{code: code, stdout: out.String(), stderr: errb.String()}
	}

	// The user manager resolves units from its own environment, not from a
	// redirected XDG_CONFIG_HOME, so `enable` may report that the unit does
	// not exist; the unit file and daemon-reload are what this pins.
	res := run("service", "install")
	t.Logf("install: exit %d\n%s%s", res.code, res.stdout, res.stderr)
	require.FileExists(t, unit)

	res = run("service", "status")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.Contains(t, res.stdout, "Service: "+platform.ServiceName)

	for _, verb := range []string{"start", "stop", "restart"} {
		res = run("service", verb)
		t.Logf("%s: exit %d\n%s%s", verb, res.code, res.stdout, res.stderr)
	}
	res = run("service", "logs", "-n", "3")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)

	res = run("service", "uninstall")
	require.Equal(t, cli.ExitOK, res.code, res.stderr)
	require.NoFileExists(t, unit)
}
