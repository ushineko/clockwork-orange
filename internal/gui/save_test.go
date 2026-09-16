package gui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/config"
)

/*
Auto-save coalesces (R7.9, D6): five edits in quick succession are one write,
landing no sooner than saveDelay after the last of them, and the write keeps
the plugin blocks this build does not know.

The coalescing is the whole point: settings_widgets.py armed a 1 s single-shot
timer on every signal, and a document written on every keystroke is a
watcher-driven daemon restarting its cycle on every keystroke.
*/
func TestAutoSaveCoalescesRapidEditsIntoOneWriteAndKeepsUnknownBlocks(t *testing.T) {
	u, _, _ := testUI(t)
	doc := config.Defaults()
	doc.Plugins["stable_diffusion"] = map[string]any{"enabled": false, "prompt": "castle", "steps": 30}
	writeDoc(t, doc)
	u.loadConfigNow()

	saved := make(chan time.Time, 8)
	u.saved = func() { saved <- time.Now() }

	start := time.Now()
	var last time.Time
	for i := range 5 {
		u.doc.DefaultWait = 100 + i
		u.scheduleSave()
		last = time.Now()
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case at := <-saved:
		require.GreaterOrEqual(t, at.Sub(last), saveDelay-20*time.Millisecond, "the write must wait for the burst to settle")
		require.Less(t, at.Sub(start), 3*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("no write happened")
	}
	select {
	case <-saved:
		t.Fatal("five edits produced more than one write")
	case <-time.After(saveDelay + 300*time.Millisecond):
	}

	got, err := config.Load(config.DefaultPath())
	require.NoError(t, err)
	require.Equal(t, 104, got.DefaultWait, "the last edit wins")
	require.Equal(t, "castle", got.Plugins["stable_diffusion"]["prompt"], "an unknown plugin block survives the save (D6)")
	require.Equal(t, 30, got.Plugins["stable_diffusion"]["steps"])
}

// A form edit lands in the document at once, so a save scheduled by another
// edit carries it too; the Basic form's mode checks stay mutually exclusive.
func TestSettingsFormWritesTheDocumentAndKeepsTheModeChecksExclusive(t *testing.T) {
	u, _, _ := testUI(t)
	f := u.newSettingsForm()

	f.dual.SetChecked(true)
	require.True(t, u.doc.DualWallpapers)
	require.False(t, f.desktop.Checked)
	f.lockscreen.SetChecked(true)
	require.True(t, u.doc.Lockscreen)
	require.False(t, u.doc.DualWallpapers, "ticking lock-screen-only clears dual")
	require.False(t, f.dual.Checked)

	f.wait.SetText("42")
	require.Equal(t, 42, u.doc.DefaultWait)
	f.wait.SetText("0")
	require.Equal(t, 42, u.doc.DefaultWait, "an out-of-range value is not written")
	f.fontSize.SetText("14")
	require.Equal(t, 14, u.doc.ConsoleFontSize)
	f.extensions.SetText(".jpg,.png")
	require.Equal(t, ".jpg,.png", u.doc.ImageExtensions)
	f.debug.SetChecked(true)
	require.True(t, u.doc.Debug)
	if f.autostart != nil {
		f.autostart.SetChecked(true)
		require.True(t, u.doc.Autostart)
		f.logsRefresh.SetText("30")
		require.Equal(t, 30, u.doc.LogsRefreshInterval)
	}
}

// The form shows the daemon's defaults for keys the file does not carry,
// rather than blank fields, and the Python's Monospace default is offered
// whether or not such a family is installed.
func TestSettingsFormShowsDefaultsForAbsentKeys(t *testing.T) {
	u, _, _ := testUI(t)
	f := u.newSettingsForm()
	require.Equal(t, "300", f.wait.Text)
	require.Equal(t, "10", f.fontSize.Text)
	require.Equal(t, consoleFontDefault, f.font.Selected)
	require.Contains(t, consoleFontNames(), consoleFontDefault)
	require.Equal(t, config.Defaults().ImageExtensions, f.extensions.Text)
}
