/*
Command clockwork-orange is the CLI and daemon (spec 010 R6).

It builds with CGO_ENABLED=0 and needs no display: it drives the desktop
through qdbus6/kwriteconfig6 on KDE, the registry and SystemParametersInfo on
Windows, and osascript on macOS. The graphical interface is the separate
clockwork-orange-gui binary, which this one starts when run with no arguments
or with --gui.
*/
package main

import (
	"context"
	"os"

	"github.com/ushineko/clockwork-orange/internal/cli"
)

func main() {
	os.Exit(cli.NewApp(cli.Options{}).Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
