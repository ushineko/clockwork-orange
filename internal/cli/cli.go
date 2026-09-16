/*
Package cli assembles the clockwork-orange command tree (spec 010 R6).

Every command does the same three things: build a core request from flags,
call one core operation, and print the result. Nothing here reaches past core
into platform, plugins or the stores, because the GUI cannot either, and the
two front ends may disagree only about presentation (tests/parity).

The root command is the v2.9.5 argparse surface, flag for flag, so existing
systemd units, desktop entries and shell habits keep working. The subcommands
(`service`, `blacklist`, `history`, `plugins`, `plugin`, `config`, `version`)
are new in v4 and give the CLI every operation the GUI has (R6.5).

Output is plain text with no ANSI colour: one fact per line, or a fixed-width
table for lists. Log lines go to stderr with the Python `[LEVEL]` prefixes.
*/
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/buildinfo"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/events"
)

// Exit codes (R6.1): success, an operation that ran and failed, and a misuse
// of the command line, so a script can tell the last two apart.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// UsageError marks a failure caused by how the command was invoked. Execute
// turns it into exit code 2, the way argparse's parser.error did (R6.1).
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// ExitError carries an exit code that is neither 1 nor 2: the GUI child's own
// status, or a self-test that already printed its verdict. An empty Msg means
// nothing more should be printed.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

// Options configure a command tree. The zero value is production.
type Options struct {
	// Deps supplies the outside world to every core operation; nil means the
	// host desktop, the user's databases and the default HTTP client.
	Deps *core.Deps
}

// App is a built command tree and the dependencies it will close on exit.
type App struct {
	Root *cobra.Command
	deps *core.Deps
}

// globalFlags are the flags every command shares.
type globalFlags struct {
	configPath string
	logLevel   string
}

// NewApp builds the command tree.
func NewApp(o Options) *App {
	deps := o.Deps
	if deps == nil {
		deps = &core.Deps{}
	}
	g := &globalFlags{}
	root := &cobra.Command{
		Use:   "clockwork-orange",
		Short: "Set the desktop wallpaper or lock screen from a URL, a file, a directory or the configured plugins",
		Long: "clockwork-orange sets the desktop wallpaper or the lock screen image from a URL,\n" +
			"a local file, a random image in a directory, or the plugins enabled in\n" +
			"~/.config/clockwork-orange.yml (Linux KDE Plasma 6, Windows 10/11, macOS 13+).\n" +
			"With no arguments it starts the graphical interface.",
		Example: `  clockwork-orange -f /path/to/image.jpg          # set from a local file
  clockwork-orange -d /path/to/wallpapers -w 30    # cycle a directory every 30 s
  clockwork-orange --lockscreen -d ~/Pictures      # random lock screen image
  clockwork-orange --desktop --lockscreen -d ~/Pictures -w 120   # different images, both surfaces
  clockwork-orange --plugin wallhaven              # download via one plugin, then set
  clockwork-orange --service                       # daemon mode: enabled plugins, 900 s default
  clockwork-orange --desktop --lockscreen -d ~/Pictures -w 300 --write-config`,
		Version: versionString(),
		// Errors are reported once, by Execute, with the program name. Leaving
		// cobra's own reporting on prints every failure twice.
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &UsageError{Err: err} })

	pf := root.PersistentFlags()
	pf.StringVar(&g.configPath, "config", "", "configuration file (default ~/.config/clockwork-orange.yml)")
	pf.StringVar(&g.logLevel, "log-level", "debug", "lowest log level printed to stderr: debug, info, warn or error")

	a := &App{Root: root, deps: deps}
	addRootFlags(root, a, g)
	root.AddCommand(
		newServiceCmd(a, g),
		newBlacklistCmd(a, g),
		newHistoryCmd(a, g),
		newPluginsCmd(a, g),
		newPluginCmd(a, g),
		newConfigCmd(a, g),
		newVersionCmd(),
	)
	return a
}

// Root builds a production command tree (used by tests/parity).
func Root() *cobra.Command { return NewApp(Options{}).Root }

/*
Execute runs the tree with args, prints any failure to stderr once, closes
the stores and returns the process exit code (R6.1).

SIGINT and SIGTERM cancel the context, which ends a cycling loop cleanly: the
Python printed "Received signal, shutting down gracefully" and exited 0, and
so does this.
*/
func (a *App) Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	a.Root.SetArgs(args)
	a.Root.SetOut(stdout)
	a.Root.SetErr(stderr)
	err := a.Root.ExecuteContext(ctx)
	if cerr := a.deps.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err == nil {
		return ExitOK
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		if exit.Msg != "" {
			_, _ = fmt.Fprintln(stderr, "clockwork-orange:", exit.Msg)
		}
		return exit.Code
	}
	_, _ = fmt.Fprintln(stderr, "clockwork-orange:", err)
	var usage *UsageError
	if errors.As(err, &usage) {
		return ExitUsage
	}
	return ExitFailure
}

// request builds the core request every operation takes, logging to the
// command's stderr.
func (a *App) request(cmd *cobra.Command, g *globalFlags) (core.Request, error) {
	level, err := parseLevel(g.logLevel)
	if err != nil {
		return core.Request{}, err
	}
	return core.Request{
		ConfigPath: g.configPath,
		Events:     newEvents(cmd.ErrOrStderr(), level),
		Deps:       a.deps,
	}, nil
}

// parseLevel maps the --log-level word to an events.Level.
func parseLevel(s string) (events.Level, error) {
	switch s {
	case "debug":
		return events.LevelDebug, nil
	case "info":
		return events.LevelInfo, nil
	case "warn", "warning":
		return events.LevelWarn, nil
	case "error":
		return events.LevelError, nil
	default:
		return 0, usagef("--log-level must be debug, info, warn or error, not %q", s)
	}
}

// group builds a command that only holds subcommands, with the same
// unknown-subcommand behaviour as the root.
func group(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help() //nolint:wrapcheck // cobra's own writer error, nothing to add
			}
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
}

// noArgs is cobra.NoArgs with the error marked as a usage error.
func noArgs() cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		return nil
	}
}

// exactArgs is cobra.ExactArgs with the error marked as a usage error.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("%s takes exactly %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// minArgs is cobra.MinimumNArgs with the same treatment.
func minArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return usagef("%s takes at least %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// maxArgs is cobra.MaximumNArgs with the same treatment.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return usagef("%s takes at most %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

func versionString() string {
	return fmt.Sprintf("%s (%s)", core.Version(), buildinfo.Commit)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version and commit this binary was built from",
		Args:  noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			say(cmd.OutOrStdout(), "clockwork-orange %s", versionString())
			return nil
		},
	}
}

// say prints one line to w. Write errors on stdout are not actionable here.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}
