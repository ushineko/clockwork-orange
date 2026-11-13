//go:build !sdembed

package sdlib

import "io/fs"

// embedded is nil in a build without the sdembed tag.
var embedded fs.FS
