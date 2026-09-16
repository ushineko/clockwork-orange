package cli

import (
	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
)

// newConfigCmd shows and migrates the configuration file (R6.5).
func newConfigCmd(a *App, g *globalFlags) *cobra.Command {
	cc := group("config", "The configuration file")
	show := &cobra.Command{
		Use: "show", Short: "Print the effective configuration as YAML", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			loaded, err := core.LoadConfig(cmd.Context(), req)
			if err != nil {
				return err
			}
			b, err := config.Marshal(loaded.Doc)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if loaded.Exists {
				say(out, "# %s", loaded.Path)
			} else {
				say(out, "# %s does not exist; these are the defaults", loaded.Path)
			}
			_, _ = out.Write(b)
			return nil
		},
	}
	migrate := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending configuration migrations and rewrite the file",
		Long: "Loads the configuration, which applies every migration (currently google_images ->\n" +
			"duckduckgo_images) and persists the result when anything changed. Loading does this\n" +
			"on every run; this command exists to do it deliberately and report the outcome.",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			loaded, err := core.LoadConfig(cmd.Context(), req)
			if err != nil {
				return err
			}
			if !loaded.Exists {
				say(cmd.OutOrStdout(), "No configuration file at %s; nothing to migrate", loaded.Path)
				return nil
			}
			say(cmd.OutOrStdout(), "Configuration at %s is current", loaded.Path)
			return nil
		},
	}
	cc.AddCommand(show, migrate)
	return cc
}
