package gui

import (
	"testing"

	"github.com/stretchr/testify/require"

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
	d := &runDialog{name: "wallhaven", override: map[string]any{"force": true}, pane: newLogPane(), termsKey: "query"}
	d.terms = nil
	require.NotPanics(t, func() { u.startRunHeadless(d) })
	got := rec.last()
	require.NotNil(t, got)
	require.Equal(t, true, got["force"])
	require.False(t, u.running, "the run released the one-at-a-time gate")
	require.Greater(t, d.pane.log.len(), 1)
	require.Contains(t, d.pane.log.text(), "running wallhaven")
	require.Contains(t, d.pane.log.text(), "Done.")
	require.InDelta(t, 1.0, d.progress.Value, 0.001)
}
