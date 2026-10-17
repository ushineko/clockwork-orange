package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// newWallpaperCmd is what is on screen and the hold on it (spec 026): the
// Service section's status card and its Pause, Resume and Skip, and the
// Review tab's Pin.
func newWallpaperCmd(a *App, g *globalFlags) *cobra.Command {
	wp := group("wallpaper", "Show the current wallpaper, pause, skip or pin it")
	verb := func(use, short string, op func(context.Context, core.Request) error, done string) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short, Args: noArgs(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				req, err := a.request(cmd, g)
				if err != nil {
					return err
				}
				if err := op(cmd.Context(), req); err != nil {
					return err
				}
				say(cmd.OutOrStdout(), "%s", done)
				return nil
			},
		}
	}
	status := &cobra.Command{
		Use: "status", Short: "Show the wallpaper on each monitor and the time of the next change", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			st, err := core.RotationStatus(cmd.Context(), req)
			printRotation(cmd.OutOrStdout(), st, time.Now())
			return err
		},
	}

	pin := &cobra.Command{
		Use:   "pin <image>",
		Short: "Show an image now and keep it until resume",
		Long: "Show an image now and keep it until `wallpaper resume`.\n\n" +
			"Without a target flag it pins every monitor. The pin survives a restart.",
		Args: exactArgs(1),
	}
	monitor := pin.Flags().Int("monitor", 0, "pin one monitor, numbered from 1")
	lock := pin.Flags().Bool("lockscreen", false, "pin the lock screen")
	pin.Flags().Bool("all", false, "pin every monitor (the default)")
	pin.MarkFlagsMutuallyExclusive("monitor", "lockscreen", "all")
	pin.RunE = func(cmd *cobra.Command, args []string) error {
		req, err := a.request(cmd, g)
		if err != nil {
			return err
		}
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve %s: %w", args[0], err)
		}
		target := core.AllMonitors
		if cmd.Flags().Changed("monitor") {
			if *monitor < 1 {
				return usagef("--monitor counts from 1")
			}
			target = *monitor - 1
		}
		if err := core.RotationPin(cmd.Context(), core.PinRequest{Request: req, Path: path, Monitor: target, Lockscreen: *lock}); err != nil {
			return err
		}
		say(cmd.OutOrStdout(), "Pinned %s", path)
		return nil
	}

	wp.AddCommand(
		status,
		verb("pause", "Keep the wallpapers on screen now until resume", core.RotationPause, "Paused"),
		verb("resume", "Clear every pin and rotate again", core.RotationResume, "Resumed"),
		verb("skip", "Change the wallpaper now; a paused display stays paused on the new one", core.RotationSkip, "Skipped"),
		pin,
	)
	return wp
}

// printRotation is `wallpaper status`.
func printRotation(w io.Writer, r core.RotationStatusResult, now time.Time) {
	st := r.State
	switch {
	case st.Rotator == "":
		say(w, "Rotator:     none has run yet")
	case r.Running:
		say(w, "Rotator:     %s (running)", st.Rotator)
	default:
		say(w, "Rotator:     %s (not running)", st.Rotator)
	}
	if st.Mode != "" {
		say(w, "Mode:        %s", st.Mode)
	}
	for i, s := range st.Desktop {
		say(w, "Monitor %d:   %s%s", i+1, shownLine(s, now), pinMark(r.Control.Desktop[i] == s.Path))
	}
	if st.Lockscreen != nil {
		say(w, "Lock screen: %s%s", shownLine(*st.Lockscreen, now), pinMark(r.Control.Lockscreen == st.Lockscreen.Path))
	}
	switch {
	case r.Paused:
		say(w, "Next change: paused")
	case st.NextChange != nil && r.Running:
		say(w, "Next change: %s (in %s)", st.NextChange.Local().Format("15:04:05"), max(st.NextChange.Sub(now), 0).Round(time.Second))
	default:
		say(w, "Next change: none scheduled")
	}
}

func shownLine(s core.Shown, now time.Time) string {
	if s.Since.IsZero() {
		return s.Path
	}
	return fmt.Sprintf("%s (for %s)", s.Path, now.Sub(s.Since).Round(time.Second))
}

func pinMark(pinned bool) string {
	if pinned {
		return " [pinned]"
	}
	return ""
}
