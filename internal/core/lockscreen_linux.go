package core

import (
	"io"

	"github.com/ushineko/clockwork-orange/internal/platform"
)

// DebugLockscreen dumps ~/.config/kscreenlockerrc (R4.4, `--debug-lockscreen`).
func DebugLockscreen(w io.Writer) error { return platform.DebugLockscreen(w) }
