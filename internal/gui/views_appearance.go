// Copied from nmsbonker (same author) — keep in sync by hand; the sample text is this project's.

package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// --- Appearance (spec 010 R7.1) -----------------------------------------------------

// buildAppearance is the scheme, font, and text-size picker. Fyne draws its own
// widgets, so these three settings are the whole of what makes the window look
// like it belongs on the user's desktop — which is why they are a section
// rather than a line in a preferences dialog.
func (u *ui) buildAppearance() fyne.CanvasObject {
	scheme := widget.NewSelect(paletteNames(), func(name string) {
		u.scheme = name
		u.applyAppearance()
	})
	scheme.SetSelected(u.scheme)

	font := widget.NewSelect(fontNames(), func(name string) {
		u.fontName = name
		u.applyAppearance()
	})
	font.SetSelected(u.fontName)

	sizes := make([]string, 0, len(textSizes))
	for _, s := range textSizes {
		sizes = append(sizes, fmt.Sprintf("%g", s))
	}
	size := widget.NewSelect(sizes, func(v string) {
		for _, s := range textSizes {
			if fmt.Sprintf("%g", s) == v {
				u.textSize = s
				u.applyAppearance()
				return
			}
		}
	})
	size.SetSelected(fmt.Sprintf("%g", u.textSize))

	reset := widget.NewButton("Reset to defaults", func() {
		u.scheme, u.fontName, u.textSize = palettes[0].name, defaultFontName, defaultTextSize
		scheme.SetSelected(u.scheme)
		font.SetSelected(u.fontName)
		size.SetSelected(fmt.Sprintf("%g", u.textSize))
		u.applyAppearance()
	})

	form := widget.NewForm(
		widget.NewFormItem("Color scheme", scheme),
		widget.NewFormItem("Font", font),
		widget.NewFormItem("Text size", size),
	)

	noteText := widget.NewLabel(
		"These are saved and restored the next time the window opens. They are the only thing " +
			"this application keeps in Fyne's own preference store; everything else lives in " +
			"clockwork-orange.yml, which the command line and the service read too.")
	noteText.Wrapping = fyne.TextWrapWord
	noteText.Importance = widget.LowImportance

	fontNote := widget.NewLabel(
		"Fonts are read from the system font directories. Fyne draws its own text and does " +
			"not consult fontconfig, so this list is what was found on disk rather than what " +
			"the desktop is configured to use. A family with no bold or italic face is drawn " +
			"in its regular face for those styles.")
	fontNote.Wrapping = fyne.TextWrapWord
	fontNote.Importance = widget.LowImportance

	schemeNote := widget.NewLabel(
		"The KDE schemes are transcribed from /usr/share/color-schemes; the Adwaita ones from " +
			"libadwaita's named colors. They are compiled in, so the window does not follow the " +
			"desktop's current scheme and does not need KDE or GNOME installed.")
	schemeNote.Wrapping = fyne.TextWrapWord
	schemeNote.Importance = widget.LowImportance

	sample := container.NewVBox(
		widget.NewLabelWithStyle("Sample", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Regular text at the chosen size."),
		widget.NewLabelWithStyle("Bold text, as used for headings.", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("[DEBUG] Selected image from source ~/Pictures/Wallpapers/Wallhaven: wallhaven-2e8e8m.jpg",
			fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
		container.NewHBox(
			statusText("active", StatusGood), statusText("activating", StatusWarn),
			statusText("failed", StatusBad),
		),
	)

	return container.NewVScroll(container.NewVBox(
		heading("Appearance", "How this window looks. Fyne draws its own widgets, so this is what decides whether it sits well next to the rest of your desktop."),
		form,
		container.NewHBox(reset),
		noteText,
		widget.NewSeparator(),
		sample,
		widget.NewSeparator(),
		schemeNote,
		fontNote,
	))
}
