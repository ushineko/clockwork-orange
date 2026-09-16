//go:build parity

/*
Package parity guards that the CLI and the GUI expose the same operations
(spec 010 R9.4). It is the only place that imports both front ends.

Copied from nmsbonker (same author) — keep in sync by hand. Filled in at
Phase 4 (CLI) and Phase 5 (GUI); until then it only asserts the harness runs.
*/
package parity

import "testing"

func TestParityHarnessRuns(t *testing.T) {
	t.Log("parity guard placeholder: populated in Phase 4/5")
}
