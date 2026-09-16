// Command clockwork-orange-gui is the Fyne desktop front end (spec 010 R7). It
// requires CGO and OpenGL.
package main

import (
	"fmt"
	"os"

	"github.com/ushineko/clockwork-orange/internal/buildinfo"
)

func main() {
	fmt.Fprintln(os.Stderr, "clockwork-orange-gui", buildinfo.Version, "not implemented yet (spec 010 Phase 5)")
	os.Exit(1)
}
