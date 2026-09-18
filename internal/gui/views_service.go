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
	toolbar := container.NewHBox(start, stop, restart, install, uninstall)

	return container.NewVBox(
		head,
		widgets.Card("Status", status, detailsPane),
		widgets.Card("Control", toolbar),
		u.journalPane(),
	)
}

/*
journalPane is the service log: journalctl's tail in the activity pane, with
Refresh now and the auto-refresh toggle and interval bound to auto_update_logs
and logs_refresh_interval in the document (R7.4). The pane keeps its scroll
position across refreshes unless it is following the tail.
*/
func (u *ui) journalPane() fyne.CanvasObject {
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
	controls := container.NewHBox(auto, widgets.Dim("every"), widgets.FixedWidth(interval, 60), widgets.Dim("s"), refresh)
	pane := u.activity.Widget(u.paneOptions("Service log", nil))
	if u.activity.Model().Len() == 0 {
		u.refreshJournal()
	}
	u.armJournalRefresh()
	return container.NewVBox(controls, pane)
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
	return container.NewVBox(head, u.activity.Widget(u.paneOptions("Application activity", func() {
		paneEvents(u.activity).Infof("Activity log cleared by user")
	})))
}
