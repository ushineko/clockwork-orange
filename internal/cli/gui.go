package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

// guiBinary is the desktop front end, a separate binary because it needs CGO
// and OpenGL and the daemon must not (D2).
const guiBinary = "clockwork-orange-gui"

// guiOverrideEnv names the GUI binary explicitly; for running from a build
// tree, and for tests.
const guiOverrideEnv = "CLOCKWORK_ORANGE_GUI"

/*
guiPath finds the GUI binary (R6.2): the override variable, then the
directory holding this executable (how the packages install both), then PATH.
*/
func guiPath() (string, error) {
	if p := os.Getenv(guiOverrideEnv); p != "" {
		return p, nil
	}
	name := guiBinary
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(beside); err == nil {
			return beside, nil
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("locate %s: %w", name, err)
	}
	return p, nil
}

/*
launchGUI runs the GUI binary with this process's stdio and returns its exit
status (R6.2). It is a child rather than an exec(2) replacement so the same
code serves Windows, where there is no exec; the parent is idle while the
window is open.
*/
func launchGUI(ctx context.Context, cmd *cobra.Command) error {
	path, err := guiPath()
	if err != nil {
		return &ExitError{Code: ExitFailure, Msg: fmt.Sprintf(
			"the graphical interface was not found (%v).\n"+
				"Install the %s binary next to this one or on PATH, "+
				"or use --file, --directory, --url or --plugin for a command-line operation.", err, guiBinary)}
	}
	child := exec.CommandContext(ctx, path) //nolint:gosec // G204: the path is our own sibling binary or an explicit operator override
	child.Stdin = os.Stdin
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()
	if err := child.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return &ExitError{Code: exit.ExitCode()}
		}
		return fmt.Errorf("start %s: %w", path, err)
	}
	return nil
}
