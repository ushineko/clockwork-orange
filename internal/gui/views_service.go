package gui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// --- Service (Linux, R7.4) ----------------------------------------------------

/*
buildService is the systemd user unit: its state, systemctl's details, the
five controls with service_manager.py's enablement matrix, and the journal
tail in the activity pane.
*/
func (u *ui) buildService() fyne.CanvasObject {
	u.loadService()
	head := widgets.Heading("Service",
		"The background service changes the wallpaper on the interval in Settings, using the "+
			"enabled plugins. It runs as a systemd user unit; the window's own timer stays idle "+
			"while the service holds the cycling lock.")

	st := u.service.State
	status := container.NewHBox(widgets.Marker(serviceStatus(st)),
		widget.NewLabelWithStyle(serviceStateText(st), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	if !u.serviceOK {
		status = container.NewHBox(widgets.Marker(fd.StatusInfo), widget.NewLabel("Checking status…"))
	}

	details := widget.NewLabel(widgets.OrNone(u.service.Details, "No details."))
	details.TextStyle = fyne.TextStyle{Monospace: true}
	details.Wrapping = fyne.TextWrapOff
	detailsPane := widgets.FixedHeight(container.NewScroll(details), 150)

	en := serviceEnablement(st)
	verb := func(label string, icon fyne.Resource, enabled bool, what string, op func(context.Context, core.Request) error, done string) *widget.Button {
		b := widget.NewButtonWithIcon(label, icon, func() {
			u.sh.Perform(what, func(ctx context.Context) error {
				if err := op(ctx, u.request()); err != nil {
					return err
				}
				fyne.Do(func() { u.serviceOK = false; u.loadService() })
				fyne.Do(func() { u.sh.OK(done) })
				return nil
			})
		})
		if !enabled || u.sh.Working() {
			b.Disable()
		}
		return b
	}
	start := verb("Start", theme.MediaPlayIcon(), en.start, "Starting the service…", core.ServiceStart, "Service started.")
	stop := verb("Stop", theme.MediaStopIcon(), en.stop, "Stopping the service…", core.ServiceStop, "Service stopped.")
	restart := verb("Restart", theme.ViewRefreshIcon(), en.restart, "Restarting the service…", core.ServiceRestart, "Service restarted.")
	install := verb("Install", theme.DownloadIcon(), en.install, "Installing the service…", core.ServiceInstall,
		"Service installed and enabled. It starts at login and runs now.")
	uninstall := widget.NewButtonWithIcon("Uninstall", theme.DeleteIcon(), func() {
		dialogs.ConfirmDestructive(u.sh.Window, "Uninstall the service?",
			"The systemd user unit is stopped, disabled and its file removed. Your configuration, "+
				"downloaded images, history and blacklist are not touched; the window's own timer "+
				"keeps changing wallpapers while it is open.", "Uninstall", func() {
				u.sh.Perform("Uninstalling the service…", func(ctx context.Context) error {
					if err := core.ServiceUninstall(ctx, u.request()); err != nil {
						return err
					}
					fyne.Do(func() { u.serviceOK = false; u.loadService() })
					fyne.Do(func() { u.sh.OK("Service uninstalled.") })
					return nil
				})
			})
	})
	uninstall.Importance = widget.DangerImportance
	if !en.uninstall || u.sh.Working() {
		uninstall.Disable()
	}
	// The two controls whose consequence the label cannot fit (spec 013 R6.1).
	// The other three do what they say.
	toolbar := container.NewHBox(start, stop, restart,
		widgets.WithTip(install, "Writes the systemd user unit, enables it so it starts at login, and starts it now. "+
			"The window's own timer stays idle while the service is cycling."),
		widgets.WithTip(uninstall, "Stops and disables the unit and removes its file. Your configuration, images, "+
			"history and blacklist are not touched."))

	// Under a divider the user can drag (spec 013 R4.1). A fixed region is
	// wrong for the pane that matters: the run worth reading is whichever one
	// went wrong, and the position is the shell's, so it survives this
	// section's rebuild on every 5 s status poll.
	//
	// The controls are affixed above the divider and the status scrolls behind
	// them (spec 014). The five verbs are the whole point of this section, and
	// when they sat in the scrolling half under a heading, a status line and a
	// 150 px details pane, the default divider position put them off the
	// bottom of it: a section reporting that the service is running, with no
	// way to stop it in sight.
	controls := container.NewVBox(
		widgets.Card("Control", toolbar),
		u.journalControls(),
	)
	return u.sh.VSplit(logSplitKey, logSplitOffset,
		container.NewBorder(nil, controls, nil, nil,
			container.NewVScroll(container.NewVBox(
				head,
				widgets.Card("Status", status, detailsPane),
			))),
		u.journalPaneWidget("Service log", nil),
	)
}

// logSplitKey is the divider over the log pane. Service and Activity share it
// on purpose: it is one pane as far as the user is concerned, and the first
// section differs only by platform (spec 013 R4.2).
const logSplitKey = "log"

// logSplitOffset is where the divider sits before anyone has dragged it:
// roughly the proportion the pane's fixed height used to give it.
const logSplitOffset = 0.55

/*
journalControls is the service log's controls: Refresh now and the auto-refresh
toggle and interval, bound to auto_update_logs and logs_refresh_interval in the
document (R7.4).

They sit above the divider with the rest of the section rather than with the
pane: the bottom half of a split is the pane and nothing else, so dragging the
bar all the way down leaves a pane and not a pane with a strip on top of it.
*/
func (u *ui) journalControls() fyne.CanvasObject {
	refresh := widget.NewButtonWithIcon("Refresh now", theme.ViewRefreshIcon(), func() { u.refreshJournal() })
	// SetChecked fires OnChanged, so the handler is attached after the
	// initial value is in place: a section build must not schedule a save.
	auto := widget.NewCheck("Auto-refresh", nil)
	auto.SetChecked(u.doc.AutoUpdateLogs)
	auto.OnChanged = func(b bool) {
		u.doc.AutoUpdateLogs = b
		u.scheduleSave()
		u.armJournalRefresh()
	}
	interval := widget.NewEntry()
	interval.SetText(strconv.Itoa(docDefaultsFor(u.doc).LogsRefreshInterval))
	interval.Validator = intRange(1, 300)
	interval.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 300 {
			u.doc.LogsRefreshInterval = n
			u.scheduleSave()
			u.armJournalRefresh()
		}
	}
	if u.activity.Model().Len() == 0 {
		u.refreshJournal()
	}
	u.armJournalRefresh()
	return container.NewHBox(auto, widgets.Dim("every"), widgets.FixedWidth(interval, 60), widgets.Dim("s"), refresh)
}

// journalPaneWidget is the log pane itself, the bottom half of the section's
// split. Options.Height is the pane's minimum, not its size: it takes whatever
// the divider gives it.
func (u *ui) journalPaneWidget(title string, onClear func()) fyne.CanvasObject {
	return u.activity.Widget(u.paneOptions(title, onClear))
}

// refreshJournal re-reads the last 50 journal lines into the pane.
func (u *ui) refreshJournal() {
	run := func() {
		text := core.ServiceLogs(context.Background(), core.ServiceLogsRequest{Request: u.request(), Lines: 50})
		fyne.Do(func() {
			u.activity.Model().Replace(widgets.OrNone(text, "No logs found."))
			u.activity.Touch()
			u.activity.Draw()
		})
	}
	if !u.sh.OnScreen() {
		run()
		return
	}
	go run()
}

// armJournalRefresh starts or stops the auto-refresh ticker per the document.
func (u *ui) armJournalRefresh() {
	if u.journalStop != nil {
		u.journalStop()
		u.journalStop = nil
	}
	if !u.doc.AutoUpdateLogs || !u.sh.OnScreen() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.journalStop = cancel
	every := docDefaultsFor(u.doc).LogsRefreshInterval
	go func() {
		t := timeTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fyne.Do(func() {
					if u.sh.Current().Title() == sectionService && !u.hiddenToTray {
						u.refreshJournal()
					}
				})
			}
		}
	}()
}

// timeTicker is a ticker every n seconds.
func timeTicker(seconds int) *time.Ticker {
	return time.NewTicker(time.Duration(seconds) * time.Second)
}

// intRange validates a numeric field inside [lo, hi].
func intRange(lo, hi int) fyne.StringValidator {
	return func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("enter a whole number")
		}
		if n < lo || n > hi {
			return fmt.Errorf("enter a number from %d to %d", lo, hi)
		}
		return nil
	}
}

// --- Activity (Windows and macOS, R7.4) --------------------------------------

// buildActivity is the in-process log where there is no service to manage: the
// wallpaper timer's cycles and plugin runs, with a Clear button.
func (u *ui) buildActivity() fyne.CanvasObject {
	head := widgets.Heading("Activity",
		"What this window has done since it opened: every wallpaper change the timer made and "+
			"every plugin run. The timer fires on the interval in Settings while the window is "+
			"open or in the tray.")
	return u.sh.VSplit(logSplitKey, logSplitOffset,
		container.NewVScroll(container.NewVBox(head)),
		u.journalPaneWidget("Application activity", func() {
			paneEvents(u.activity).Infof("Activity log cleared by user")
		}),
	)
}
