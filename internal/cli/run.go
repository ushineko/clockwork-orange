package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/events"
)

/*
rootFlags is the v2.9.5 argparse surface (R6.1). `--run-plugin` is gone: it
re-entered the frozen Python interpreter to run a plugin subprocess, and the
Go plugins run in-process (D3, DV8); cobra rejects it as unknown, exit 2.
*/
type rootFlags struct {
	lockscreen, desktop  bool
	url, file, directory string
	plugin, pluginConfig string
	wait                 int
	debugLockscreen      bool
	writeConfig          bool
	gui, service         bool
	selfTest, offline    bool
}

// operationFlags are the root's own flags. When none is set, the bare
// invocation starts the GUI (R6.2); `--config` and `--log-level` alone do
// not count.
var operationFlags = []string{ //nolint:gochecknoglobals // a fixed table
	"lockscreen", "desktop", "url", "file", "directory", "plugin", "plugin-config", "wait",
	"debug-lockscreen", "write-config", "gui", "service", "self-test", "offline",
}

func addRootFlags(root *cobra.Command, a *App, g *globalFlags) {
	f := &rootFlags{}
	fl := root.Flags()
	fl.BoolVar(&f.lockscreen, "lockscreen", false, "set the lock screen image instead of the desktop wallpaper")
	fl.BoolVar(&f.desktop, "desktop", false, "set the desktop wallpaper (combine with --lockscreen for both)")
	fl.StringVarP(&f.url, "url", "u", "", "download an image from URL and set it as the wallpaper")
	fl.StringVarP(&f.file, "file", "f", "", "set the wallpaper from a local file")
	fl.StringVarP(&f.directory, "directory", "d", "", "set a random wallpaper from a directory")
	fl.StringVar(&f.plugin, "plugin", "", "use one plugin as the source: "+strings.Join(core.AvailablePluginNames(), ", "))
	fl.StringVar(&f.pluginConfig, "plugin-config", "", "JSON object merged over the plugin's block in the config file")
	fl.IntVarP(&f.wait, "wait", "w", 0, "seconds between wallpaper changes (cycles until interrupted)")
	fl.BoolVar(&f.debugLockscreen, "debug-lockscreen", false, "show the current lock screen configuration and exit")
	fl.BoolVar(&f.writeConfig, "write-config", false, "write the configuration file from the current options and exit")
	fl.BoolVar(&f.gui, "gui", false, "start the graphical interface")
	fl.BoolVar(&f.service, "service", false, "background service mode (default wait 900 s when unspecified)")
	fl.BoolVar(&f.selfTest, "self-test", false, "verify the environment and dependencies, exit 0 when everything passes")
	fl.BoolVar(&f.offline, "offline", false, "with --self-test: skip the network probe")
	_ = fl.MarkHidden("offline")

	root.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		if !anyChanged(cmd, operationFlags) {
			return launchGUI(cmd.Context(), cmd)
		}
		if f.selfTest {
			return runSelfTest(cmd, a, g, f.offline)
		}
		if f.gui {
			return launchGUI(cmd.Context(), cmd)
		}
		return runLegacy(cmd, a, g, f)
	}
}

func anyChanged(cmd *cobra.Command, names []string) bool {
	return slices.ContainsFunc(names, cmd.Flags().Changed)
}

/*
runLegacy is main() from clockwork-orange.py after argument parsing: load and
merge the config, validate, then dispatch on mode (R6.3, R6.4). Exit codes
follow the Python: parser.error cases return a UsageError (2), an operation
that ran and failed returns a plain error (1).
*/
func runLegacy(cmd *cobra.Command, a *App, g *globalFlags, f *rootFlags) error {
	ctx := cmd.Context()
	req, err := a.request(cmd, g)
	if err != nil {
		return err
	}
	ev := req.Events

	if err := checkExclusive(f); err != nil {
		return err
	}
	if f.plugin != "" && !slices.Contains(core.AvailablePluginNames(), f.plugin) {
		return usagef("argument --plugin: invalid choice: '%s' (choose from %s)", f.plugin, quoteList(core.AvailablePluginNames()))
	}

	ev.Debugf("Merging configuration with command line arguments")
	loaded, err := core.LoadConfig(ctx, req)
	if err != nil {
		return err
	}
	doc := loaded.Doc
	if !loaded.Exists {
		// load_config_file returned {} for a missing file, so no default_wait
		// applied and `-d dir` set once instead of cycling. Defaults() is the
		// GUI's starting point, not the CLI's.
		doc = config.Document{}
	}
	mode := core.ResolveMode(doc, f.desktop, f.lockscreen)
	if mode != core.ModeDefault {
		ev.Debugf("Resolved %s mode from flags and config", mode)
	}
	wait, waitSet := mergeWait(f, doc, cmd.Flags().Changed("wait"), ev)

	if err := validate(f, mode, wait, waitSet, len(doc.EnabledPlugins()) > 0); err != nil {
		return err
	}

	if f.debugLockscreen {
		return core.DebugLockscreen(cmd.OutOrStdout())
	}
	if f.writeConfig {
		return writeConfig(ctx, req, f, mode, wait, ev)
	}

	file, dir := f.file, f.directory
	if f.plugin != "" {
		file, dir, err = resolvePlugin(ctx, req, f, &doc, ev)
		if err != nil {
			return err
		}
	}

	target := targetDescription(mode)
	ev.Debugf("Starting %s set process", target)
	err = perform(ctx, req, mode, wait, f.url, file, dir, ev)
	// A signal ended a cycling loop: a clean exit, as in the Python.
	interrupted := ctx.Err() != nil || errors.Is(err, context.Canceled)
	if interrupted {
		ev.Debugf("Received signal, shutting down gracefully")
		return nil
	}
	if err != nil {
		ev.Errorf("%v", err)
		return fmt.Errorf("failed to set %s: %w", target, err)
	}
	ev.Debugf("%s set successfully", capitalize(target))
	ev.Debugf("Process completed")
	return nil
}

// sourceFlags are the mutually exclusive source flags, in argparse's display
// form and in declaration order.
var sourceFlags = []struct{ name, display string }{ //nolint:gochecknoglobals // a fixed table
	{"url", "-u/--url"}, {"file", "-f/--file"}, {"directory", "-d/--directory"}, {"plugin", "--plugin"},
}

// checkExclusive is argparse's mutually exclusive group over -u/-f/-d/--plugin.
func checkExclusive(f *rootFlags) error {
	values := map[string]string{"url": f.url, "file": f.file, "directory": f.directory, "plugin": f.plugin}
	var set []string
	for _, s := range sourceFlags {
		if values[s.name] != "" {
			set = append(set, s.display)
		}
	}
	if len(set) > 1 {
		return usagef("argument %s: not allowed with argument %s", set[1], set[0])
	}
	return nil
}

func quoteList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = "'" + s + "'"
	}
	return strings.Join(q, ", ")
}

/*
mergeWait is the wait half of merge_config_with_args (R2.5): a zero flag
counts as unset (Python `if not merged_args.wait`), the config's default_wait
fills in, then --service supplies 900. waitSet reports whether a value exists
at all, which is what `args.wait is not None` tested.
*/
func mergeWait(f *rootFlags, doc config.Document, flagGiven bool, ev events.Events) (wait int, waitSet bool) {
	if f.wait != 0 {
		return f.wait, true
	}
	switch {
	case doc.DefaultWait != 0:
		ev.Debugf("Set default wait interval from config: %d", doc.DefaultWait)
		return doc.DefaultWait, true
	case f.service:
		ev.Debugf("Service mode: defaulting to %ds wait interval", config.DefaultWaitService)
		return config.DefaultWaitService, true
	default:
		// An explicit `-w 0` with nothing to fall back on survives as 0 and
		// fails validation, as in the Python.
		return 0, flagGiven
	}
}

/*
validate is _validate_args, message for message (R6.1). The two dual-mode
checks in the Python were unreachable (the --url check above them fires
first, and the source check is the same test as the first one), so they are
not repeated here.
*/
func validate(f *rootFlags, mode core.Mode, wait int, waitSet, hasEnabledPlugins bool) error {
	if f.file == "" && f.directory == "" && f.plugin == "" && !hasEnabledPlugins {
		return usagef("Operation requires either --file, --directory, --plugin, or enabled plugins in config")
	}
	if waitSet && wait <= 0 {
		return usagef("--wait must be a positive integer")
	}
	if f.url != "" && (mode == core.ModeLockscreen || mode == core.ModeDual) {
		return usagef("--url cannot be used with --lockscreen (lock screen requires local files)")
	}
	return nil
}

// writeConfig is `--write-config` (R2.7).
func writeConfig(ctx context.Context, req core.Request, f *rootFlags, mode core.Mode, wait int, ev events.Events) error {
	_, err := core.WriteConfig(ctx, core.WriteConfigRequest{
		Request:    req,
		Desktop:    mode == core.ModeDesktop || mode == core.ModeDual,
		Lockscreen: mode == core.ModeLockscreen || mode == core.ModeDual,
		Dual:       mode == core.ModeDual,
		Wait:       wait,
		URL:        f.url, File: f.file, Directory: f.directory,
	})
	if err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	ev.Debugf("Configuration file written successfully")
	return nil
}

/*
resolvePlugin is _handle_specific_plugin_execution (R6.4): run the named
plugin with --plugin-config merged over its block, then treat a directory
result as -d and a file result as -f. Every failure here is exit 1, as in the
Python, including malformed JSON.
*/
func resolvePlugin(ctx context.Context, req core.Request, f *rootFlags, doc *config.Document, ev events.Events) (file, dir string, err error) {
	ev.Debugf("Plugin mode: %s", f.plugin)
	var override map[string]any
	if f.pluginConfig != "" {
		if err := json.Unmarshal([]byte(f.pluginConfig), &override); err != nil {
			return "", "", fmt.Errorf("invalid JSON in --plugin-config: %w", err)
		}
	}
	res, err := core.RunPlugin(ctx, core.RunPluginRequest{Request: req, Name: f.plugin, Override: override, Doc: doc})
	if err != nil {
		return "", "", fmt.Errorf("plugin execution failed: %w", err)
	}
	ev.Debugf("Plugin returned path: %s", res.Path)
	info, err := os.Stat(res.Path)
	switch {
	case err != nil:
		return "", "", fmt.Errorf("plugin returned invalid path: %s", res.Path)
	case info.IsDir():
		ev.Debugf("Plugin resolved to directory")
		return "", res.Path, nil
	default:
		ev.Debugf("Plugin resolved to file")
		return res.Path, "", nil
	}
}

/*
perform is _perform_wallpaper_operation (R6.3). The four Python handlers
differ only in wording and in which source flags they honour, so one dispatch
on the source covers them: a URL, a file, a directory (once or cycling), or
the enabled plugins (once or cycling). The lock-screen and dual handlers
never see --url because validate rejected it.
*/
func perform(ctx context.Context, req core.Request, mode core.Mode, wait int, url, file, dir string, ev events.Events) error {
	interval := time.Duration(wait) * time.Second
	switch {
	case url != "":
		ev.Debugf("%sURL mode: %s", modePrefix(mode), url)
		return core.SetFromURL(ctx, core.SetURLRequest{Request: req, URL: url})
	case file != "":
		return core.SetFromFile(ctx, core.SetRequest{Request: req, Mode: mode, Path: file})
	case dir != "":
		if wait > 0 {
			ev.Debugf("%scontinuous mode with %d second intervals", modePrefix(mode), wait)
			return core.RunLoop(ctx, core.LoopRequest{Request: req, Mode: mode, Wait: interval, Sources: []string{dir}})
		}
		return core.SetFromDirectory(ctx, core.SetRequest{Request: req, Mode: mode, Path: dir})
	default:
		if wait > 0 {
			return core.RunLoop(ctx, core.LoopRequest{Request: req, Mode: mode, Wait: interval})
		}
		_, err := core.Cycle(ctx, core.CycleRequest{Request: req, Mode: mode})
		if errors.Is(err, core.ErrNoEnabledPlugins) && mode == core.ModeDefault {
			return core.ErrNoSource
		}
		return err
	}
}

// targetDescription is _get_target_description.
func targetDescription(mode core.Mode) string {
	switch mode {
	case core.ModeDual:
		return "both desktop and lock screen"
	case core.ModeLockscreen:
		return "lock screen"
	case core.ModeDesktop:
		return "desktop"
	default:
		return "wallpaper"
	}
}

// modePrefix is the "Desktop " / "Lock screen " prefix the Python put on its
// per-mode debug lines, and nothing in default mode.
func modePrefix(mode core.Mode) string {
	switch mode {
	case core.ModeDesktop:
		return "Desktop "
	case core.ModeLockscreen:
		return "Lock screen "
	case core.ModeDual:
		return "Dual wallpaper "
	default:
		return ""
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
