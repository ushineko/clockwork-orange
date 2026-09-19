package gui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/clockwork-orange/internal/config"
)

// --- the settings file (spec 013 R2) -----------------------------------------

// The window's own settings go in the directory this program already owns,
// beside history.db and blacklist.db, rather than in a second directory named
// for the app ID (AC2).
func TestTheSettingsFileLivesBesideTheStores(t *testing.T) {
	u, _, _ := testUI(t)
	require.Equal(t, filepath.Join(config.StateDir(), "gui-settings.json"), u.sh.Settings().Path())
	require.Equal(t, filepath.Join(config.Dir(), "clockwork-orange", "gui-settings.json"), u.sh.Settings().Path(),
		"beside the SQLite stores, not in a directory named for the app ID")
}

// An appearance chosen in one run is in effect in the next, through that file
// rather than through Fyne's preference store (AC3).
func TestAnAppearanceChosenInOneRunIsInEffectInTheNext(t *testing.T) {
	u, _, _ := testUI(t)
	a := u.sh.Appearance()
	a.Scheme, a.TextSize = "Oxygen Dark", 15
	u.setAppearance(a)
	require.NoError(t, u.sh.Settings().Flush())
	require.FileExists(t, u.sh.Settings().Path())

	// The next launch reads the file, with a preference store that was never
	// written to.
	next := shell.Headless(u.sh.App, u.shellOptions(Options{}))
	require.Equal(t, "Oxygen Dark", next.Appearance().Scheme)
	require.Equal(t, float32(15), next.Appearance().TextSize)
}

// What the CLI can also see stays in clockwork-orange.yml: nothing the daemon
// or a 2.9.x build reads moved into the settings file (AC4).
func TestTheSettingsFileHoldsNothingTheCLIReads(t *testing.T) {
	u, _, _ := testUI(t)
	u.doc.WindowWidth, u.doc.WindowHeight = 1000, 700
	u.doc.ConsoleFontFamily, u.doc.ConsoleFontSize = "DejaVu Sans Mono", 12
	u.performSave()
	require.NoError(t, u.sh.Settings().Flush())

	doc, err := config.Load(config.DefaultPath())
	require.NoError(t, err)
	require.Equal(t, 1000, doc.WindowWidth)
	require.Equal(t, 700, doc.WindowHeight)
	require.Equal(t, "DejaVu Sans Mono", doc.ConsoleFontFamily)
	require.Equal(t, 12, doc.ConsoleFontSize)

	for _, key := range []string{"window", "windowSize", "console", "consoleFont"} {
		require.Falsef(t, u.sh.Settings().Has(key), "%q belongs in the YAML, not the settings file", key)
	}
}

// --- the navigation's shape (spec 013 R3) -------------------------------------

// Every shape is offered, which is what puts the control in the header and
// binds Ctrl+B (AC5), and a shape chosen in one run is in effect in the next
// (AC7).
func TestTheNavigationShapeIsOfferedAndRemembered(t *testing.T) {
	u, _, _ := testUI(t)
	o := u.shellOptions(Options{})
	require.Equal(t, []shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden}, o.NavModes)
	require.Equal(t, []shell.NavPlacement{shell.NavLeft, shell.NavTop}, o.NavPlacements)

	// Written the way SetNavShape writes it. SetNavShape itself relays the
	// window, which a headless shell with a stand-in window has not got; what
	// this pins is that the program's options let the stored shape through.
	require.NoError(t, u.sh.Settings().Set(shell.NavKey, map[string]string{
		"mode":      shell.NavIcons.String(),
		"placement": shell.NavTop.String(),
	}))
	require.NoError(t, u.sh.Settings().Flush())

	next := shell.Headless(u.sh.App, u.shellOptions(Options{}))
	mode, place := next.NavShape()
	require.Equal(t, shell.NavIcons, mode)
	require.Equal(t, shell.NavTop, place)
}

// The window's own key handler is untouched: Ctrl+B is the shell's, and the
// review's keys still reach the review (AC6).
func TestTheProgramsKeyHandlerDoesNotBindCtrlB(t *testing.T) {
	u, _, _ := testUI(t)
	require.NotPanics(t, func() { u.onTypedKey(&fyne.KeyEvent{Name: fyne.KeyB}) },
		"the program's handler ignores B; Ctrl+B is a shell shortcut, not a typed key")
}

// --- the appearance is not read from the preference store (spec 013 R2.2) -----

func TestTheThemeComesFromTheAppearanceTheShellHolds(t *testing.T) {
	u, _, _ := testUI(t)
	a := u.sh.Appearance()
	a.Scheme = "Adwaita Light"
	u.setAppearance(a)
	th, ok := u.themeFor(u.sh.Appearance()).(fdtheme.Theme)
	require.True(t, ok)
	require.Equal(t, "Adwaita Light", th.Palette().Name)
}
