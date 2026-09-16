package gui

import (
	"context"
	"fmt"
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// --- the plugin run dialog (R7.5) ----------------------------------------------

/*
runDialog is Download Now / Reset & Run: a progress bar driven by the plugin's
Progress events, a log pane, a live preview of the last saved image, and,
when the plugin has a string_list field, a checklist of its terms that
overrides the query for this run (plugins_tab.py PluginExecutionDialog).

Its state is on *ui rather than in the closure so that the dialog's widgets
can be driven headlessly and so one run at a time is enforced through the
same working() every other button consults.
*/
type runDialog struct {
	name     string
	override map[string]any
	// termsKey is the string_list field, "" when the plugin has none.
	termsKey string
	terms    *widget.CheckGroup

	progress *widget.ProgressBar
	status   *widget.Label
	pane     *logPane
	preview  *canvas.Image
	start    *widget.Button
	cancel   *widget.Button
	closeBtn *widget.Button
	cancelFn context.CancelFunc
	dlg      *dialog.CustomDialog
	// lastSaved is the newest ImageSaved path, for the preview.
	lastSaved string
}

// openRunDialog builds and shows the dialog. reset adds reset=true.
func (u *ui) openRunDialog(name, title string, form *pluginForm, reset bool) {
	if u.working() {
		u.flash("Something is already running. Wait for it to finish, or cancel it.", StatusWarn)
		return
	}
	d := &runDialog{name: name, override: map[string]any{"force": true}, pane: newLogPane()}
	if reset {
		d.override["reset"] = true
	}
	var checklist fyne.CanvasObject
	if !reset {
		for key, w := range form.fields {
			if st, ok := w.(*searchTerms); ok {
				d.termsKey = key
				all := make([]string, 0, len(st.terms))
				for _, t := range st.terms {
					all = append(all, t.term)
				}
				d.terms = widget.NewCheckGroup(all, nil)
				d.terms.SetSelected(st.enabledTerms())
				checklist = container.NewVBox(widget.NewLabel("Select terms to include in this run:"), d.terms)
				break
			}
		}
	}

	d.progress = widget.NewProgressBar()
	d.status = widget.NewLabel("Waiting to start…")
	d.status.Importance = widget.LowImportance
	d.preview = canvas.NewImageFromResource(nil)
	d.preview.FillMode = canvas.ImageFillContain

	d.start = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() { u.startRun(d) })
	d.start.Importance = widget.HighImportance
	d.cancel = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		if d.cancelFn != nil {
			d.cancelFn()
		}
	})
	d.cancel.Disable()
	d.closeBtn = widget.NewButton("Close", func() {
		if d.dlg != nil {
			d.dlg.Hide()
		}
		// The directory has changed under the review; scan it again, as the
		// Python did in the finally.
		if u.review != nil {
			u.review.scan()
			u.review.draw(u)
		}
	})

	pane := d.pane.widget(u, "Output", nil)
	body := container.NewVBox()
	if checklist != nil {
		body.Add(checklist)
	}
	body.Add(d.progress)
	body.Add(d.status)
	body.Add(container.NewBorder(nil, nil, nil, fixedWidth(fixedHeight(d.preview, logPaneHeight), 300), pane))
	if !u.onScreen() {
		// Headless: no dialog to show; the caller drives startRun.
		return
	}
	d.dlg = dialog.NewCustomWithoutButtons(title, body, u.win)
	d.dlg.SetButtons([]fyne.CanvasObject{d.start, d.cancel, d.closeBtn})
	d.dlg.Resize(fyne.NewSize(900, 640))
	d.dlg.Show()
	if d.terms == nil {
		// No terms to pick: start at once, as the Python did.
		u.startRun(d)
	}
}

// runEvents bridges the plugin's events into the dialog: every line to the
// pane, progress to the bar, saved images to the preview.
func (u *ui) runEvents(d *runDialog) events.Events {
	return events.Events{
		OnLog: func(level events.Level, msg string) { d.pane.log.append(level, msg) },
		OnProgress: func(pct int, msg string) {
			fyne.Do(func() {
				d.progress.SetValue(float64(pct) / 100)
				d.status.SetText(fmt.Sprintf("%d%% — %s", pct, msg))
			})
		},
		OnImageSaved: func(path string) {
			d.lastSaved = path
			img, _, err := imaging.DecodeFile(path)
			if err != nil {
				return
			}
			fyne.Do(func() { d.showPreview(img) })
		},
	}
}

func (d *runDialog) showPreview(img image.Image) {
	if d.preview == nil {
		return
	}
	d.preview.Image = img
	d.preview.Refresh()
}

// startRun runs the plugin with the dialog's overrides on a goroutine with a
// cancellable context. u.running gates every other operation meanwhile.
func (u *ui) startRun(d *runDialog) {
	if u.working() {
		u.flash("Something is already running. Wait for it to finish, or cancel it.", StatusWarn)
		return
	}
	if d.terms != nil {
		d.override[d.termsKey] = anySlice(d.terms.Selected)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelFn = cancel
	u.running = true
	d.start.Disable()
	d.closeBtn.Disable()
	d.cancel.Enable()
	d.pane.log.reset()
	d.pane.log.append(events.LevelInfo, "Starting "+d.name+"…")
	d.progress.SetValue(0)
	d.status.SetText("Running…")
	u.regate()

	req := core.RunPluginRequest{Request: u.requestWithEvents(u.runEvents(d)), Name: d.name, Override: d.override}
	finish := func(res core.PluginResultPath, err error, cancelled bool) {
		u.running = false
		d.cancelFn = nil
		d.start.Enable()
		d.closeBtn.Enable()
		d.cancel.Disable()
		switch {
		case cancelled:
			d.pane.log.append(events.LevelWarn, "Cancelled.")
			d.status.SetText("Cancelled")
		case err != nil:
			d.pane.log.append(events.LevelError, "Error: "+err.Error())
			d.status.SetText("Failed")
			u.flash(d.name+": "+err.Error(), StatusBad)
		default:
			d.pane.log.append(events.LevelInfo, "Done.")
			d.status.SetText("Done — " + orNone(res.Message, res.Path))
			d.progress.SetValue(1)
		}
		d.pane.draw()
		u.regate()
	}
	run := func() {
		stop := d.pane.pump()
		res, err := core.RunPlugin(ctx, req)
		stop()
		cancelled := ctx.Err() != nil // read before cancel() below makes it always true
		cancel()
		fyne.Do(func() { finish(core.PluginResultPath{Path: res.Path, Message: res.Message}, err, cancelled) })
	}
	if !u.onScreen() {
		run()
		return
	}
	go run()
}

// startRunHeadless builds the dialog's widgets without a window and runs the
// plugin inline. For tests; the window path is openRunDialog → startRun.
func (u *ui) startRunHeadless(d *runDialog) {
	d.progress = widget.NewProgressBar()
	d.status = widget.NewLabel("")
	d.preview = canvas.NewImageFromResource(nil)
	d.start = widget.NewButton("Start", nil)
	d.cancel = widget.NewButton("Cancel", nil)
	d.closeBtn = widget.NewButton("Close", nil)
	u.startRun(d)
}
