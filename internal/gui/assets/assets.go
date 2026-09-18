/*
Package assets holds the files the desktop front end embeds.

The icon is a copy of packaging/icons/clockwork-orange-256x256.png. It is
duplicated rather than referenced because go:embed cannot reach outside the
package directory, and the packaging copy has to stay on disk for the desktop
entry and the icon theme to install. Change one and change the other. The
README is the copy the About section renders (spec 010 R7.10); the Phase 7
cutover rewrites both copies together.
*/
package assets

import _ "embed"

//go:embed clockwork-orange.png
var iconPNG []byte

//go:embed README.md
var readme []byte

// IconPNG is the application icon: the window icon, the tray icon, and the
// image the About section draws at 72 px.
func IconPNG() []byte { return iconPNG }

// README is the project README, rendered by the About section.
func README() []byte { return readme }
