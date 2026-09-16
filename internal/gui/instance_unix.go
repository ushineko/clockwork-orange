//go:build unix

package gui

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
)

/*
A second launch shows the first window (R7.19).

Closing the window hides it to the tray, so the next click on the launcher,
the menu or a taskbar pin starts a second process. The Python let that second
process exit silently, which with a hidden first window meant "nothing
happens". Here the first instance listens on a Unix socket and the second
connects, writes one line and exits; the first brings its window back.
*/

// showSocketName is the socket's file name; it lives in the lock directory so
// tests isolate it the way they isolate the lock (CLOCKWORK_LOCK_DIR).
const showSocketName = "clockwork_orange_gui_show.sock"

func showSocketPath() string {
	dir := os.Getenv("CLOCKWORK_LOCK_DIR")
	if dir == "" {
		dir = os.Getenv("XDG_RUNTIME_DIR")
	}
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, showSocketName)
}

// listenShow starts the listener; the returned function stops it. Failures
// are logged to the activity pane, not fatal: the window works without it,
// the second launch just goes back to doing nothing.
func (u *ui) listenShow() func() {
	path := showSocketPath()
	_ = os.Remove(path) // a stale socket from a crashed instance
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		u.activity.events().Warnf("Second-launch listener unavailable: %v", err)
		return func() {}
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return // closed
			}
			_ = conn.Close()
			show := u.showWindow
			if u.showHook != nil {
				show = u.showHook // tests observe the request without touching widgets off-thread
			}
			fyne.Do(show)
		}
	}()
	return func() {
		_ = l.Close()
		_ = os.Remove(path)
	}
}

// requestShow asks a running instance to show its window; false when none
// answered, which is when the caller should open a window of its own.
func requestShow() bool {
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.DialContext(context.Background(), "unix", showSocketPath())
	if err != nil {
		return false
	}
	_, _ = conn.Write([]byte("show\n"))
	_ = conn.Close()
	return true
}
