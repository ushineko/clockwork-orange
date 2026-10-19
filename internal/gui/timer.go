package gui

import (
	"context"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/platform"
)

/*
wallpaperTimer is the in-process rotation (R7.12): the first change 2 s after
start, then every default_wait seconds, skipped while a change is already in
progress. The change is the same core.Rotate the daemon runs (DV2), so the GUI
and the daemon select images by one rule and honour the same pins.

DV10: when the daemon holds the cycling lock, the timer stays idle and the
status bar says so, instead of both of them changing the wallpaper every
period, as 2.9.x did when a user ran the GUI beside the service.

It follows the desktop's own wallpaper changes while it is the rotator (spec
028 R4), and watches rotation-control.json as the daemon does (spec 026 R3): a skip
fires at once and restarts the interval, and a resume after a full pause
restarts it too. While it is the rotator it records the time of its next
change for the Wallpaper card.
*/
type wallpaperTimer struct {
	mu      sync.Mutex
	stop    chan struct{}
	running bool
	// daemonHeld records the last check of the daemon lock, for the status bar.
	daemonHeld bool
	// fired counts cycles started, for tests.
	fired int
}

// firstFireDelay is how long after start the first change happens.
const firstFireDelay = 2 * time.Second

// start arms the timer.
func (t *wallpaperTimer) start(u *ui) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stop != nil {
		close(t.stop)
	}
	t.stop = make(chan struct{})
	stop := t.stop
	interval := time.Duration(waitSeconds(u.doc)) * time.Second
	mode := core.ResolveMode(u.doc, false, false)
	paneEvents(u.activity).Infof("Wallpaper timer: every %d seconds", waitSeconds(u.doc))
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-stop
			cancel()
		}()
		control, closeControl, err := core.WatchRotationControl(ctx)
		if err != nil {
			paneEvents(u.activity).Warnf("Pause and skip from elsewhere will not reach the timer: %v", err)
		}
		defer closeControl()
		_, ctl, _ := core.LoadRotation()
		lastSkip := ctl.Skip
		desktop := core.WatchDesktop(ctx, u.request())
		paused := core.RotationPaused(ctx, u.request(), mode)

		next := time.NewTimer(firstFireDelay)
		defer next.Stop()
		reset := func(d time.Duration) {
			if !next.Stop() {
				select {
				case <-next.C:
				default:
				}
			}
			next.Reset(d)
			t.recordNext(u, mode, time.Now().Add(d), paused)
		}
		t.recordNext(u, mode, time.Now().Add(firstFireDelay), paused)
		for {
			select {
			case <-stop:
				return
			case <-next.C:
				if !paused {
					t.fire(u, false)
				}
				reset(interval)
			case img, ok := <-desktop:
				if !ok {
					desktop = nil
					continue
				}
				// The daemon follows the desktop itself while it rotates.
				if !core.DaemonRunning() {
					if err := core.NoteDesktopWallpaper(img, paneEvents(u.activity)); err != nil {
						paneEvents(u.activity).Warnf("%v", err)
					}
				}
			case _, ok := <-control:
				if !ok {
					control = nil
					continue
				}
				_, ctl, err := core.LoadRotation()
				if err != nil {
					continue
				}
				nowPaused := core.RotationPaused(ctx, u.request(), mode)
				switch {
				case ctl.Skip != lastSkip:
					lastSkip = ctl.Skip
					paused = nowPaused
					t.fire(u, true)
					reset(interval)
				case paused && !nowPaused:
					paused = false
					reset(interval)
				case nowPaused != paused:
					paused = nowPaused
					t.recordNext(u, mode, time.Time{}, paused)
				}
			}
		}
	}()
}

/*
recordNext writes the next change into rotation.json while this window is
the rotator; with the daemon cycling, the file is the daemon's to write.

It then reloads the card itself (spec 029 R1). Every change, skip and pause
the timer makes ends here, and the card is in this process: waiting for the
file watcher to report a write this window made is what left the card a
change behind on macOS.
*/
func (t *wallpaperTimer) recordNext(u *ui, mode core.Mode, at time.Time, paused bool) {
	defer fyne.Do(u.rotationChanged)
	if core.DaemonRunning() {
		return
	}
	var next *time.Time
	if !paused && !at.IsZero() {
		next = &at
	}
	if err := core.SetNextChange(core.RotatorGUI, mode, next); err != nil {
		paneEvents(u.activity).Warnf("Could not record the next change: %v", err)
	}
}

// rearm restarts the timer with the interval the document now carries. Called
// after every save, because Settings may have changed default_wait.
func (t *wallpaperTimer) rearm(u *ui) {
	t.halt()
	t.start(u)
}

// halt stops the timer.
func (t *wallpaperTimer) halt() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stop != nil {
		close(t.stop)
		t.stop = nil
	}
}

/*
fire runs one rotation, unless one is already running or the daemon holds the
lock. Its log lines stream into the activity pane; the result is not flashed,
because a banner every five minutes on a window that is usually in the tray
is noise, and the pane says what happened.
*/
func (t *wallpaperTimer) fire(u *ui, skip bool) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.fired++
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	held := core.DaemonRunning()
	fyne.Do(func() {
		if held != t.daemonHeld {
			t.daemonHeld = held
			u.sh.RedrawStatus()
		}
	})
	ev := paneEvents(u.activity)
	if held {
		ev.Infof("The service holds the cycling lock; the window's timer is idle")
		fyne.Do(u.activity.Draw)
		return
	}
	ev.Infof("=== Wallpaper Change Cycle ===")
	stop := u.activity.Pump()
	defer stop()
	mode := core.ResolveMode(u.doc, false, false)
	_, err := core.Rotate(context.Background(), core.RotateRequest{
		CycleRequest: core.CycleRequest{Request: u.requestWithEvents(ev), Mode: mode},
		Rotator:      core.RotatorGUI,
		Skip:         skip,
	})
	if err != nil {
		ev.Errorf("%v", err)
		return
	}
	ev.Infof("Wallpaper changed")
}

// --- tray and lifecycle (R7.11) ---------------------------------------------

// hasTray reports whether the platform offers a system tray. Fyne's desktop
// driver does; the test driver does not.
func hasTray(a fyne.App) bool {
	_, ok := a.(desktop.App)
	return ok
}

// setupTray installs the tray icon and its menu: Show, About, Quit.
func (u *ui) setupTray() {
	d, ok := u.sh.App.(desktop.App)
	if !ok {
		return
	}
	menu := fyne.NewMenu("Clockwork Orange",
		fyne.NewMenuItem("Show", u.showWindow),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("About", func() { u.showWindow(); u.sh.Select(sectionAbout) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", u.quit),
	)
	d.SetSystemTrayMenu(menu)
	d.SetSystemTrayIcon(appIcon())
}

// showWindow brings the window back from the tray, reloading the card when a
// change arrived while it was hidden (spec 029 R2).
func (u *ui) showWindow() {
	u.hiddenToTray = false
	if !u.rotationOK {
		u.rotationChanged()
	}
	if u.sh.Window != nil {
		u.sh.Window.Show()
		u.sh.Window.RequestFocus()
	}
}

/*
onClose is the window's close intercept: hide to the tray and say so, the way
closeEvent did, so the timer keeps changing wallpapers. Without a tray the
window quits instead: hidden with no way back, it would be a process the user
cannot see and cannot stop.
*/
func (u *ui) onClose() {
	if !hasTray(u.sh.App) {
		u.quit()
		return
	}
	u.hiddenToTray = true
	u.sh.Window.Hide()
	u.notify("Clockwork Orange", "Minimized to tray. Wallpaper changes continue in the background.")
}

// quit stops the timers, writes a pending save, releases the single-instance
// lock and exits.
func (u *ui) quit() {
	u.shutdown()
	if u.sh.App != nil {
		u.sh.App.Quit()
	}
}

// shutdown is quit without the exit, for the path where the window closed
// itself (no tray) and ShowAndRun returned.
func (u *ui) shutdown() {
	u.timer.halt()
	if u.stopRotation != nil {
		u.stopRotation()
		u.stopRotation = nil
	}
	if u.tickStop != nil {
		u.tickStop()
		u.tickStop = nil
	}
	if u.review != nil {
		u.review.detach()
	}
	u.flushSave()
	if u.stopListen != nil {
		u.stopListen()
		u.stopListen = nil
	}
	if u.stopPprof != nil {
		u.stopPprof()
		u.stopPprof = nil
	}
	if u.release != nil {
		u.release()
		u.release = nil
	}
}

// announceService sends the "Service running" / "Service stopped"
// notifications on a state change (R7.11); not on the first read.
func (u *ui) announceService(previous, current platform.ServiceState) {
	if previous == "" || previous == current {
		return
	}
	switch current {
	case platform.StateActive:
		u.notify("Clockwork Orange", "Service is running")
	case platform.StateInactive, platform.StateFailed:
		u.notify("Clockwork Orange", "Service stopped")
	case platform.StateActivating, platform.StateDeactivating, platform.StateUnknown:
	}
}
