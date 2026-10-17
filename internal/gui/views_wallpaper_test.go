package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/platform"
)

// writeRotation puts a rotation.json and rotation-control.json in place, as a
// daemon would have left them.
func writeRotation(t *testing.T, st core.RotationState, ctl core.RotationControl) {
	t.Helper()
	require.NoError(t, os.MkdirAll(config.StateDir(), 0o700))
	for path, v := range map[string]any{core.RotationPath(): st, core.RotationControlPath(): ctl} {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, b, 0o600))
	}
}

// R7: the card shows each monitor and the countdown from what the daemon
// recorded; Pause becomes Resume once everything is pinned; and the section
// keeps its size across the two.
func TestTheServiceSectionShowsTheWallpaperAndHoldsItsSize(t *testing.T) {
	u, _, _ := testUI(t)
	// The daemon is alive when its cycling lock is held.
	lock, ok := platform.TryLock("clockwork_orange_service_lock")
	require.True(t, ok)
	t.Cleanup(lock.Release)

	now := time.Now()
	next := now.Add(5 * time.Minute)
	st := core.RotationState{
		Rotator: core.RotatorDaemon, Mode: "desktop", NextChange: &next,
		Desktop: []core.Shown{{Path: "/pics/a.jpg", Since: now}, {Path: "/pics/b.jpg", Since: now}},
	}
	writeRotation(t, st, core.RotationControl{})

	running := u.buildService()
	text := strings.Join(fynetest.Texts(running), "\n")
	require.Contains(t, text, "Monitor 1")
	require.Contains(t, text, "Monitor 2")
	require.Contains(t, text, "b.jpg")
	require.Regexp(t, `Next change in [45]:\d\d`, text)
	require.NotNil(t, findButton(running, "Pause"))
	runningSize := running.MinSize()

	st.NextChange = nil
	writeRotation(t, st, core.RotationControl{Desktop: map[int]string{0: "/pics/a.jpg", 1: "/pics/b.jpg"}})
	u.rotationOK = false
	paused := u.buildService()
	text = strings.Join(fynetest.Texts(paused), "\n")
	require.Contains(t, text, "Paused")
	require.Contains(t, text, "pinned")
	require.NotNil(t, findButton(paused, "Resume"))
	require.Equal(t, runningSize, paused.MinSize(), "pausing reflows nothing")
}

// R8: the Pin menu lists each monitor the desktop has, all of them and the
// lock screen; choosing a monitor shows the image there and records the pin.
func TestThePinMenuPinsTheReviewedImage(t *testing.T) {
	u, fp, _ := testUI(t)
	fp.monitors = 2
	img := filepath.Join(t.TempDir(), "pick.jpg")
	require.NoError(t, os.WriteFile(img, []byte("x"), 0o600))

	labels := func() []string {
		var out []string
		for _, it := range u.pinMenuItems(img, core.RotationTargets(t.Context(), u.request())) {
			if !it.IsSeparator {
				out = append(out, it.Label)
			}
		}
		return out
	}
	require.Equal(t, []string{"Monitor 1", "Monitor 2", "All monitors"}, labels(),
		"desktop mode offers no lock screen")
	doc := config.Defaults()
	doc.Lockscreen = true
	require.NoError(t, config.Save(config.DefaultPath(), doc))
	require.Equal(t, []string{"Lock screen"}, labels(), "lock screen mode offers no monitor")
	doc.DualWallpapers = true
	require.NoError(t, config.Save(config.DefaultPath(), doc))
	require.Equal(t, []string{"Monitor 1", "Monitor 2", "All monitors", "Lock screen"}, labels())

	items := u.pinMenuItems(img, core.RotationTargets(t.Context(), u.request()))

	items[1].Action()

	_, ctl, err := core.LoadRotation()
	require.NoError(t, err)
	require.Equal(t, map[int]string{1: img}, ctl.Desktop)
	require.Contains(t, fp.set, img)
}
