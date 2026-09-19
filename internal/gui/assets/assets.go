/*
Package assets holds the files the desktop front end embeds.

The icon is a copy of packaging/icons/clockwork-orange-256x256.png. It is
duplicated rather than referenced because go:embed cannot reach outside the
package directory, and the packaging copy has to stay on disk for the desktop
entry and the icon theme to install. Change one and change the other. The
README is the copy the About section renders (spec 010 R7.10); the Phase 7
cutover rewrites both copies together.

The three plugin marks are this package's own drawings (spec 013 R10). They are
monochrome silhouettes, filled and never stroked, because Fyne recolours an
SVG's fills for the active scheme and leaves its strokes as authored: a stroked
mark stays the colour it was drawn in and disappears into a light scheme. For
the same reason they use only the attributes Fyne's recolouring understands --
no rx on a rect, no fill-rule on a path -- since it re-marshals the document and
drops the rest. A hole is cut by winding the inner subpath the other way, which
nonzero filling handles without a rule.
*/
package assets

import _ "embed"

//go:embed clockwork-orange.png
var iconPNG []byte

//go:embed README.md
var readme []byte

//go:embed plugin-local.svg
var pluginLocalSVG []byte

//go:embed plugin-wallhaven.svg
var pluginWallhavenSVG []byte

//go:embed plugin-duckduckgo.svg
var pluginDuckDuckGoSVG []byte

// IconPNG is the application icon: the window icon, the tray icon, and the
// image the About section draws at 72 px.
func IconPNG() []byte { return iconPNG }

// README is the project README, rendered by the About section.
func README() []byte { return readme }

/*
PluginSVG is the navigation mark for a plugin, by the plugin's registry name,
and nil for a name this build has no drawing for.

One silhouette per source rather than one picture icon for all of them: three
identical entries are three things to guess between, and in the icons-only
navigation the mark is nearly all there is. A folder for the directory it
reads, a wall of tiles for the gallery it downloads from, a magnifier for the
search it runs.
*/
func PluginSVG(name string) []byte {
	switch name {
	case "local":
		return pluginLocalSVG
	case "wallhaven":
		return pluginWallhavenSVG
	case "duckduckgo_images":
		return pluginDuckDuckGoSVG
	}
	return nil
}
