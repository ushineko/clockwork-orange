//go:build sdembed

package sdlib

import (
	"embed"
	"io/fs"
)

//go:embed all:embedded
var embeddedFiles embed.FS

// embedded holds the libraries staged into internal/sdlib/embedded by
// `make sd-lib`.
var embedded fs.FS = mustSub(embeddedFiles, "embedded")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
