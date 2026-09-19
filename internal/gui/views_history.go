package gui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/clockwork-orange/internal/core"
)

// --- History (R7.7) -----------------------------------------------------------

// buildHistory is the download history: three statistics, refreshed on show
// and every 5 s, and the two operations on it.
func (u *ui) buildHistory() fyne.CanvasObject {
	u.loadHistory()
	head := widgets.Heading("History",
		"Every image a plugin has downloaded, by URL and by content, so the same picture is not "+
			"fetched twice even after you delete the file.")

	stats := widget.NewForm(
		widget.NewFormItem("Total downloads tracked", widget.NewLabel(fmt.Sprintf("%d", u.history.TotalRecords))),
		widget.NewFormItem("Unique images", widget.NewLabel(fmt.Sprintf("%d", u.history.UniqueImages))),
		widget.NewFormItem("Database size", widget.NewLabel(widgets.HumanSize(u.history.DBSizeBytes))),
	)
	if !u.histOK {
		stats = widget.NewForm(widget.NewFormItem("Statistics", widget.NewLabel("reading…")))
	}

	reset := widget.NewButtonWithIcon("Reset history database", theme.DeleteIcon(), func() {
		dialogs.ConfirmDestructive(u.sh.Window, "Clear the download history?",
			"Every record is deleted and the database compacted. Images already on disk are not "+
				"touched, and the blacklist is not touched; plugins may download previously seen "+
				"images again. This cannot be undone.", "Clear history", func() {
				u.sh.Perform("Clearing the history…", func(ctx context.Context) error {
					if err := core.HistoryClear(ctx, u.request()); err != nil {
						return err
					}
					fyne.Do(func() { u.sh.OK("History database cleared.") })
					return nil
				})
			})
	})
	reset.Importance = widget.DangerImportance

	scan := widget.NewButtonWithIcon("Scan & import existing files", theme.FolderOpenIcon(), func() {
		u.sh.PerformCancellable("Scanning existing images…", func(ctx context.Context) error {
			res, err := core.HistoryImport(ctx, core.HistoryImportRequest{Request: u.request()})
			if err != nil {
				return err
			}
			fyne.Do(func() {
				u.sh.OK(fmt.Sprintf("Imported %d image(s) from %s; skipped %d duplicate(s).", res.Imported, res.Dir, res.Skipped))
			})
			return nil
		})
	})
	u.sh.Gate(reset, scan)
	// What each button costs, for the pointer that hovers before it commits
	// (spec 013 R6.1). The notes below the buttons say the same thing to
	// whoever reads the section rather than hovers over it.
	resetTip := widgets.WithTip(reset, "Deletes every record and compacts the database. Images on disk and the "+
		"blacklist are untouched, but plugins may download previously seen images again. Cannot be undone.")
	scanTip := widgets.WithTip(scan, "Records every *.jpg already in the DuckDuckGo Images download directory as "+
		"downloaded, so a fresh history does not fetch them all again. Cancellable.")

	return container.NewVBox(
		head,
		widgets.Card("Database statistics", stats),
		widgets.Card("Actions",
			container.NewHBox(resetTip, scanTip),
			widgets.Note("Resetting the history lets previously deleted images be downloaded again. "+
				"Scanning records every *.jpg in the DuckDuckGo Images download directory as "+
				"already downloaded.", fd.StatusInfo)),
	)
}
