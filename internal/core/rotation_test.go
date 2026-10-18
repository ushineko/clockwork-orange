package core

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/engine"
	"github.com/ushineko/clockwork-orange/internal/events"
)

func seedEngine(t *testing.T) {
	t.Helper()
	prev := engine.Rand
	engine.Rand = rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test draws
	t.Cleanup(func() { engine.Rand = prev })
}

// rotate runs one rotation over fixed sources, as the daemon would.
func rotate(t *testing.T, d *Deps, mode Mode, dir string, skip bool) {
	t.Helper()
	_, err := Rotate(context.Background(), RotateRequest{
		CycleRequest: CycleRequest{Request: Request{Deps: d}, Mode: mode, Sources: []string{dir}},
		Rotator:      RotatorDaemon,
		Skip:         skip,
	})
	require.NoError(t, err)
}

func shownDesktop(t *testing.T) []string {
	t.Helper()
	st, _, err := LoadRotation()
	require.NoError(t, err)
	out := make([]string, len(st.Desktop))
	for i, s := range st.Desktop {
		out[i] = s.Path
	}
	return out
}

func pin(t *testing.T, d *Deps, path string, monitor int) {
	t.Helper()
	require.NoError(t, RotationPin(context.Background(), PinRequest{Request: Request{Deps: d}, Path: path, Monitor: monitor}))
}

// R1: a rotation records one image per monitor, and the time an image was
// set survives a rotation that kept it.
func TestRotateRecordsWhatIsOnScreen(t *testing.T) {
	seedEngine(t)
	d, fp, _ := testDeps(t, 2)
	dir := imageDir(t, 6)

	rotate(t, d, ModeDesktop, dir, false)

	st, _, err := LoadRotation()
	require.NoError(t, err)
	require.Equal(t, RotatorDaemon, st.Rotator)
	require.Equal(t, "desktop", st.Mode)
	require.Equal(t, fp.multi[0], shownDesktop(t))
	require.False(t, st.Desktop[0].Since.IsZero())
}

// R2: concurrent writers of the control file serialise; none is lost.
func TestConcurrentControlWritersAllLand(t *testing.T) {
	testDeps(t, 1)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			require.NoError(t, updateControl(func(c *RotationControl) { c.Skip++ }))
		})
	}
	wg.Wait()
	_, ctl, err := LoadRotation()
	require.NoError(t, err)
	require.Equal(t, 20, ctl.Skip)
}

// R1/R2: a write that cannot complete leaves the previous file whole.
func TestAFailedWriteKeepsThePreviousFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test cannot write to")
	}
	testDeps(t, 1)
	require.NoError(t, updateControl(func(c *RotationControl) { c.Skip = 7 }))
	dir := config.StateDir()
	require.NoError(t, os.Chmod(dir, 0o500))       //nolint:gosec // read-only on purpose
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec

	require.Error(t, updateControl(func(c *RotationControl) { c.Skip = 8 }))
	_, ctl, err := LoadRotation()
	require.NoError(t, err)
	require.Equal(t, 7, ctl.Skip)
}

// R3/R4: a pinned monitor keeps its image while the other one rotates.
func TestAPinnedMonitorKeepsItsImage(t *testing.T) {
	seedEngine(t)
	d, _, _ := testDeps(t, 2)
	dir := imageDir(t, 6)
	rotate(t, d, ModeDesktop, dir, false)
	pinned := filepath.Join(dir, "imga.jpg")
	pin(t, d, pinned, 0)

	seen := map[string]bool{}
	for range 6 {
		rotate(t, d, ModeDesktop, dir, false)
		shown := shownDesktop(t)
		require.Equal(t, pinned, shown[0])
		require.NotEqual(t, pinned, shown[1], "an unpinned monitor avoids pinned images")
		seen[shown[1]] = true
	}
	require.Greater(t, len(seen), 1, "the unpinned monitor kept rotating")
}

// R4: pause pins what is shown, a rotation then changes nothing, resume
// clears the pins.
func TestPauseHoldsAndResumeReleases(t *testing.T) {
	seedEngine(t)
	d, fp, _ := testDeps(t, 2)
	dir := imageDir(t, 6)
	rotate(t, d, ModeDual, dir, false)
	before := shownDesktop(t)
	ctx := context.Background()

	require.NoError(t, RotationPause(ctx, Request{Deps: d}))
	status, err := RotationStatus(ctx, Request{Deps: d})
	require.NoError(t, err)
	require.True(t, status.Paused)
	require.NotEmpty(t, status.Control.Lockscreen, "dual mode pauses the lock screen too")

	calls := fp.sets() + len(fp.lock)
	rotate(t, d, ModeDual, dir, false)
	require.Equal(t, calls, fp.sets()+len(fp.lock), "a paused rotation touches nothing")
	require.Equal(t, before, shownDesktop(t))

	require.NoError(t, RotationResume(ctx, Request{Deps: d}))
	status, err = RotationStatus(ctx, Request{Deps: d})
	require.NoError(t, err)
	require.False(t, status.Paused)
	require.False(t, status.Control.Pinned())
}

// R4: skip on a paused display picks new images and stays paused on them.
func TestSkipWhilePausedMovesThePins(t *testing.T) {
	seedEngine(t)
	d, _, _ := testDeps(t, 2)
	dir := imageDir(t, 8)
	rotate(t, d, ModeDesktop, dir, false)
	ctx := context.Background()
	require.NoError(t, RotationPause(ctx, Request{Deps: d}))
	before := shownDesktop(t)

	rotate(t, d, ModeDesktop, dir, true)

	after := shownDesktop(t)
	require.NotEqual(t, before, after)
	status, err := RotationStatus(ctx, Request{Deps: d})
	require.NoError(t, err)
	require.True(t, status.Paused)
	require.Equal(t, after[0], status.Control.Desktop[0])
	require.Equal(t, after[1], status.Control.Desktop[1])
}

// R5: a pin with nothing rotating sets the image at once and keeps the
// other monitor's image.
func TestPinAppliesAtOnceAndKeepsTheOtherMonitor(t *testing.T) {
	seedEngine(t)
	d, fp, _ := testDeps(t, 2)
	dir := imageDir(t, 6)
	rotate(t, d, ModeDesktop, dir, false)
	other := shownDesktop(t)[0]
	img := filepath.Join(dir, "imgf.jpg")

	pin(t, d, img, 1)

	require.Equal(t, []string{other, img}, fp.multi[len(fp.multi)-1])
	require.Equal(t, []string{other, img}, shownDesktop(t))
}

// R4: with one monitor pinned, a skip changes only the other one.
func TestSkipLeavesAPartialPinAlone(t *testing.T) {
	seedEngine(t)
	d, _, _ := testDeps(t, 2)
	dir := imageDir(t, 8)
	rotate(t, d, ModeDesktop, dir, false)
	img := filepath.Join(dir, "imgh.jpg")
	pin(t, d, img, 0)

	for range 3 {
		rotate(t, d, ModeDesktop, dir, true)
		require.Equal(t, img, shownDesktop(t)[0])
	}
	_, ctl, err := LoadRotation()
	require.NoError(t, err)
	require.Equal(t, map[int]string{0: img}, ctl.Desktop)
}

// R4: a pin written before the loop starts holds on its first cycle; R3: a
// skip cuts the loop's wait short.
func TestTheLoopHonoursPinsAndSkips(t *testing.T) {
	seedEngine(t)
	d, fp, _ := testDeps(t, 2)
	dir := imageDir(t, 6)
	img := filepath.Join(dir, "imgc.jpg")
	pin(t, d, img, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cycles := make(chan struct{}, 4)
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, LoopRequest{
			Request: Request{Deps: d}, Mode: ModeDesktop, Sources: []string{dir}, Wait: time.Hour,
			OnCycle: func(CycleResult, error) { cycles <- struct{}{} },
		})
	}()
	waitFor(t, cycles)
	fp.mu.Lock()
	require.Equal(t, img, fp.multi[len(fp.multi)-1][0])
	fp.mu.Unlock()

	require.Eventually(t, func() bool {
		st, _, _ := LoadRotation()
		return st.NextChange != nil
	}, 5*time.Second, 20*time.Millisecond, "the loop records its next change")
	require.NoError(t, RotationSkip(ctx, Request{Deps: d}))
	waitFor(t, cycles)

	cancel()
	require.NoError(t, <-done)
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}

// R9: a pinned image that disappears is unpinned, and the monitor rotates.
func TestADeletedPinIsDropped(t *testing.T) {
	seedEngine(t)
	d, _, _ := testDeps(t, 2)
	dir := imageDir(t, 6)
	rotate(t, d, ModeDesktop, dir, false)
	img := filepath.Join(dir, "imgb.jpg")
	pin(t, d, img, 0)
	require.NoError(t, os.Remove(img))

	var logs []string
	_, err := Rotate(context.Background(), RotateRequest{
		CycleRequest: CycleRequest{
			Request: Request{Deps: d, Events: events.Events{OnLog: func(_ events.Level, m string) { logs = append(logs, m) }}},
			Mode:    ModeDesktop, Sources: []string{dir},
		},
		Rotator: RotatorDaemon,
	})
	require.NoError(t, err)
	_, ctl, err := LoadRotation()
	require.NoError(t, err)
	require.False(t, ctl.Pinned())
	require.NotEqual(t, img, shownDesktop(t)[0])
	require.Contains(t, logs, "Pinned image is gone, monitor 1 rotates again: "+img)
}

// R10: only an absolute path to an existing regular image is pinned.
func TestPinRefusesBadPaths(t *testing.T) {
	d, _, _ := testDeps(t, 1)
	dir := imageDir(t, 1)
	for _, bad := range []string{"imga.jpg", filepath.Join(dir, "missing.jpg"), dir} {
		err := RotationPin(context.Background(), PinRequest{Request: Request{Deps: d}, Path: bad, Monitor: 0})
		require.ErrorContains(t, err, bad)
	}
}
