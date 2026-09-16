//go:build !linux && !windows && !darwin

package platform

import (
	"context"
	"runtime"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// UnsupportedPlatform is the build for operating systems the port does not
// target; every setter returns ErrUnsupported.
type UnsupportedPlatform struct {
	Runner Runner
	Events events.Events
}

// New returns the stub for this host.
func New(r Runner) Platform { return &UnsupportedPlatform{Runner: r} }

// NewWithEvents is New with a log sink.
func NewWithEvents(r Runner, ev events.Events) Platform {
	return &UnsupportedPlatform{Runner: r, Events: ev}
}

// Name implements Platform.
func (*UnsupportedPlatform) Name() string { return runtime.GOOS }

// MonitorCount implements Platform.
func (*UnsupportedPlatform) MonitorCount(context.Context) int { return 1 }

// SetWallpaper implements Platform.
func (*UnsupportedPlatform) SetWallpaper(context.Context, string) error { return ErrUnsupported }

// SetWallpaperMulti implements Platform.
func (*UnsupportedPlatform) SetWallpaperMulti(context.Context, []string) error { return ErrUnsupported }

// SetLockscreen implements Platform.
func (*UnsupportedPlatform) SetLockscreen(context.Context, string) error { return ErrUnsupported }

// LockscreenSupported implements Platform.
func (*UnsupportedPlatform) LockscreenSupported() bool { return false }
