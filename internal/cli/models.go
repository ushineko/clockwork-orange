package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/core"
)

/*
newModelsCmd manages the Stable Diffusion models (spec 031 R6, R7): `models
list` shows the catalog and what is already on disk, `models pull` downloads
one with progress, resume and a checksum.
*/
func newModelsCmd(a *App, g *globalFlags) *cobra.Command {
	m := group("models", "Stable Diffusion models for the stable_diffusion plugin")
	m.AddCommand(&cobra.Command{
		Use: "list", Short: "List the models, their size and licence, and which are pulled", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			infos, err := core.ListModels(cmd.Context(), core.ListModelsRequest{Request: req})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			say(out, "%s  %s  %s  %s  %s", pad("NAME", 14), pad("SIZE", 8), pad("PULLED", 6), pad("LICENSE", 26), "DESCRIPTION")
			for _, mi := range infos {
				pulled := "no"
				if mi.Pulled {
					pulled = "yes"
				}
				say(out, "%s  %s  %s  %s  %s", pad(mi.Name, 14), pad(humanBytes(mi.SizeBytes), 8), pad(pulled, 6),
					pad(mi.License, 26), mi.Description)
			}
			return nil
		},
	})
	m.AddCommand(&cobra.Command{
		Use: "pull <name>", Short: "Download a model (resumes a partial download, verifies the checksum)",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			res, err := core.PullModel(cmd.Context(), core.PullModelRequest{Request: req, Name: args[0]})
			if err != nil {
				return fmt.Errorf("models pull %s: %w", args[0], err)
			}
			say(cmd.OutOrStdout(), "%s", res.Path)
			return nil
		},
	})
	return m
}

// humanBytes renders a size the way the catalog quotes it: 4.3 GB.
func humanBytes(n int64) string {
	const gb, mb = 1e9, 1e6
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/mb)
	}
	return fmt.Sprintf("%d B", n)
}
