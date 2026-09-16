// Copied from nmsbonker (same author) — keep in sync by hand; the sample text is this project's.

package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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

	// The console font is the monospace face of the log panes (Service,
	// Activity, plugin runs). It lives in clockwork-orange.yml as
	// console_font_family / console_font_size, where the Python GUI kept it,
	// and is set here so every look-and-feel choice is in one section.
	consoleFont := widget.NewSelect(consoleFontNames(), nil)
	consoleFont.SetSelected(orNone(u.doc.ConsoleFontFamily, consoleFontDefault))
	consoleFont.OnChanged = func(name string) {
		u.doc.ConsoleFontFamily = name
		u.scheduleSave()
		u.applyAppearance()
		u.refresh()
	}
	consoleSizes := make([]string, 0, 12)
	for _, n := range []int{8, 9, 10, 11, 12, 13, 14, 16, 18, 20, 24} {
		consoleSizes = append(consoleSizes, fmt.Sprintf("%d", n))
	}
	consoleSizeSel := widget.NewSelect(consoleSizes, nil)
	consoleSizeSel.SetSelected(fmt.Sprintf("%d", int(consoleSize(u.doc))))
	consoleSizeSel.OnChanged = func(v string) {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 6 && n <= 48 {
			u.doc.ConsoleFontSize = n
			u.scheduleSave()
			u.refresh()
		}
	}

	// Interface scale (R7.16): saved now, applied when the window next opens,
	// because Fyne fixes a window's scale when it is created.
	scaleSel := widget.NewSelect(scaleLabels(), nil)
	scaleSel.SetSelected(scaleLabel(float32(u.app.Preferences().Float(prefScale))))
	restart := widget.NewButtonWithIcon("Restart the window now", theme.ViewRefreshIcon(), func() { u.restart() })
	restart.Hide()
	scaleSel.OnChanged = func(v string) {
		u.app.Preferences().SetFloat(prefScale, float64(scaleValue(v)))
		restart.Show()
		u.flash("Interface scale saved. It applies when the window next opens.", StatusInfo)
	}

	form := widget.NewForm(
		widget.NewFormItem("Color scheme", scheme),
		widget.NewFormItem("Font", font),
		widget.NewFormItem("Text size", size),
		widget.NewFormItem("Console font", consoleFont),
		widget.NewFormItem("Console text size", consoleSizeSel),
		widget.NewFormItem("Interface scale", container.NewHBox(scaleSel, restart)),
	)

	noteText := widget.NewLabel(
		"Every setting about how this window looks is here. The colour scheme, font and text " +
			"size are kept in Fyne's own preference store; the console font and its size are " +
			"written to clockwork-orange.yml as console_font_family and console_font_size, the " +
			"keys the previous versions used, and apply to the log panes in Service, Activity " +
			"and plugin runs.")
	noteText.Wrapping = fyne.TextWrapWord
	noteText.Importance = widget.LowImportance

	scaleNote := widget.NewLabel(
		"Interface scale enlarges everything in the window, text included, on top of the desktop's " +
			"own scale. Fyne draws text without hinting, which on a fractionally scaled desktop reads " +
			"soft at the default size; 1.2 is usually enough. It takes effect when the window is opened.")
	scaleNote.Wrapping = fyne.TextWrapWord
	scaleNote.Importance = widget.LowImportance

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
		scaleNote,
	))
}

// scaleLabels are the Select's entries for scaleChoices.
func scaleLabels() []string {
	out := make([]string, 0, len(scaleChoices))
	for _, s := range scaleChoices {
		out = append(out, scaleLabel(s))
	}
	return out
}

// scaleLabel names one choice; anything not in the table is shown as its
// number so a value set by hand is not silently replaced.
func scaleLabel(s float32) string {
	if s <= 0 {
		return "System"
	}
	return fmt.Sprintf("%g×", s)
}

// scaleValue is the inverse of scaleLabel.
func scaleValue(label string) float32 {
	if label == "System" {
		return 0
	}
	var f float32
	if _, err := fmt.Sscanf(label, "%g", &f); err != nil || f <= 0 {
		return 0
	}
	return f
}
