package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"
)

// --- Appearance (spec 010 R7.1) -----------------------------------------------------

// sampleLogLine is the monospace line the live sample shows: a log line that
// looks like this program's own is the useful preview of the console font.
const sampleLogLine = "[DEBUG] Selected image from source ~/Pictures/Wallpapers/Wallhaven: wallhaven-2e8e8m.jpg"

// buildAppearance is the scheme, font, text-size and scale picker, plus this
// program's console font and size. Fyne draws its own widgets, so these
// settings are the whole of what makes the window look like it belongs on the
// user's desktop, which is why they are a section rather than a line in a
// preferences dialog.
func (u *ui) buildAppearance() fyne.CanvasObject {
	// The choices the controls edit; each change saves and applies the whole
	// set, so one control cannot undo another's value.
	a := u.sh.Appearance()
	apply := func() { u.setAppearance(a) }

	scheme := widget.NewSelect(fdtheme.SchemeNames(), func(name string) {
		a.Scheme = name
		apply()
	})
	scheme.SetSelected(a.Scheme)

	font := widget.NewSelect(fdtheme.FontNames(), func(name string) {
		a.Font = name
		apply()
	})
	font.SetSelected(a.Font)

	sizes := make([]string, 0, len(fdtheme.TextSizes()))
	for _, s := range fdtheme.TextSizes() {
		sizes = append(sizes, fmt.Sprintf("%g", s))
	}
	size := widget.NewSelect(sizes, func(v string) {
		for _, s := range fdtheme.TextSizes() {
			if fmt.Sprintf("%g", s) == v {
				a.TextSize = s
				apply()
				return
			}
		}
	})
	size.SetSelected(fmt.Sprintf("%g", a.TextSize))

	reset := widget.NewButton("Reset to defaults", func() {
		d := fdtheme.DefaultAppearance()
		a.Scheme, a.Font, a.TextSize = d.Scheme, d.Font, d.TextSize
		scheme.SetSelected(a.Scheme)
		font.SetSelected(a.Font)
		size.SetSelected(fmt.Sprintf("%g", a.TextSize))
		apply()
	})

	// The console font is the monospace face of the log panes (Service,
	// Activity, plugin runs). It lives in clockwork-orange.yml as
	// console_font_family / console_font_size, where the Python GUI kept it,
	// and is set here so every look-and-feel choice is in one section.
	consoleFont := widget.NewSelect(consoleFontNames(), nil)
	consoleFont.SetSelected(widgets.OrNone(u.doc.ConsoleFontFamily, consoleFontDefault))
	consoleFont.OnChanged = func(name string) {
		u.doc.ConsoleFontFamily = name
		u.scheduleSave()
		u.sh.SetAppearance(u.sh.Appearance()) // same appearance, new console font through themeFor
		u.sh.Refresh()
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
			u.sh.Refresh()
		}
	}

	// Interface scale (R7.16): saved now, applied when the window next opens,
	// because Fyne fixes a window's scale when it is created.
	scales := make([]string, 0, len(fdtheme.ScaleChoices()))
	for _, s := range fdtheme.ScaleChoices() {
		scales = append(scales, fdtheme.ScaleLabel(s))
	}
	scaleSel := widget.NewSelect(scales, nil)
	scaleSel.SetSelected(fdtheme.ScaleLabel(a.Scale))
	restart := widget.NewButtonWithIcon("Restart the window now", theme.ViewRefreshIcon(), func() { u.sh.Restart() })
	restart.Hide()
	scaleSel.OnChanged = func(v string) {
		a.Scale = fdtheme.ScaleValue(v)
		apply()
		restart.Show()
		u.sh.Flash("Interface scale saved. It applies when the window next opens.", fd.StatusInfo)
	}

	form := widget.NewForm(
		widget.NewFormItem("Color scheme", scheme),
		widget.NewFormItem("Font", font),
		widget.NewFormItem("Text size", size),
		widget.NewFormItem("Console font", consoleFont),
		widget.NewFormItem("Console text size", consoleSizeSel),
		widget.NewFormItem("Interface scale", container.NewHBox(
			widgets.WithTip(scaleSel, "Enlarges everything in the window on top of the desktop's own scale. "+
				"Fyne fixes a window's scale when it is created, so this applies when the window next opens."),
			restart)),
	)

	return container.NewVScroll(container.NewVBox(
		widgets.Heading("Appearance", "How this window looks. Fyne draws its own widgets, so this is what decides whether it sits well next to the rest of your desktop."),
		form,
		container.NewHBox(reset),
		widgets.Dim("Every setting about how this window looks is here. The colour scheme, font and text "+
			"size are kept in Fyne's own preference store; the console font and its size are "+
			"written to clockwork-orange.yml as console_font_family and console_font_size, the "+
			"keys the previous versions used, and apply to the log panes in Service, Activity "+
			"and plugin runs."),
		widget.NewSeparator(),
		fdtheme.Sample(sampleLogLine),
		widget.NewSeparator(),
		widgets.Dim("The KDE schemes are transcribed from the desktop's colour-scheme files, the Adwaita ones from "+
			"libadwaita's named colours, the Windows and macOS ones from their published design tokens. They are "+
			"compiled in, so the window does not follow the desktop's current scheme and needs no desktop installed."),
		widgets.Dim("Fonts are read from the system font directories. Fyne draws its own text and does "+
			"not consult fontconfig, so this list is what was found on disk rather than what "+
			"the desktop is configured to use. A family with no bold or italic face is drawn "+
			"in its regular face for those styles."),
		widgets.Dim("Interface scale enlarges everything in the window, text included, on top of the desktop's "+
			"own scale. Fyne draws text without hinting, which on a fractionally scaled desktop reads "+
			"soft at the default size; 1.2 is usually enough. It takes effect when the window is opened."),
	))
}
