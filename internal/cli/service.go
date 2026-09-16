package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// newServiceCmd is the systemd user unit control the GUI's Service section
// has (R6.5, R4.5). On Windows and macOS every verb reports that there is no
// service to manage, as the Python did.
func newServiceCmd(a *App, g *globalFlags) *cobra.Command {
	svc := group("service", "Control the background service (systemd user unit on Linux)")
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
	svc.AddCommand(
		verb("install", "Write and enable the user unit", core.ServiceInstall, "Service installed and enabled"),
		verb("uninstall", "Stop, disable and remove the user unit", core.ServiceUninstall, "Service uninstalled"),
		verb("start", "Start the service", core.ServiceStart, "Service started"),
		verb("stop", "Stop the service", core.ServiceStop, "Service stopped"),
		verb("restart", "Restart the service", core.ServiceRestart, "Service restarted"),
		&cobra.Command{
			Use: "status", Short: "Show whether the service is running, with systemctl's details", Args: noArgs(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				req, err := a.request(cmd, g)
				if err != nil {
					return err
				}
				st := core.ServiceStatus(cmd.Context(), req)
				out := cmd.OutOrStdout()
				say(out, "Service: %s", st.Name)
				say(out, "State:   %s", st.State)
				if st.Details != "" {
					say(out, "")
					say(out, "%s", st.Details)
				}
				return nil
			},
		},
	)
	logs := &cobra.Command{
		Use: "logs", Short: "Print the tail of the service journal", Args: noArgs(),
	}
	lines := logs.Flags().IntP("lines", "n", 50, "number of journal lines")
	logs.RunE = func(cmd *cobra.Command, _ []string) error {
		req, err := a.request(cmd, g)
		if err != nil {
			return err
		}
		say(cmd.OutOrStdout(), "%s", core.ServiceLogs(cmd.Context(), core.ServiceLogsRequest{Request: req, Lines: *lines}))
		return nil
	}
	svc.AddCommand(logs)
	return svc
}
