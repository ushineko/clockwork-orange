package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DebounceSeconds is the quiet window the daemon waits for after a config
// change before it acts on it (spec 009, R2.6).
const DebounceSeconds = 2.0

// changedBuffer bounds the tokens Watcher keeps for a consumer that is busy
// elsewhere. A full buffer already says "something changed", so further tokens
// carry no information and are dropped rather than stalling the event loop.
const changedBuffer = 16

/*
Watcher reports changes to one config file (R2.6).

It watches the file's parent directory, not the file: editors and this
package's own Save replace the file by rename, which would orphan a watch on
the inode. Every Create/Write/Rename/Chmod event whose resolved path equals
the resolved config path becomes one token on Changed(). fsnotify reports a
rename-onto-the-config as a Create of the config path, which is how the "new
name" of an atomic replace is matched.
*/
type Watcher struct {
	fs       *fsnotify.Watcher
	changed  chan struct{}
	target   string
	done     chan struct{}
	closeMu  sync.Once
	closeErr error
}

// Watch starts watching path's parent directory, creating it if absent, until
// ctx is done or Close is called.
func Watch(ctx context.Context, path string) (*Watcher, error) {
	dir := filepath.Dir(path)
	if err := MkdirAll(dir); err != nil {
		return nil, err
	}
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create config watcher: %w", err)
	}
	if err := fs.Add(dir); err != nil {
		_ = fs.Close()
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	w := &Watcher{
		fs:      fs,
		changed: make(chan struct{}, changedBuffer),
		target:  resolvePath(path),
		done:    make(chan struct{}),
	}
	go w.run(ctx)
	return w, nil
}

// Changed delivers one token per raw event that matched the config path. The
// channel is never closed; consumers stop through their own context.
func (w *Watcher) Changed() <-chan struct{} { return w.changed }

// Close stops the watcher. Safe to call more than once and after ctx ended.
func (w *Watcher) Close() error {
	w.closeMu.Do(func() {
		if err := w.fs.Close(); err != nil {
			w.closeErr = fmt.Errorf("close config watcher: %w", err)
		}
		<-w.done
	})
	return w.closeErr
}

func (w *Watcher) run(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case <-ctx.Done():
			// Closing here also ends the Events channel, but the loop must not
			// wait for that: Close() blocks on done.
			_ = w.fs.Close()
			w.drainUntilClosed()
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if w.matches(ev) {
				select {
				case w.changed <- struct{}{}:
				default:
				}
			}
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "[config-watcher] %v\n", err)
		}
	}
}

// drainUntilClosed consumes whatever fsnotify still has queued after Close so
// its goroutines can finish.
func (w *Watcher) drainUntilClosed() {
	for {
		select {
		case _, ok := <-w.fs.Events:
			if !ok {
				return
			}
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
		}
	}
}

// matches applies the ConfigWatcher rule: Create, Write, Rename and Chmod
// events on the config path itself. Remove is not a change the daemon acts on.
func (w *Watcher) matches(ev fsnotify.Event) bool {
	if !ev.Has(fsnotify.Create) && !ev.Has(fsnotify.Write) &&
		!ev.Has(fsnotify.Rename) && !ev.Has(fsnotify.Chmod) {
		return false
	}
	return resolvePath(ev.Name) == w.target
}

// resolvePath is Python's Path.resolve() for a file that may not exist yet:
// symlinks are followed on the whole path when possible, otherwise on the
// parent, and the result is absolute and clean.
func resolvePath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	dir, base := filepath.Split(p)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(resolved, base)
	}
	return filepath.Clean(p)
}

// DrainBurst coalesces a burst of rapid config writes into one trigger (spec
// 009 `_drain_config_change_burst`). Call it right after the first token was
// observed on changed. Any token that arrives inside the quiet window restarts
// the window; the function returns once the file has been quiet for debounce,
// or promptly when ctx is done. A closed channel counts as quiet.
func DrainBurst(ctx context.Context, changed <-chan struct{}, debounce time.Duration) {
	// Python cleared the event on entry: tokens already queued belong to the
	// burst the caller just observed.
	drainPending(changed)
	timer := time.NewTimer(debounce)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-changed:
			if !ok {
				return
			}
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(debounce)
		case <-timer.C:
			return
		}
	}
}

func drainPending(changed <-chan struct{}) {
	for {
		select {
		case _, ok := <-changed:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// waitTick is the granularity at which WaitForNextCycle re-checks its
// deadline, matching the Python loop's `change_event.wait(timeout=1.0)`.
const waitTick = time.Second

// WaitForNextCycle sleeps for wait, in 1 s ticks, until a config change or
// ctx done (spec 009 `_wait_for_next_cycle`). On the first token it runs
// DrainBurst and reports true so the caller cycles once for the whole burst.
// It reports false when wait elapses or ctx ends, including a ctx that ended
// while draining.
func WaitForNextCycle(ctx context.Context, wait time.Duration, changed <-chan struct{}, debounce time.Duration) (interrupted bool) {
	deadline := time.Now().Add(wait)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(min(remaining, waitTick))
		select {
		case <-ctx.Done():
			return false
		case _, ok := <-changed:
			if !ok {
				// The source is gone; keep sleeping on the timer alone.
				changed = nil
				continue
			}
			DrainBurst(ctx, changed, debounce)
			return ctx.Err() == nil
		case <-timer.C:
		}
	}
}
