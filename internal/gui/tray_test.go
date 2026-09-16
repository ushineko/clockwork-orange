package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/require"
)

// trayApp is the test app with the desktop.App tray methods, standing in for
// the glfw driver's app on a desktop with a system tray.
type trayApp struct {
	fyne.App
	menu *fyne.Menu
	icon fyne.Resource
}

func (a *trayApp) SetSystemTrayMenu(m *fyne.Menu)       { a.menu = m }
func (a *trayApp) SetSystemTrayIcon(icon fyne.Resource) { a.icon = icon }
func (a *trayApp) SetSystemTrayWindow(fyne.Window)      {}

// With a tray, closing the window hides it and installs Show / About / Quit;
// without one it quits, because a hidden window with no way back is a process
// the user cannot see and cannot stop (R7.11).
func TestCloseInterceptHidesToTrayOrQuits(t *testing.T) {
	u, _, _ := testUI(t)
	require.False(t, hasTray(u.app), "the test driver has no tray")
	quit := false
	u.app = &quitRecorder{App: u.app, onQuit: func() { quit = true }}
	u.onClose()
	require.True(t, quit, "no tray: close quits")
	require.False(t, u.hiddenToTray)

	u2, _, _ := testUI(t)
	tray := &trayApp{App: test.NewApp()}
	u2.app = tray
	require.True(t, hasTray(u2.app))
	u2.setupTray()
	require.NotNil(t, tray.menu)
	require.NotNil(t, tray.icon)
	var labels []string
	for _, it := range tray.menu.Items {
		if !it.IsSeparator {
			labels = append(labels, it.Label)
		}
	}
	require.Equal(t, []string{"Show", "About", "Quit"}, labels)
	u2.onClose()
	require.True(t, u2.hiddenToTray, "with a tray: close hides")
	u2.showWindow()
	require.False(t, u2.hiddenToTray)
}

type quitRecorder struct {
	fyne.App
	onQuit func()
}

func (q *quitRecorder) Quit() { q.onQuit() }
