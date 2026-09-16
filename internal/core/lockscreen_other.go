//go:build !linux

package core

import (
	"io"

	"github.com/ushineko/clockwork-orange/internal/platform"
)

// DebugLockscreen is Linux/KDE only.
func DebugLockscreen(io.Writer) error { return platform.ErrUnsupported }
