// Copied from nmsbonker (same author) — keep in sync by hand; the optional
// thumbnail column is this project's (spec 010 R7.8).

package gui

import (
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// thumbCellSize is the edge of a thumbnail cell (R7.8: 64 px).
const thumbCellSize = 64

/*
detailTable is the read-only listing this window uses everywhere the CLI prints
a fixed-width table: the detect candidates, the report rows, the installed
compilers, the archives.

It is deliberately not a widget. Fyne's table asks for its contents through
callbacks, and every section that wanted one was writing the same three closures
over a different slice of strings; this holds the strings and hands back the
table. Rows carry a Status so the verdict column is coloured from the active
scheme rather than from a hard-coded green.

Sorting is not offered here. The two tables where order is a question — the mod
build order, where order IS the conflict rule, and the report — handle it
themselves, because in one of them re-ordering the rows would misrepresent what
the build will do.
*/
type detailTable struct {
	head   []string
	rows   [][]string
	stat   []Status
	widths []float32
	// thumbCol, when >= 0, is a column drawn from thumbs (one JPEG per row)
	// instead of from the row's text. -1 (the zero value minus one) is none;
	// a table with no thumbnails leaves it at 0 and empty thumbs, which also
	// means none.
	thumbCol int
	thumbs   [][]byte
	// images caches the decoded thumbnails by row, so scrolling does not
	// decode the same JPEG again for every recycled cell.
	images map[int]fyne.Resource
}

// hasThumbs reports whether a thumbnail column is in use.
func (t *detailTable) hasThumbs() bool { return len(t.thumbs) > 0 && t.thumbCol >= 0 }

// thumb is the row's thumbnail as a resource, or nil.
func (t *detailTable) thumb(row int) fyne.Resource {
	if row < 0 || row >= len(t.thumbs) || len(t.thumbs[row]) == 0 {
		return nil
	}
	if t.images == nil {
		t.images = map[int]fyne.Resource{}
	}
	if r, ok := t.images[row]; ok {
		return r
	}
	r := fyne.NewStaticResource("thumb.jpg", t.thumbs[row])
	t.images[row] = r
	return r
}

func (t *detailTable) header(cols ...string) { t.head = cols }

// row adds a line. st colours every cell in it: these tables have one verdict
// per row rather than per cell, and colouring only the verdict column made the
// eye hunt for which row the colour belonged to.
func (t *detailTable) row(st Status, cols ...string) {
	t.rows = append(t.rows, cols)
	t.stat = append(t.stat, st)
}

// setWidths pins the column widths. Without it the measured widths are used,
// which is right for a listing of unknown content and wrong for one whose first
// column is a fixed vocabulary.
func (t *detailTable) setWidths(w ...float32) { t.widths = w }

// measure derives a column width from the widest cell, clamped. Fyne cannot ask
// a label how wide its text will be without a canvas, so this counts runes and
// multiplies — approximate on purpose, and the truncation ellipsis covers the
// cases where it is short.
func (t *detailTable) measure(col int) float32 {
	const (
		perRune = 7.0
		padding = 24.0
		minW    = 70.0
		maxW    = 460.0
	)
	widest := 0
	if col < len(t.head) {
		widest = utf8.RuneCountInString(t.head[col])
	}
	for _, r := range t.rows {
		if col < len(r) {
			if n := utf8.RuneCountInString(r[col]); n > widest {
				widest = n
			}
		}
	}
	w := float32(widest)*perRune + padding
	return min(max(w, minW), maxW)
}

// widget renders the table.
func (t *detailTable) widget() *widget.Table {
	cols := len(t.head)
	table := widget.NewTable(
		func() (int, int) { return len(t.rows), cols },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			if !t.hasThumbs() {
				return l
			}
			img := canvas.NewImageFromResource(nil)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(thumbCellSize, thumbCellSize))
			img.Hide()
			return container.NewStack(img, l)
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l, img := cellParts(o)
			if img != nil {
				if id.Col == t.thumbCol {
					img.Resource = t.thumb(id.Row)
					img.Show()
					img.Refresh()
					l.Hide()
					return
				}
				img.Hide()
				l.Show()
			}
			if id.Row < 0 || id.Row >= len(t.rows) {
				l.Importance = widget.MediumImportance
				l.SetText("")
				return
			}
			row := t.rows[id.Row]
			text := ""
			if id.Col >= 0 && id.Col < len(row) {
				text = row[id.Col]
			}
			// Importance before SetText: SetText is what refreshes the label,
			// and the refresh is where importance becomes a colour. The other
			// way round, a scrolled table paints each recycled cell in the
			// colour of the row it last held.
			l.Importance = importanceFor(t.stat[id.Row])
			l.SetText(text)
		},
	)
	table.ShowHeaderRow = true
	if t.hasThumbs() {
		table.SetRowHeight(-1, thumbCellSize+8) // the default for every row
		for i := range t.rows {
			table.SetRowHeight(i, thumbCellSize+8)
		}
	}
	table.CreateHeader = func() fyne.CanvasObject {
		l := widget.NewLabel("")
		l.TextStyle = fyne.TextStyle{Bold: true}
		l.Truncation = fyne.TextTruncateEllipsis
		return l
	}
	table.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		l := o.(*widget.Label)
		// Row headers are off in these tables, but UpdateHeader is still called
		// with Col == -1 for the corner cell. Guard it rather than indexing the
		// header slice with a negative number.
		if id.Col < 0 || id.Col >= len(t.head) {
			l.SetText("")
			return
		}
		l.SetText(t.head[id.Col])
	}
	for i := range cols {
		if i < len(t.widths) {
			table.SetColumnWidth(i, t.widths[i])
			continue
		}
		table.SetColumnWidth(i, t.measure(i))
	}
	return table
}

// cellParts splits a template cell into its label and, when the table has a
// thumbnail column, its image.
func cellParts(o fyne.CanvasObject) (*widget.Label, *canvas.Image) {
	if l, ok := o.(*widget.Label); ok {
		return l, nil
	}
	stack := o.(*fyne.Container)
	return stack.Objects[1].(*widget.Label), stack.Objects[0].(*canvas.Image)
}

// importanceFor maps a row's status onto the label importance that paints it
// from the active scheme's roles.
func importanceFor(st Status) widget.Importance {
	switch st {
	case StatusGood:
		return widget.SuccessImportance
	case StatusWarn:
		return widget.WarningImportance
	case StatusBad:
		return widget.DangerImportance
	}
	return widget.MediumImportance
}
