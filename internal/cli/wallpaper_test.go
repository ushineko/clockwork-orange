package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// `wallpaper status` names each target, marks the pinned ones, and says
// paused instead of a time while everything is held.
func TestWallpaperStatusPrintsTargetsPinsAndTheNextChange(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.Local)
	next := now.Add(90 * time.Second)
	r := core.RotationStatusResult{
		Running: true,
		State: core.RotationState{
			Rotator: core.RotatorDaemon, Mode: "dual", NextChange: &next,
			Desktop:    []core.Shown{{Path: "/p/a.jpg", Since: now.Add(-time.Minute)}, {Path: "/p/b.jpg"}},
			Lockscreen: &core.Shown{Path: "/p/c.jpg"},
		},
		Control: core.RotationControl{Desktop: map[int]string{1: "/p/b.jpg"}},
	}
	var out bytes.Buffer
	printRotation(&out, r, now)
	require.Equal(t, `Rotator:     daemon (running)
Mode:        dual
Monitor 1:   /p/a.jpg (for 1m0s)
Monitor 2:   /p/b.jpg [pinned]
Lock screen: /p/c.jpg
Next change: 12:01:30 (in 1m30s)
`, out.String())

	r.Paused = true
	out.Reset()
	printRotation(&out, r, now)
	require.Contains(t, out.String(), "Next change: paused")
}
