/*
Package buildinfo carries what this binary was built from.

Copied from nmsbonker (same author) — keep in sync by hand.

It is a package of its own so that the -ldflags path points at program metadata
rather than at some feature package that happens to be convenient.
*/
package buildinfo

// Version and Commit are injected at build time via -ldflags -X (spec 010
// R1.4). The defaults are what an unadorned `go build` or `go test` produces.
var (
	Version = "dev"
	Commit  = "unknown"
)

// UserAgent identifies this program to remote services. It carries the program
// name and version and nothing that identifies the machine or the user.
func UserAgent() string { return "clockwork-orange/" + Version }
