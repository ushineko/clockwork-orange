package gui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// --- Blacklist (R7.8) ---------------------------------------------------------

// blacklistState is the section's per-visit state: the filter text and the
// rows picked for removal, kept on *ui so a rebuild does not lose them.
type blacklistState struct {
	filter   string
	selected map[string]bool
}

/*
filterBlacklist keeps the rows whose hash, date or source contains the filter,
case-insensitively (blacklist_tab.py filter_table). Order is the store's:
newest first.
*/
func filterBlacklist(items []store.BlacklistItem, filter string) []store.BlacklistItem {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return items
	}
	var out []store.BlacklistItem
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Hash), f) ||
			strings.Contains(strings.ToLower(it.Date), f) ||
			strings.Contains(strings.ToLower(it.Source), f) {
			out = append(out, it)
		}
	}
	return out
}

// buildBlacklist is the table of blocked images with their thumbnails, a live
// filter, and Remove Selected.
func (u *ui) buildBlacklist() fyne.CanvasObject {
	u.loadBlacklist()
	if u.blState.selected == nil {
		u.blState.selected = map[string]bool{}
	}
	head := widgets.Heading("Blacklist",
		"Images marked in a plugin's review are recorded here by content hash and deleted; a "+
			"plugin that downloads the same picture again deletes it at once. Remove a row to "+
			"allow the image back.")

	filter := widget.NewEntry()
	filter.SetPlaceHolder("Filter by hash, date or source plugin…")
	filter.SetText(u.blState.filter)
	filter.OnChanged = func(s string) {
		u.blState.filter = s
		u.sh.Refresh()
	}
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		u.blOK = false
		u.loadBlacklist()
	})

	rows := filterBlacklist(u.blacklist, u.blState.filter)
	t := table.New()
	t.Header("", "Thumb", "Hash", "Date added", "Source plugin")
	thumbs := make([][]byte, 0, len(rows))
	for _, it := range rows {
		mark := " "
		if u.blState.selected[it.Hash] {
			mark = "✓"
		}
		t.Row(fd.StatusInfo, mark, "", it.Hash, it.Date, it.Source)
		thumbs = append(thumbs, it.Thumbnail)
	}
	t.SetThumbnails(1, thumbs)
	t.SetWidths(28, table.ThumbCellSize+8, 470, 140, 140)
	grid := t.Widget()
	grid.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(rows) {
			h := rows[id.Row].Hash
			u.blState.selected[h] = !u.blState.selected[h]
			if !u.blState.selected[h] {
				delete(u.blState.selected, h)
			}
		}
		grid.UnselectAll()
		u.sh.Refresh()
	}

	remove := widget.NewButtonWithIcon(fmt.Sprintf("Remove selected from blacklist (%d)", len(u.blState.selected)),
		theme.DeleteIcon(), func() { u.removeSelectedBlacklist() })
	remove.Importance = widget.DangerImportance
	if len(u.blState.selected) == 0 || u.sh.Working() {
		remove.Disable()
	}
	count := widgets.Dim(fmt.Sprintf("%d of %d shown · click a row to select it", len(rows), len(u.blacklist)))
	if !u.blOK {
		count = widgets.Dim("reading…")
	}

	return container.NewBorder(
		container.NewVBox(head, container.NewBorder(nil, nil, widgets.Dim("Filter"), refresh, filter), count),
		container.NewHBox(remove), nil, nil,
		widgets.FixedHeight(grid, 480))
}

// removeSelectedBlacklist confirms and removes the picked hashes.
func (u *ui) removeSelectedBlacklist() {
	hashes := make([]string, 0, len(u.blState.selected))
	for h := range u.blState.selected {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	if len(hashes) == 0 {
		return
	}
	dialogs.ConfirmDestructive(u.sh.Window, "Remove from the blacklist?",
		fmt.Sprintf("%d image(s) will be allowed to be downloaded again. The files were deleted when "+
			"they were blacklisted and are not restored; the history is not touched.", len(hashes)),
		"Remove", func() {
			u.sh.Perform("Removing from the blacklist…", func(ctx context.Context) error {
				if err := core.BlacklistRemove(ctx, core.BlacklistRemoveRequest{Request: u.request(), Hashes: hashes}); err != nil {
					return err
				}
				fyne.Do(func() { u.blState.selected = map[string]bool{} })
				fyne.Do(func() { u.sh.OK(fmt.Sprintf("Removed %d image(s) from the blacklist.", len(hashes))) })
				return nil
			})
		})
}
