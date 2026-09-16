package gui

import (
	"runtime"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/config"
)

// --- Settings (R7.9) ----------------------------------------------------------

/*
settingsForm holds the Basic and Advanced widgets, kept apart from the section
so the round trip -- document into widgets, widgets into document -- can be
driven without a window manager.

Every change writes its value into u.doc at once and schedules the coalesced
save; there is no Save button, as in settings_widgets.py. The three mode
checks are mutually exclusive: ticking one clears the other two, and clearing
the last leaves the default mode.
*/
type settingsForm struct {
	body                      fyne.CanvasObject
	dual, desktop, lockscreen *widget.Check
	wait                      *widget.Entry
	font                      *widget.Select
	fontSize                  *widget.Entry
	extensions                *widget.Entry
	debug                     *widget.Check
	// Linux and macOS only; nil on Windows (settings_widgets.py).
	autostart      *widget.Check
	restartDelay   *widget.Entry
	logsRefresh    *widget.Entry
	autoUpdateLogs *widget.Check
}

// consoleFontDefault is what the Python offered when the config named none.
const consoleFontDefault = "Monospace"

// newSettingsForm builds the widgets from the document.
func (u *ui) newSettingsForm() *settingsForm {
	f := &settingsForm{}
	d := docDefaultsFor(u.doc)

	// Fyne's Check calls OnChanged from SetChecked too, so the handlers guard
	// against re-entry while set() is running.
	var setting bool
	f.dual = widget.NewCheck("Dual wallpapers (desktop + lock screen, different images)", func(b bool) {
		if setting {
			return
		}
		u.doc.DualWallpapers = b
		if b {
			u.doc.Desktop, u.doc.Lockscreen = false, false
			setting = true
			f.desktop.SetChecked(false)
			f.lockscreen.SetChecked(false)
			setting = false
		}
		u.scheduleSave()
		u.redrawStatus()
	})
	f.desktop = widget.NewCheck("Desktop wallpaper only", func(b bool) {
		if setting {
			return
		}
		u.doc.Desktop = b
		if b {
			u.doc.DualWallpapers, u.doc.Lockscreen = false, false
			setting = true
			f.dual.SetChecked(false)
			f.lockscreen.SetChecked(false)
			setting = false
		}
		u.scheduleSave()
		u.redrawStatus()
	})
	f.lockscreen = widget.NewCheck("Lock screen wallpaper only", func(b bool) {
		if setting {
			return
		}
		u.doc.Lockscreen = b
		if b {
			u.doc.DualWallpapers, u.doc.Desktop = false, false
			setting = true
			f.dual.SetChecked(false)
			f.desktop.SetChecked(false)
			setting = false
		}
		u.scheduleSave()
		u.redrawStatus()
	})

	f.wait = numericEntry(1, 86400, func(n int) { u.doc.DefaultWait = n; u.scheduleSave(); u.redrawStatus() })
	f.font = widget.NewSelect(consoleFontNames(), func(name string) {
		if setting {
			return
		}
		u.doc.ConsoleFontFamily = name
		u.scheduleSave()
	})
	f.fontSize = numericEntry(6, 48, func(n int) { u.doc.ConsoleFontSize = n; u.scheduleSave() })
	f.extensions = widget.NewEntry()
	f.extensions.SetPlaceHolder("Comma-separated extensions")
	f.extensions.OnChanged = func(s string) {
		if setting {
			return
		}
		u.doc.ImageExtensions = s
		u.scheduleSave()
	}
	f.debug = widget.NewCheck("Enable debug logging", func(b bool) {
		if setting {
			return
		}
		u.doc.Debug = b
		u.scheduleSave()
	})
	if runtime.GOOS != "windows" {
		f.autostart = widget.NewCheck("Start the service automatically on login", func(b bool) {
			if setting {
				return
			}
			u.doc.Autostart = b
			u.scheduleSave()
		})
		f.restartDelay = numericEntry(1, 300, func(n int) { u.doc.RestartDelay = n; u.scheduleSave() })
		f.logsRefresh = numericEntry(1, 300, func(n int) { u.doc.LogsRefreshInterval = n; u.scheduleSave() })
		f.autoUpdateLogs = widget.NewCheck("Auto-update service logs", func(b bool) {
			if setting {
				return
			}
			u.doc.AutoUpdateLogs = b
			u.scheduleSave()
		})
	}

	setting = true
	f.set(u.doc, d)
	setting = false

	basic := widget.NewForm(
		widget.NewFormItem("Wallpaper mode", container.NewVBox(f.dual, f.desktop, f.lockscreen)),
		widget.NewFormItem("Wait interval (s)", fixedWidth(f.wait, 120)),
		widget.NewFormItem("Console font", f.font),
		widget.NewFormItem("Console font size", fixedWidth(f.fontSize, 120)),
	)
	advanced := widget.NewForm(
		widget.NewFormItem("Image extensions", f.extensions),
		widget.NewFormItem("Debug mode", f.debug),
	)
	if f.autostart != nil {
		advanced.Append("Auto-start", f.autostart)
		advanced.Append("Restart delay (s)", fixedWidth(f.restartDelay, 120))
		advanced.Append("Logs refresh interval (s)", fixedWidth(f.logsRefresh, 120))
		advanced.Append("Auto-update logs", f.autoUpdateLogs)
	}
	f.body = container.NewVBox(card("Basic", basic), card("Advanced", advanced))
	return f
}

// set fills the widgets from the document (with defaults filled in).
func (f *settingsForm) set(doc, d config.Document) {
	switch {
	case doc.DualWallpapers:
		f.dual.SetChecked(true)
	case doc.Desktop:
		f.desktop.SetChecked(true)
	case doc.Lockscreen:
		f.lockscreen.SetChecked(true)
	}
	f.wait.SetText(strconv.Itoa(d.DefaultWait))
	f.font.SetSelected(orNone(d.ConsoleFontFamily, consoleFontDefault))
	f.fontSize.SetText(strconv.Itoa(d.ConsoleFontSize))
	f.extensions.SetText(d.ImageExtensions)
	f.debug.SetChecked(doc.Debug)
	if f.autostart != nil {
		f.autostart.SetChecked(doc.Autostart)
		f.restartDelay.SetText(strconv.Itoa(d.RestartDelay))
		f.logsRefresh.SetText(strconv.Itoa(d.LogsRefreshInterval))
		f.autoUpdateLogs.SetChecked(doc.AutoUpdateLogs)
	}
}

// numericEntry is a whole-number field validated inside [lo, hi]; onChange
// fires only for a value in range.
func numericEntry(lo, hi int, onChange func(int)) *widget.Entry {
	e := widget.NewEntry()
	e.Validator = intRange(lo, hi)
	var setting bool
	e.OnChanged = func(s string) {
		if setting {
			return
		}
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= lo && n <= hi {
			onChange(n)
		}
	}
	return e
}

// consoleFontNames is the font list for the console font Select: the system
// families the Appearance section found, with the Python default present
// whether or not a family of that name is installed.
func consoleFontNames() []string {
	names := fontNames()
	for _, n := range names {
		if n == consoleFontDefault {
			return names
		}
	}
	return append([]string{consoleFontDefault}, names...)
}

/*
buildSettings is the document as a form: Basic and Advanced, then a read-only
view of the YAML with Validate and Copy. Every change is written 1 s after
the last keystroke (auto-save), which the flash reports.
*/
func (u *ui) buildSettings() fyne.CanvasObject {
	f := u.newSettingsForm()
	yaml, err := config.Marshal(u.doc)
	text := string(yaml)
	if err != nil {
		text = "Error formatting YAML: " + err.Error()
	}
	raw := widget.NewMultiLineEntry()
	raw.TextStyle = fyne.TextStyle{Monospace: true}
	raw.SetText(text)
	raw.Disable()
	raw.Wrapping = fyne.TextWrapOff

	validate := widget.NewButtonWithIcon("Validate", theme.ConfirmIcon(), func() {
		if _, err := config.Parse(yaml); err != nil {
			u.flash("Invalid YAML syntax: "+err.Error(), StatusBad)
			return
		}
		u.flash("YAML syntax is valid.", StatusGood)
	})
	copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		u.app.Clipboard().SetContent(text)
		u.flash("Copied the configuration to the clipboard.", StatusGood)
	})

	where := u.docPath
	if !u.docExists {
		where += " (not written yet; the first change creates it)"
	}
	return container.NewVBox(
		heading("Settings", "Changes are saved a second after you make them, to "+where+
			". The service and the command line read the same file."),
		f.body,
		card("Raw YAML",
			container.NewHBox(validate, copyBtn),
			fixedHeight(raw, 320),
			note("Read-only: this is the document as it will be written. Edit it above, or in a text "+
				"editor while the window is closed.", StatusInfo)),
	)
}
