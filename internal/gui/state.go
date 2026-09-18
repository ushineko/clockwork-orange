package gui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	fd "github.com/ushineko/fynedesygn"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/events"
)

/*
Talking to the core from a window.

internal/core is synchronous: an operation takes a request struct, does the work
on the calling goroutine, and returns a result. A plugin run is a minute of
that. So every call runs on a goroutine of its own, through the shell's
Perform or one of the loaders below, and hops back to the UI thread with
fyne.Do; nothing here may be called from a widget handler without the `go`.

The busy indicator has to be started before the goroutine can fail, or a
failure leaves the popup up for the life of the window -- hence the deferred
done(). And the results a section renders are fields on *ui, written only on
the UI thread, so a load that finishes after the user has navigated away
updates state that the next rebuild picks up rather than a widget that is no
longer on screen.
*/

// request is the core request every operation takes, carrying the --config
// override so that this window and `clockwork-orange --config …` read one
// document, and the fakes a test supplied.
func (u *ui) request() core.Request {
	return core.Request{ConfigPath: u.configPath, Deps: u.deps}
}

// requestWithEvents is request plus a log sink, for the operations whose
// lines belong in the activity pane.
func (u *ui) requestWithEvents(ev events.Events) core.Request {
	r := u.request()
	r.Events = ev
	return r
}

// --- loaders ---------------------------------------------------------------

/*
loadConfigNow reads the document synchronously. It is the one load that does
not go through busy: the window's size, the timer's interval and every form
come out of it, and a config file is a few hundred bytes.

A missing file is not an error: the document is Defaults() and docExists is
false, which is what the Python GUI showed for a first run.
*/
func (u *ui) loadConfigNow() {
	res, err := core.LoadConfig(context.Background(), u.request())
	if err != nil {
		u.docOK = true // do not retry on every rebuild
		u.docErr = "Read the configuration: " + err.Error()
		if u.sh != nil {
			// Before the shell exists (the first read, in Run) the banner
			// waits for onStart.
			u.sh.Flash(u.docErr, fd.StatusBad)
		}
		return
	}
	u.doc, u.docPath, u.docExists, u.docOK, u.docErr = res.Doc, res.Path, res.Exists, true, ""
}

// loadService fills the service state for the status bar and the Service
// section (Linux only; elsewhere the platform layer answers without a call).
func (u *ui) loadService() {
	if u.serviceOK {
		return
	}
	// Marked loaded before the call rather than after, so a section rebuilt
	// while the first load is still running does not start a second one.
	u.serviceOK = true
	run := func() {
		res := core.ServiceStatus(context.Background(), u.request())
		fyne.Do(func() {
			previous := u.service.State
			u.service = res
			u.sh.Rebuild()
			u.announceService(previous, res.State)
		})
	}
	if !u.sh.OnScreen() {
		run()
		return
	}
	go run()
}

// pollService re-reads the service state on the 5 s tick without touching
// the loaded flag: the poll is what keeps the state fresh, not F5.
func (u *ui) pollService() {
	go func() {
		res := core.ServiceStatus(context.Background(), u.request())
		fyne.Do(func() {
			if res.State == u.service.State && res.Details == u.service.Details {
				return
			}
			previous := u.service.State
			u.service = res
			u.sh.RedrawStatus()
			if u.sh.Current().Title() == sectionService {
				u.sh.Refresh()
			}
			u.announceService(previous, res.State)
		})
	}()
}

// loadBlacklist fills the Blacklist section's table.
func (u *ui) loadBlacklist() {
	if u.blOK {
		return
	}
	u.blOK = true
	run := func() {
		items, err := core.BlacklistList(context.Background(), u.request())
		if err != nil {
			fyne.Do(func() { u.sh.Report("Read the blacklist", err) })
			return
		}
		fyne.Do(func() {
			u.blacklist = items
			u.sh.Refresh()
		})
	}
	if !u.sh.OnScreen() {
		run()
		return
	}
	go func() {
		done := u.sh.Busy("Reading the blacklist…")
		defer done()
		run()
	}()
}

// loadHistory fills the History section's statistics. Also called on the 5 s
// tick while the section is on screen (R7.7).
func (u *ui) loadHistory() {
	if u.histOK {
		return
	}
	u.histOK = true
	run := func() {
		stats, err := core.HistoryStats(context.Background(), u.request())
		if err != nil {
			fyne.Do(func() { u.sh.Report("Read the history", err) })
			return
		}
		fyne.Do(func() {
			u.history = stats
			u.sh.Refresh()
		})
	}
	if !u.sh.OnScreen() {
		run()
		return
	}
	go run()
}

// --- polling -----------------------------------------------------------------

// startPolling runs the 5 s tick: service state, history stats while History
// is on screen, and the window size for persisting (R7.2, R7.4, R7.7).
func (u *ui) startPolling() {
	ctx, cancel := context.WithCancel(context.Background())
	u.tickStop = cancel
	go func() {
		status := time.NewTicker(statusPollInterval)
		size := time.NewTicker(sizePollInterval)
		defer status.Stop()
		defer size.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-status.C:
				fyne.Do(func() {
					if u.sh.Window == nil || u.hiddenToTray {
						return
					}
					u.pollService()
					if u.sh.Current().Title() == sectionHistory {
						u.histOK = false
						u.loadHistory()
					}
				})
			case <-size.C:
				fyne.Do(func() {
					if u.sh.Window == nil || u.hiddenToTray {
						return
					}
					u.noteSize(u.sh.Window.Canvas().Size())
				})
			}
		}
	}()
}

/*
noteSize records the window's size in the document when it has changed and
schedules a save (R7.2). Fyne has no resize callback, so the size is polled;
comparing with the last persisted size makes the poll write only on a real
change, and the auto-save's 1 s coalescing turns a drag into one write.
*/
func (u *ui) noteSize(s fyne.Size) {
	if s.Width <= 0 || s.Height <= 0 || s == u.lastSize {
		return
	}
	if u.lastSize.IsZero() {
		// The first reading is the baseline, not a resize: the canvas comes
		// up a pixel or two off the requested size, and saving that on every
		// start flashed "Saved" at a user who had changed nothing.
		u.lastSize = s
		return
	}
	u.lastSize = s
	u.doc.WindowWidth, u.doc.WindowHeight = int(s.Width), int(s.Height)
	u.scheduleSave()
}

// --- auto-save -----------------------------------------------------------------

// saveDelay is how long after the last edit the document is written (R7.9):
// long enough to swallow a burst of keystrokes, short enough that the daemon
// picks up a change while the user is still looking at the section.
const saveDelay = time.Second

/*
scheduleSave arms the coalescing save timer. Every edit in every form calls
it after writing its value into u.doc, so the whole document -- Basic,
Advanced, every plugin block, the window size -- goes to disk as one write,
1 s after the last change. Unknown plugin blocks ride along untouched because
they were never taken out of u.doc (D6).
*/
func (u *ui) scheduleSave() {
	u.saveMu.Lock()
	defer u.saveMu.Unlock()
	u.saveSeq++
	seq := u.saveSeq
	if u.saveTimer != nil {
		u.saveTimer.Stop()
	}
	u.saveTimer = time.AfterFunc(saveDelay, func() {
		u.saveMu.Lock()
		stale := seq != u.saveSeq
		u.saveMu.Unlock()
		if stale {
			return
		}
		fyne.Do(u.performSave)
	})
}

// performSave writes u.doc and re-arms the wallpaper timer with the interval
// it may have just changed. Called on the UI thread.
func (u *ui) performSave() {
	path, err := core.SaveConfig(context.Background(), core.SaveConfigRequest{Request: u.request(), Doc: u.doc})
	if err != nil {
		u.sh.Flash("Save the configuration: "+err.Error(), fd.StatusBad)
		return
	}
	u.docPath, u.docExists = path, true
	if u.sh.OnScreen() {
		// Headless there is no banner to show and no timer to re-arm, and
		// touching widgets from the save timer's goroutine would race the
		// test driving them: the test driver runs fyne.Do inline.
		u.sh.Flash("Saved", fd.StatusGood)
		u.notify("Saved", "Configuration saved")
		u.timer.rearm(u)
		u.sh.RedrawStatus()
	}
	if u.saved != nil {
		u.saved()
	}
}

// flushSave writes a pending save now, for quit.
func (u *ui) flushSave() {
	u.saveMu.Lock()
	pending := u.saveTimer != nil && u.saveTimer.Stop()
	u.saveMu.Unlock()
	if pending {
		u.performSave()
	}
}

// docDefaultsFor is the document a form reads: the loaded one, whose zero
// values are filled from Defaults() so an absent key shows the default the
// daemon will use rather than an empty field.
func docDefaultsFor(doc config.Document) config.Document {
	d := config.Defaults()
	if doc.DefaultWait > 0 {
		d.DefaultWait = doc.DefaultWait
	}
	if doc.ConsoleFontFamily != "" {
		d.ConsoleFontFamily = doc.ConsoleFontFamily
	}
	if doc.ConsoleFontSize > 0 {
		d.ConsoleFontSize = doc.ConsoleFontSize
	}
	if doc.ImageExtensions != "" {
		d.ImageExtensions = doc.ImageExtensions
	}
	if doc.RestartDelay > 0 {
		d.RestartDelay = doc.RestartDelay
	}
	if doc.LogsRefreshInterval > 0 {
		d.LogsRefreshInterval = doc.LogsRefreshInterval
	}
	return d
}
