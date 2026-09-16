package config

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testDebounce is the quiet window the ported spec 009 tests use.
const testDebounce = 200 * time.Millisecond

// observedChange is a channel that already carries the first token, which is
// the state the caller of DrainBurst is in.
func observedChange() chan struct{} {
	ch := make(chan struct{}, changedBuffer)
	ch <- struct{}{}
	return ch
}

func TestDrainBurstReturnsAfterTheQuietWindowForASingleSettledChange(t *testing.T) {
	// Port of test_single_change_returns_after_quiet_window: a settled change
	// returns after ~debounce, not immediately and not after the full wait.
	ch := observedChange()
	start := time.Now()
	DrainBurst(context.Background(), ch, testDebounce)
	elapsed := time.Since(start)

	require.GreaterOrEqual(t, elapsed, 180*time.Millisecond)
	require.Less(t, elapsed, 600*time.Millisecond)
	require.Empty(t, ch)
}

func TestDrainBurstDoesNotReturnMidBurst(t *testing.T) {
	// Port of test_burst_is_coalesced_into_single_return: five writes 50 ms
	// apart keep restarting the window; the function returns only after the
	// quiet window that follows the LAST write. That is what collapses a
	// login-time flurry into one wallpaper switch.
	ch := observedChange()
	var lastWrite atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 5 {
			time.Sleep(50 * time.Millisecond)
			lastWrite.Store(time.Now().UnixNano())
			ch <- struct{}{}
		}
	}()

	start := time.Now()
	DrainBurst(context.Background(), ch, testDebounce)
	elapsed := time.Since(start)
	<-done

	require.NotZero(t, lastWrite.Load())
	quietSinceLastWrite := time.Since(time.Unix(0, lastWrite.Load()))
	require.GreaterOrEqual(t, quietSinceLastWrite, 180*time.Millisecond)
	require.GreaterOrEqual(t, elapsed, 400*time.Millisecond, "burst (~250 ms) plus quiet window (~200 ms)")
	require.Empty(t, ch)
}

func TestDrainBurstReturnsPromptlyOnShutdown(t *testing.T) {
	// Port of test_returns_promptly_on_shutdown: a 30 s window would block
	// shutdown for seconds if the context were not honoured.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	DrainBurst(ctx, observedChange(), 30*time.Second)
	require.Less(t, time.Since(start), time.Second)
}

func TestDrainBurstTreatsAClosedChannelAsQuiet(t *testing.T) {
	// A closed source must not spin the loop forever on zero-value receives.
	ch := make(chan struct{})
	close(ch)
	start := time.Now()
	DrainBurst(context.Background(), ch, 30*time.Second)
	require.Less(t, time.Since(start), time.Second)
}

func TestWaitForNextCycleSleepsTheFullWaitWhenNothingChanges(t *testing.T) {
	ch := make(chan struct{}, 1)
	start := time.Now()
	interrupted := WaitForNextCycle(context.Background(), 300*time.Millisecond, ch, testDebounce)
	elapsed := time.Since(start)
	require.False(t, interrupted)
	require.GreaterOrEqual(t, elapsed, 280*time.Millisecond)
	require.Less(t, elapsed, time.Second)
}

func TestWaitForNextCycleReturnsFalseNotTrueWhenCancelledWhileDraining(t *testing.T) {
	// The Python loop returned without the "config change detected" path on
	// shutdown; a true here would start a wallpaper cycle during shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	ch := observedChange()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	interrupted := WaitForNextCycle(ctx, time.Hour, ch, 30*time.Second)
	require.False(t, interrupted)
	require.Less(t, time.Since(start), time.Second)
}

func TestWaitForNextCycleWithAClosedChannelStillHonoursTheDeadline(t *testing.T) {
	ch := make(chan struct{})
	close(ch)
	start := time.Now()
	require.False(t, WaitForNextCycle(context.Background(), 150*time.Millisecond, ch, testDebounce))
	require.GreaterOrEqual(t, time.Since(start), 130*time.Millisecond)
}

// startWatcher watches a config path under a fresh temp directory.
func startWatcher(t *testing.T) (*Watcher, string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "conf", FileName)
	ctx, cancel := context.WithCancel(context.Background())
	w, err := Watch(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		require.NoError(t, w.Close())
	})
	return w, cfg
}

func TestWatcherCoalescesFourRapidWritesIntoOneInterruption(t *testing.T) {
	// End to end against fsnotify: the login-time flurry from spec 009 must
	// interrupt the wait exactly once, ~debounce after the last write.
	w, cfg := startWatcher(t)

	var lastWrite time.Time
	for i := range 4 {
		require.NoError(t, os.WriteFile(cfg, []byte("default_wait: "+string(rune('1'+i))+"\n"), 0o600))
		lastWrite = time.Now()
		time.Sleep(10 * time.Millisecond)
	}

	interrupted := WaitForNextCycle(context.Background(), 30*time.Second, w.Changed(), testDebounce)
	since := time.Since(lastWrite)
	require.True(t, interrupted)
	require.GreaterOrEqual(t, since, 180*time.Millisecond, "must not fire before the quiet window")
	require.Less(t, since, time.Second, "must fire ~debounce after the last write, not later")

	// No second interruption: the burst was consumed whole.
	start := time.Now()
	require.False(t, WaitForNextCycle(context.Background(), 400*time.Millisecond, w.Changed(), testDebounce))
	require.GreaterOrEqual(t, time.Since(start), 380*time.Millisecond)
}

func TestWatcherSeesAnAtomicSaveButIgnoresSiblingFiles(t *testing.T) {
	// Save writes a temp file beside the config and renames it into place. The
	// temp file must not count as a change; the rename must.
	w, cfg := startWatcher(t)

	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(cfg), "other.yml"), []byte("x: 1\n"), 0o600))
	select {
	case <-w.Changed():
		t.Fatal("a sibling file must not trigger a config change")
	case <-time.After(300 * time.Millisecond):
	}

	require.NoError(t, Save(cfg, Document{DefaultWait: 7}))
	select {
	case <-w.Changed():
	case <-time.After(2 * time.Second):
		t.Fatal("an atomic replace of the config must trigger a change")
	}
}

func TestWatchCreatesAMissingParentDirectory(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "not", "yet", FileName)
	w, err := Watch(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { require.NoError(t, w.Close()) }()
	st, err := os.Stat(filepath.Dir(cfg))
	require.NoError(t, err)
	require.True(t, st.IsDir())
}

func TestWatchRefusesABareTildeParent(t *testing.T) {
	_, err := Watch(context.Background(), filepath.Join(t.TempDir(), "~", FileName))
	require.ErrorIs(t, err, ErrTildeComponent)
}

func TestWatcherCloseIsIdempotentAndReturnsAfterContextCancel(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), FileName)
	ctx, cancel := context.WithCancel(context.Background())
	w, err := Watch(ctx, cfg)
	require.NoError(t, err)
	cancel()

	done := make(chan error, 2)
	go func() { done <- w.Close() }()
	go func() { done <- w.Close() }()
	for range 2 {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Close hung after the context was cancelled")
		}
	}
}

func TestResolvePathFollowsADirectorySymlinkForAFileThatDoesNotExistYet(t *testing.T) {
	// ~/.config may itself be a symlink into a dotfiles repo; the event path
	// fsnotify reports and the configured path must still compare equal.
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	require.NoError(t, os.MkdirAll(realDir, 0o750))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(realDir, link))

	want := filepath.Join(resolvePath(realDir), FileName)
	require.Equal(t, want, resolvePath(filepath.Join(link, FileName)))
}
