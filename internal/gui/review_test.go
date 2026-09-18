package gui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/plugins"
)

// reviewDir writes n images with distinct mtimes, oldest first, and returns
// the paths newest first: the order the review shows.
func reviewDir(t *testing.T, n int) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	var newestFirst []string
	base := time.Now().Add(-time.Hour)
	for i := range n {
		p := filepath.Join(dir, string(rune('a'+i))+".jpg")
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
		require.NoError(t, os.Chtimes(p, base.Add(time.Duration(i)*time.Minute), base.Add(time.Duration(i)*time.Minute)))
		newestFirst = append([]string{p}, newestFirst...)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	return dir, newestFirst
}

// ←/→ move within the list and stop at its ends; Space marks and unmarks the
// image on screen; marks index the list, so a rescan clears them (R7.6).
func TestReviewKeyboardNavigatesAndMarks(t *testing.T) {
	u, _, _ := testUI(t)
	dir, want := reviewDir(t, 3)
	r := &reviewModel{plugin: "local", dir: dir, marked: map[int]bool{}}
	r.scan()
	require.Equal(t, want, r.images, "newest first; the .txt is not an image")
	require.Equal(t, 0, r.index)

	require.True(t, r.handleKey(u, fyne.KeyLeft), "at the first image ← is still ours")
	require.Equal(t, 0, r.index, "and does not move before the first")
	require.True(t, r.handleKey(u, fyne.KeyRight))
	require.Equal(t, 1, r.index)
	require.True(t, r.handleKey(u, fyne.KeySpace))
	require.True(t, r.marked[1])
	require.Equal(t, 1, r.markedCount())
	require.Contains(t, r.infoText(time.Now()), "[MARKED FOR DELETION]")
	require.Contains(t, r.infoText(time.Now()), "Image 2 of 3")
	require.True(t, r.handleKey(u, fyne.KeySpace))
	require.False(t, r.marked[1], "Space toggles")
	r.handleKey(u, fyne.KeySpace)
	r.handleKey(u, fyne.KeyRight)
	r.handleKey(u, fyne.KeyRight)
	require.Equal(t, 2, r.index, "→ stops at the last image")
	r.handleKey(u, fyne.KeySpace)
	require.Equal(t, []string{want[1], want[2]}, r.markedPaths())
	require.False(t, r.handleKey(u, fyne.KeyF5), "other keys are not ours")

	r.scan()
	require.Zero(t, r.markedCount(), "a rescan clears marks, which indexed the old list")

	empty := &reviewModel{dir: t.TempDir(), marked: map[int]bool{}}
	empty.scan()
	require.False(t, empty.handleKey(u, fyne.KeyRight), "no images: the keys fall through")
	require.Equal(t, "No images found for review.", empty.infoText(time.Now()))
	missing := &reviewModel{dir: "/nonexistent/review", marked: map[int]bool{}}
	missing.scan()
	require.Contains(t, missing.infoText(time.Now()), "Directory not found")
}

// The info panel's relative time is get_relative_time's wording.
func TestRelativeTimeWordsMatchThePython(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	require.Equal(t, "Just now", relativeTime(now.Add(-30*time.Second), now))
	require.Equal(t, "1 min ago", relativeTime(now.Add(-90*time.Second), now))
	require.Equal(t, "5 mins ago", relativeTime(now.Add(-5*time.Minute), now))
	require.Equal(t, "1 hour ago", relativeTime(now.Add(-time.Hour), now))
	require.Equal(t, "3 hours ago", relativeTime(now.Add(-3*time.Hour), now))
	require.Equal(t, "2 days ago", relativeTime(now.Add(-49*time.Hour), now))
	require.Equal(t, "2026-09-01", relativeTime(now.Add(-14*24*time.Hour), now))
}

// A marked image gets a red border and both diagonals, pen max(5, 2 % of the
// shorter side); the middle of a quadrant stays untouched.
func TestMarkOverlayDrawsARedBorderAndBothDiagonals(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := range 300 {
		for x := range 400 {
			src.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	out := markOverlay(src)
	red := color.RGBA{R: 255, A: 255}
	pen := 6 // 2 % of 300
	require.Equal(t, red, out.RGBAAt(0, 0), "corner is border")
	require.Equal(t, red, out.RGBAAt(200, pen-1), "top edge is border")
	require.Equal(t, red, out.RGBAAt(399, 150), "right edge is border")
	require.Equal(t, red, out.RGBAAt(200, 150), "the diagonals cross at the centre")
	require.Equal(t, red, out.RGBAAt(100, 75), "on the main diagonal")
	require.Equal(t, red, out.RGBAAt(300, 75), "on the other diagonal")
	require.Equal(t, color.RGBA{B: 255, A: 255}, out.RGBAAt(200, 60), "between the lines the image is untouched")
	require.Equal(t, color.RGBA{B: 255, A: 255}, src.RGBAAt(0, 0), "the source is not modified")

	small := markOverlay(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	require.Equal(t, red, small.RGBAAt(4, 50), "pen never thinner than 5")
	require.NotEqual(t, red, small.RGBAAt(5, 50))
}

// Apply runs the plugin with action=process_blacklist, the marked paths as
// targets and force=true, then rescans (R7.6).
func TestApplyBlacklistSendsProcessBlacklistWithTheMarkedPaths(t *testing.T) {
	u, _, _ := testUI(t)
	dir, want := reviewDir(t, 3)
	rec := &recordingPlugin{name: "local", dir: dir}
	u.deps.Registry = []plugins.Plugin{rec}
	doc := config.Defaults()
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": dir}
	writeDoc(t, doc)
	u.loadConfigNow()

	body := u.buildPlugin("local")
	rv := u.review
	require.NotNil(t, rv)
	u.pluginTab = 1 // the review keys work only on the Review tab
	require.Equal(t, want, rv.images)
	rv.handleKey(u, fyne.KeySpace)
	rv.handleKey(u, fyne.KeyRight)
	rv.handleKey(u, fyne.KeyRight)
	rv.handleKey(u, fyne.KeySpace)
	require.Equal(t, "Apply blacklist (2)", rv.applyBtn.Text)
	require.False(t, rv.applyBtn.Disabled())
	_ = body

	u.applyBlacklist("local", rv)
	got := rec.last()
	require.NotNil(t, got, "the plugin was run")
	require.Equal(t, "process_blacklist", got["action"])
	require.Equal(t, true, got["force"])
	require.Equal(t, []any{want[0], want[2]}, got["targets"])
	require.Equal(t, dir, got["path"], "the plugin's own block rides along")
	require.Zero(t, rv.markedCount(), "rescanned after the run")
}

// The arrow keys move the review only while its tab is showing; on the
// Configuration tab they would change an image nobody can see (R7.18).
func TestReviewKeysAreIgnoredOnTheConfigurationTab(t *testing.T) {
	u, _, _ := testUI(t)
	dir, _ := reviewDir(t, 3)
	doc := config.Defaults()
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": dir}
	writeDoc(t, doc)
	u.loadConfigNow()
	_ = u.buildPlugin("local")
	u.sh.Select("Local")
	u.pluginTab = 0
	u.onTypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 0, u.review.index)
	u.pluginTab = 1
	u.onTypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	require.Equal(t, 1, u.review.index)
}
