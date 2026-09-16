//go:build tools

// This file pins the module's direct dependencies before the packages that use
// them exist (spec 010 Phase 1), so that `go mod tidy` run from any package
// during parallel development does not drop them. It is never compiled.
package tools

import (
	_ "fyne.io/fyne/v2"
	_ "github.com/fsnotify/fsnotify"
	_ "github.com/spf13/cobra"
	_ "github.com/stretchr/testify/require"
	_ "golang.org/x/image/draw"
	_ "golang.org/x/sys/unix"
	_ "gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)
