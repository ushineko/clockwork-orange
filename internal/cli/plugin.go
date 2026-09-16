package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// newPluginsCmd lists the registry (R6.5).
func newPluginsCmd(a *App, g *globalFlags) *cobra.Command {
	pl := group("plugins", "The wallpaper source plugins compiled into this build")
	pl.AddCommand(&cobra.Command{
		Use: "list", Short: "List plugins and whether each is enabled in the config", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			infos, err := core.PluginsList(cmd.Context(), req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			say(out, "%s  %s  %s", pad("NAME", 18), pad("ENABLED", 7), "DESCRIPTION")
			for _, p := range infos {
				enabled := "no"
				if p.Enabled {
					enabled = "yes"
				}
				say(out, "%s  %s  %s", pad(p.Name, 18), pad(enabled, 7), p.Description)
			}
			return nil
		},
	})
	return pl
}

/*
newPluginCmd runs one plugin without setting a wallpaper: the GUI's Download
Now and Reset & Run buttons (R6.5, R7.5). Unlike the root's --plugin flag it
stops after the download and reports the path the plugin produced.
*/
func newPluginCmd(a *App, g *globalFlags) *cobra.Command {
	pc := group("plugin", "Run one plugin")
	run := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a plugin now and print the path it produced",
		Args:  exactArgs(1),
	}
	force := run.Flags().Bool("force", false, "ignore the plugin's interval and run now")
	reset := run.Flags().Bool("reset", false, "delete the plugin's download directory before running")
	override := run.Flags().String("plugin-config", "", "JSON object merged over the plugin's block in the config file")
	run.RunE = func(cmd *cobra.Command, args []string) error {
		req, err := a.request(cmd, g)
		if err != nil {
			return err
		}
		var ov map[string]any
		if *override != "" {
			if err := json.Unmarshal([]byte(*override), &ov); err != nil {
				return usagef("--plugin-config is not a JSON object: %v", err)
			}
		}
		res, err := core.RunPlugin(cmd.Context(), core.RunPluginRequest{
			Request: req, Name: args[0], Override: ov, Force: *force, Reset: *reset,
		})
		if err != nil {
			return fmt.Errorf("plugin %s: %w", args[0], err)
		}
		out := cmd.OutOrStdout()
		if res.Message != "" {
			say(out, "%s", res.Message)
		}
		say(out, "%s", res.Path)
		return nil
	}
	pc.AddCommand(run)
	return pc
}
