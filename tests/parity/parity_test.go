//go:build parity

/*
Package parity guards that the CLI and the GUI expose the same operations
(spec 010 R9.4). It is the only place that imports both front ends.

Copied from nmsbonker (same author) — keep in sync by hand.

It is a package of its own, behind a build tag, for two reasons. It imports
both front ends, which no other test does and no binary does — the CLI must
not link the GUI at all — and `make test` passes the tag deliberately, so the
guard runs on every test run rather than when someone remembers to ask for
it. It needs no display: nothing here constructs a window.

Phase 4 (this file) pins the CLI half: every leaf command maps to exactly one
core operation, and the exceptions carry reasons. Phase 5 adds gui.Actions()
and the two-way comparison; until then the GUI side is the table below.

If this fails, the fix is to implement the missing side, not to edit the
allow-list.
*/
package parity

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/cli"
)

/*
coreOperations is the R6.5 table: each leaf command and the one core operation
it renders. A leaf missing here is a command that landed without deciding
what it is; an entry without a leaf is a command that was removed without
updating the contract.
*/
var coreOperations = map[string]string{
	"service install":   "core.ServiceInstall",
	"service uninstall": "core.ServiceUninstall",
	"service start":     "core.ServiceStart",
	"service stop":      "core.ServiceStop",
	"service restart":   "core.ServiceRestart",
	"service status":    "core.ServiceStatus",
	"service logs":      "core.ServiceLogs",
	"blacklist list":    "core.BlacklistList",
	"blacklist remove":  "core.BlacklistRemove",
	"blacklist add":     "core.BlacklistAdd",
	"history stats":     "core.HistoryStats",
	"history clear":     "core.HistoryClear",
	"history import":    "core.HistoryImport",
	"plugins list":      "core.PluginsList",
	"plugin run":        "core.RunPlugin",
	"config show":       "core.LoadConfig",
	"config migrate":    "core.LoadConfig",
	"version":           "core.Version",
}

/*
notInTheGUI are commands and root flags with no GUI affordance, each with the
reason. An entry here is a decision someone made and can be argued with,
which is the difference between an exception and an omission (R9.4).
*/
var notInTheGUI = map[string]string{
	"version":            "the window title and About already show what it prints",
	"config migrate":     "migrations run on every load; the GUI loads on start",
	"--self-test":        "an installation probe for packaging and CI, run before there is a window",
	"--debug-lockscreen": "a KDE diagnostic dump for bug reports",
	"--write-config":     "the GUI auto-saves; the flag exists for the systemd unit and scripts",
	"completion":         "generates shell completion scripts, which a window cannot use",
	"help":               "cobra's help command",
}

// Every leaf in the command tree is a documented core operation or a
// documented exception, and every documented operation still has its leaf.
func TestEveryLeafCommandMapsToOneCoreOperation(t *testing.T) {
	got := map[string]bool{}
	for _, name := range leaves(cli.Root(), "") {
		if _, skip := notInTheGUI[name]; skip && coreOperations[name] == "" {
			continue
		}
		got[name] = true
		require.NotEmptyf(t, coreOperations[name], "%q is a leaf command with no core operation recorded", name)
	}
	require.NotEmpty(t, got, "the command tree came back empty, so this proves nothing")
	var missing []string
	for name := range coreOperations {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	require.Emptyf(t, missing, "operations in the table with no command: %v", missing)
}

// The root command keeps every v2.9.5 flag (R6.1) and no --run-plugin (DV8).
func TestRootKeepsTheLegacyFlagSurface(t *testing.T) {
	root := cli.Root()
	for _, name := range []string{
		"lockscreen", "desktop", "url", "file", "directory", "plugin", "plugin-config", "wait",
		"debug-lockscreen", "write-config", "gui", "service", "self-test",
	} {
		require.NotNilf(t, root.Flags().Lookup(name), "--%s is a v2.9.5 flag", name)
	}
	for short, long := range map[string]string{"u": "url", "f": "file", "d": "directory", "w": "wait"} {
		require.Equalf(t, long, root.Flags().ShorthandLookup(short).Name, "-%s", short)
	}
	require.Nil(t, root.Flags().Lookup("run-plugin"), "--run-plugin is dropped (DV8)")
}

// The allow-list is the interesting half of this file, so it is held to the
// same rule as the rest: an entry with no reason beside it is an omission
// somebody wrote down rather than a decision anybody made.
func TestEveryAllowListEntryCarriesAReason(t *testing.T) {
	root := cli.Root()
	for name, reason := range notInTheGUI {
		require.NotEmptyf(t, strings.TrimSpace(reason), "%q is allow-listed with no reason given", name)
		if strings.HasPrefix(name, "--") {
			require.NotNilf(t, root.Flags().Lookup(strings.TrimPrefix(name, "--")), "%s is allow-listed but not a root flag", name)
		}
	}
}

// leaves returns the full path of every command that actually does something —
// a group with subcommands is a heading, not an operation.
func leaves(cmd *cobra.Command, prefix string) []string {
	var out []string
	for _, c := range cmd.Commands() {
		name := strings.Fields(c.Use)[0]
		path := strings.TrimSpace(prefix + " " + name)
		if len(c.Commands()) == 0 {
			out = append(out, path)
			continue
		}
		out = append(out, leaves(c, path)...)
	}
	return out
}
