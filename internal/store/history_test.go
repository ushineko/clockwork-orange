package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func openTempHistory(t *testing.T) *History {
	t.Helper()
	h, err := OpenHistory(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	return h
}

func TestAddEntryReturnsFalseNotErrorOnDuplicateURL(t *testing.T) {
	h := openTempHistory(t)
	img := goldenImage(t, "landscape_1920x1080.png")

	added, err := h.AddEntry("https://example.com/a.jpg", img, "wallhaven")
	require.NoError(t, err)
	require.True(t, added)

	// Same URL, different file: url_hash UNIQUE fires; Python returned False.
	added, err = h.AddEntry("https://example.com/a.jpg", goldenImage(t, "square_600x600.png"), "dup")
	require.NoError(t, err)
	require.False(t, added)

	// Different URL, same image bytes: allowed (only url_hash is unique).
	added, err = h.AddEntry("https://example.com/c.jpg", img, "imported")
	require.NoError(t, err)
	require.True(t, added)

	stats, err := h.Stats()
	require.NoError(t, err)
	require.Equal(t, 2, stats.TotalRecords)
	require.Equal(t, 1, stats.UniqueImages)
	require.Positive(t, stats.DBSizeBytes)
}

func TestAddEntryReportsUnreadableImageInsteadOfRecordingIt(t *testing.T) {
	h := openTempHistory(t)
	added, err := h.AddEntry("https://example.com/x.jpg", filepath.Join(t.TempDir(), "missing.jpg"), "x")
	require.Error(t, err)
	require.False(t, added)
	seen, err := h.SeenURL("https://example.com/x.jpg")
	require.NoError(t, err)
	require.False(t, seen)
}

func TestSeenImageTreatsMissingFileAsUnseen(t *testing.T) {
	h := openTempHistory(t)
	seen, err := h.SeenImage(filepath.Join(t.TempDir(), "gone.jpg"))
	require.NoError(t, err)
	require.False(t, seen)
}

func TestSeenImageMatchesByContentNotByPath(t *testing.T) {
	h := openTempHistory(t)
	_, err := h.AddEntry("https://example.com/a.jpg", goldenImage(t, "small_800x600.jpg"), "local")
	require.NoError(t, err)

	// A byte-identical copy under another name is the same image.
	dup := copyFile(t, goldenImage(t, "small_800x600.jpg"), "renamed.jpg")
	seen, err := h.SeenImage(dup)
	require.NoError(t, err)
	require.True(t, seen)

	seen, err = h.SeenImage(goldenImage(t, "square_600x600.png"))
	require.NoError(t, err)
	require.False(t, seen)
}

func TestClearEmptiesTableAndLeavesDatabaseUsable(t *testing.T) {
	h := openTempHistory(t)
	for _, u := range []string{"https://e.com/1", "https://e.com/2"} {
		_, err := h.AddEntry(u, goldenImage(t, "square_600x600.png"), "x")
		require.NoError(t, err)
	}
	require.NoError(t, h.Clear())
	stats, err := h.Stats()
	require.NoError(t, err)
	require.Zero(t, stats.TotalRecords)
	require.Zero(t, stats.UniqueImages)

	// VACUUM must not have left the connection in a broken state.
	added, err := h.AddEntry("https://e.com/1", goldenImage(t, "square_600x600.png"), "x")
	require.NoError(t, err)
	require.True(t, added)
}

func TestHistoryTimestampUsesInjectableClockAsFloatSeconds(t *testing.T) {
	fixedClock(t, 1700000000.5)
	h := openTempHistory(t)
	_, err := h.AddEntry("https://e.com/1", goldenImage(t, "square_600x600.png"), "x")
	require.NoError(t, err)
	var ts float64
	require.NoError(t, h.db.QueryRowContext(context.Background(), `SELECT timestamp FROM downloads`).Scan(&ts))
	require.InDelta(t, 1700000000.5, ts, 1e-6)
}

func TestOpenHistoryFailsWhenParentDirectoryMissing(t *testing.T) {
	_, err := OpenHistory(filepath.Join(t.TempDir(), "no", "such", "dir", "history.db"))
	require.Error(t, err)
}
