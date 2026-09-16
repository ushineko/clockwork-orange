package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// newBlacklistCmd is the Blacklist section as commands (R6.5, R7.8).
func newBlacklistCmd(a *App, g *globalFlags) *cobra.Command {
	bl := group("blacklist", "Images that will never be downloaded again")
	list := &cobra.Command{
		Use: "list", Short: "List blacklisted images, newest first", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			items, err := core.BlacklistList(cmd.Context(), req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(items) == 0 {
				say(out, "The blacklist is empty.")
				return nil
			}
			say(out, "%-64s  %-16s  %s", "HASH", "DATE ADDED", "SOURCE")
			for _, it := range items {
				say(out, "%-64s  %-16s  %s", it.Hash, it.Date, it.Source)
			}
			return nil
		},
	}
	remove := &cobra.Command{
		Use: "remove <hash>...", Short: "Remove hashes from the blacklist", Args: minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			if err := core.BlacklistRemove(cmd.Context(), core.BlacklistRemoveRequest{Request: req, Hashes: args}); err != nil {
				return err
			}
			say(cmd.OutOrStdout(), "Removed %d hash(es) from the blacklist", len(args))
			return nil
		},
	}
	add := &cobra.Command{
		Use:   "add <file>...",
		Short: "Blacklist image files and delete them",
		Long:  "Hashes each file, stores the hash with a thumbnail, and deletes the file, exactly what the GUI's review mode does when you mark an image.",
		Args:  minArgs(1),
	}
	source := add.Flags().String("plugin", "manual", "plugin name recorded as the source")
	add.RunE = func(cmd *cobra.Command, args []string) error {
		req, err := a.request(cmd, g)
		if err != nil {
			return err
		}
		n, err := core.BlacklistAdd(cmd.Context(), core.BlacklistAddRequest{Request: req, Paths: args, Plugin: *source})
		if err != nil {
			return err
		}
		say(cmd.OutOrStdout(), "Blacklisted and removed %d file(s)", n)
		return nil
	}
	bl.AddCommand(list, remove, add)
	return bl
}

// newHistoryCmd is the History section as commands (R6.5, R7.7).
func newHistoryCmd(a *App, g *globalFlags) *cobra.Command {
	h := group("history", "The download history that keeps plugins from fetching the same image twice")
	stats := &cobra.Command{
		Use: "stats", Short: "Show how many downloads and unique images are recorded", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			s, err := core.HistoryStats(cmd.Context(), req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			say(out, "Total downloads tracked: %d", s.TotalRecords)
			say(out, "Unique images:           %d", s.UniqueImages)
			say(out, "Database size:           %s", humanSize(s.DBSizeBytes))
			return nil
		},
	}
	clearCmd := &cobra.Command{
		Use: "clear", Short: "Delete every history record (plugins may download previously seen images again)", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			if err := core.HistoryClear(cmd.Context(), req); err != nil {
				return err
			}
			say(cmd.OutOrStdout(), "History cleared")
			return nil
		},
	}
	imp := &cobra.Command{
		Use:   "import [directory]",
		Short: "Record existing JPEG files as already downloaded",
		Long: "Scans a directory of *.jpg / *.jpeg files and records each as downloaded, so plugins\n" +
			"skip them. Defaults to the DuckDuckGo Images download directory from the config.",
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := a.request(cmd, g)
			if err != nil {
				return err
			}
			var dir string
			if len(args) == 1 {
				dir = args[0]
			}
			res, err := core.HistoryImport(cmd.Context(), core.HistoryImportRequest{Request: req, Dir: dir})
			if err != nil {
				return err
			}
			say(cmd.OutOrStdout(), "Scanned %s: %d imported, %d skipped", res.Dir, res.Imported, res.Skipped)
			return nil
		},
	}
	h.AddCommand(stats, clearCmd, imp)
	return h
}

// humanSize renders a byte count the way the History section does: B, KB or
// MB, with the exact count beside it so a script still has a number.
func humanSize(n int64) string {
	const kb, mb = 1024, 1024 * 1024
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB (%d bytes)", float64(n)/mb, n)
	case n >= kb:
		return fmt.Sprintf("%.1f KB (%d bytes)", float64(n)/kb, n)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// pad right-pads s to width for a column, never truncating.
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
