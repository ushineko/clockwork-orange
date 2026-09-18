package gui

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/clockwork-orange/internal/gui/assets"
)

// The window, taskbar and tray icon.
//
// The bytes live in internal/gui/assets, embedded at build time.
// packaging/icons/ holds the same drawing again, on disk for the installer to
// place into the icon theme.
func appIcon() fyne.Resource {
	return fyne.NewStaticResource("clockwork-orange.png", assets.IconPNG())
}
