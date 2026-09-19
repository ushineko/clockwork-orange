package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

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

/*
pluginIcon is a plugin section's navigation mark (spec 013 R10).

Themed, so the drawing is recoloured for the active scheme the way every stock
icon is; a plugin this build has no drawing for keeps the generic picture icon,
which is what every plugin had before.

Deferred like the rest of the section icons: a theme icon cannot be constructed
before there is an app, and --help lists the section names.
*/
func pluginIcon(name string) func() fyne.Resource {
	return func() fyne.Resource {
		svg := assets.PluginSVG(name)
		if svg == nil {
			return theme.FileImageIcon()
		}
		return theme.NewThemedResource(fyne.NewStaticResource("plugin-"+name+".svg", svg))
	}
}
