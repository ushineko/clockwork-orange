/*
Command clockwork-orange-gui is the desktop front end (spec 010 R7).

It is a separate binary from cmd/clockwork-orange on purpose. This one needs
CGO, OpenGL and a display server; the daemon needs none of those and must
not, because it is what systemd runs on a machine that may have no session
yet. Neither binary imports the other's front end.

The flags are parsed with the standard library rather than cobra: there are
four, two of them exist so a capture script can deep-link into a section, and
pulling a command framework in for that would put a second command tree in a
project whose whole point is that there is one.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/buildinfo"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/gui"
)

func main() {
	section := flag.String("section", "",
		"open on this section: "+strings.Join(gui.SectionNames(), ", "))
	scheme := flag.String("scheme", "",
		"use this colour scheme for this run without saving it: "+strings.Join(gui.SchemeNames(), ", "))
	// The same flag the CLI takes, spelled the same way, because the two front
	// ends read one document and pointing them at different ones is a thing
	// you do deliberately rather than by accident.
	configPath := flag.String("config", "", "configuration file (default ~/.config/clockwork-orange.yml)")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *version {
		fmt.Printf("clockwork-orange-gui %s (%s)\n", core.Version(), buildinfo.Commit)
		os.Exit(0)
	}

	gui.Run(gui.Options{ConfigPath: *configPath, Section: *section, Scheme: *scheme})
}
