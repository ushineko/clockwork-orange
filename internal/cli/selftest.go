package cli

import (
	"github.com/spf13/cobra"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// runSelfTest prints one line per probe and the verdict, exit 0 iff every
// probe passed (R6.6). It runs before the config is read: a broken
// installation must be diagnosable without a valid config file.
func runSelfTest(cmd *cobra.Command, a *App, g *globalFlags, offline bool) error {
	req, err := a.request(cmd, g)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	say(out, "Running Clockwork Orange self-test...")
	res := core.SelfTest(cmd.Context(), core.SelfTestRequest{Request: req, SkipNetwork: offline})
	for _, c := range res.Checks {
		tag := "[OK]  "
		if !c.OK {
			tag = "[FAIL]"
		}
		say(out, "%s %s: %s", tag, c.Name, c.Detail)
	}
	say(out, "All passed: %t", res.OK)
	if !res.OK {
		return &ExitError{Code: ExitFailure}
	}
	return nil
}
