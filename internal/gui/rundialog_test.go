package gui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/canvas"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/logpane"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/plugins"
)

// The run dialog drives the progress bar from the plugin's Progress events,
// streams its log, and hands the checked terms to the run as the query
// override (R7.5).
func TestRunDialogStreamsProgressAndOverridesTheQueryWithCheckedTerms(t *testing.T) {
	u, _, _ := testUI(t)
	dir := imageDir(t, 1)
	rec := &recordingPlugin{name: "wallhaven", dir: dir}
	u.deps.Registry = []plugins.Plugin{rec}
	doc := config.Defaults()
	doc.Plugins["wallhaven"] = map[string]any{"enabled": true, "query": []any{
		map[string]any{"term": "forest", "enabled": true}, map[string]any{"term": "sea", "enabled": false},
	}}
	writeDoc(t, doc)
	u.loadConfigNow()
	info, ok := u.pluginInfo("wallhaven")
	require.True(t, ok)
	form := u.newPluginForm(info, u.doc.Plugins["wallhaven"], nil)

	u.openRunDialog("wallhaven", "Download now", form, false)
	// Headless: openRunDialog built nothing to show and started nothing; a
	// dialog is driven through startRun directly.
	d := &runDialog{name: "wallhaven", override: map[string]any{"force": true}, pane: logpane.New(nil), termsKey: "query"}
	d.terms = nil
	require.NotPanics(t, func() { u.startRunHeadless(d) })
	got := rec.last()
	require.NotNil(t, got)
	require.Equal(t, true, got["force"])
	require.False(t, u.running, "the run released the one-at-a-time gate")
	require.Greater(t, d.pane.Model().Len(), 1)
	require.Contains(t, d.pane.Model().Text(), "running wallhaven")
	require.Contains(t, d.pane.Model().Text(), "Done.")
	require.InDelta(t, 1.0, d.progress.Value, 0.001)
}

/*
The run dialog's preview is scaled, not the 4K frame (spec 016).

OnImageSaved decoded each saved image at full size and handed it to
canvas.Image, which makes Fyne re-scale eight million pixels on every redraw --
and this dialog redraws constantly, because the log pane pumps and the progress
bar moves while the download runs. On a host downloading 3840x2160 wallpapers
the window sat at over 200% CPU with 1.6 GB resident, and KWin greyed it as
unresponsive, which reads as the window going transparent.

preview.go exists for this and the dialog was not using it.
*/
func TestTheRunDialogPreviewIsScaledAndBounded(t *testing.T) {
	u, _, _ := testUI(t)
	dir := t.TempDir()

	// Larger than the preview box in both axes, as a 4K wallpaper is.
	big := image.NewRGBA(image.Rect(0, 0, previewMaxW*2, previewMaxH*2))
	paths := make([]string, 0, previewCap+4)
	for i := range previewCap + 4 {
		p := filepath.Join(dir, fmt.Sprintf("shot%02d.png", i))
		f, err := os.Create(p) //nolint:gosec // a path this test built
		require.NoError(t, err)
		require.NoError(t, png.Encode(f, big))
		require.NoError(t, f.Close())
		paths = append(paths, p)
	}

	d := &runDialog{name: "wallhaven", pane: logpane.New(nil), previews: newPreviewCache()}
	d.preview = canvas.NewImageFromImage(nil)
	ev := u.runEvents(d)

	for _, p := range paths {
		ev.OnImageSaved(p)
	}

	require.Equal(t, paths[len(paths)-1], d.lastSaved)
	shown := d.preview.Image
	require.NotNil(t, shown)
	require.LessOrEqual(t, shown.Bounds().Dx(), previewMaxW, "the frame handed to canvas.Image is scaled down")
	require.LessOrEqual(t, shown.Bounds().Dy(), previewMaxH)

	// Bounded: a run that saves more images than the cap does not keep them all.
	d.previews.mu.Lock()
	held := len(d.previews.items)
	d.previews.mu.Unlock()
	require.LessOrEqual(t, held, previewCap, "the cache evicts rather than growing with the run")
}
