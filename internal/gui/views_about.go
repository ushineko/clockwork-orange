// Copied from nmsbonker (same author) — keep in sync by hand; the text is this
// project's.

package gui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/gui/assets"
)

// --- About (R7.10) ------------------------------------------------------------

// projectURL is where the README lives. It is the one place this window sends a
// user outside itself, and it opens in the desktop's browser rather than in any
// view of ours.
const projectURL = "https://github.com/ushineko/clockwork-orange"

// tagline is the About dialog's motto, from main_window.py.
const tagline = "My choice is your imperative"

/*
buildAbout is the logo, the name, the version and the README (R7.10). The
README is rendered from an embedded copy with Fyne's Markdown widget; the Qt
heading-anchor hack is not ported (DV5), so the table of contents is plain
text.
*/
func (u *ui) buildAbout() fyne.CanvasObject {
	logo := canvas.NewImageFromResource(appIcon())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(72, 72))

	name := widget.NewLabelWithStyle("Clockwork Orange", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	motto := widget.NewLabel("“" + tagline + "”")
	motto.Importance = widget.LowImportance
	ver := widget.NewLabel(u.version + "  •  © 2025-2026 github.com/ushineko")
	ver.Importance = widget.LowImportance

	head := container.NewBorder(nil, nil, container.NewPadded(logo), nil,
		container.NewVBox(name, motto, ver, aboutLink()))

	readme := widget.NewRichTextFromMarkdown(string(assets.README()))
	readme.Wrapping = fyne.TextWrapWord

	return container.NewVBox(head, widget.NewSeparator(), readme)
}

// aboutLink points at the repository.
func aboutLink() fyne.CanvasObject {
	link, err := url.Parse(projectURL)
	if err != nil {
		// Unreachable for a constant that parses, but a window that panics on a
		// bad link is worse than one that shows the address as text.
		return widget.NewLabel(projectURL)
	}
	return widget.NewHyperlink("github.com/ushineko/clockwork-orange", link)
}
