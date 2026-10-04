package plugins

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/store"
)

func writeLastRun(t *testing.T, dir string, at time.Time) {
	t.Helper()
	secs := float64(at.UnixNano()) / float64(time.Second)
	writeFile(t, filepath.Join(dir, lastRunFile), []byte(strconv.FormatFloat(secs, 'f', -1, 64)))
}

func TestShouldRunAlwaysRunsWhenNoMarkerExists(t *testing.T) {
	dir := t.TempDir()
	require.True(t, ShouldRun(dir, "daily", fixedNow))
	require.True(t, ShouldRun(filepath.Join(dir, "does-not-exist"), "weekly", fixedNow))
}

func TestShouldRunAlwaysIntervalIgnoresTheMarker(t *testing.T) {
	dir := t.TempDir()
	writeLastRun(t, dir, fixedNow)
	require.True(t, ShouldRun(dir, "always", fixedNow))
	require.True(t, ShouldRun(dir, "Always", fixedNow))
}

func TestShouldRunComparesIntervalCaseInsensitivelyAgainstElapsedTime(t *testing.T) {
	cases := []struct {
		interval string
		elapsed  time.Duration
		want     bool
	}{
		{"Hourly", 59 * time.Minute, false},
		{"hourly", time.Hour, false}, // strictly greater than, as `>` in Python
		{"HOURLY", time.Hour + time.Second, true},
		{"Daily", 23 * time.Hour, false},
		{"daily", 24*time.Hour + time.Second, true},
		{"Weekly", 6 * 24 * time.Hour, false},
		{"weekly", 7*24*time.Hour + time.Second, true},
	}
	for _, c := range cases {
		dir := t.TempDir()
		writeLastRun(t, dir, fixedNow.Add(-c.elapsed))
		require.Equal(t, c.want, ShouldRun(dir, c.interval, fixedNow), "%s after %s", c.interval, c.elapsed)
	}
}

func TestShouldRunNeverRunsForAnUnknownIntervalWithAMarker(t *testing.T) {
	// The Python if/elif chain fell through to `return False` for anything
	// but hourly/daily/weekly; a typo in the config silently disabled the
	// plugin after its first run. Kept as-is (spec deviations preamble).
	dir := t.TempDir()
	writeLastRun(t, dir, fixedNow.Add(-365*24*time.Hour))
	require.False(t, ShouldRun(dir, "monthly", fixedNow))
	require.False(t, ShouldRun(dir, "", fixedNow))
}

func TestShouldRunTreatsAnUnparsableMarkerAsDue(t *testing.T) {
	for _, content := range []string{"garbage", "", "nan", "inf", "-inf"} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, lastRunFile), []byte(content))
		require.True(t, ShouldRun(dir, "daily", fixedNow), "content %q", content)
	}
}

func TestShouldRunAcceptsThePythonFloatTextWithSurroundingWhitespace(t *testing.T) {
	dir := t.TempDir()
	secs := float64(fixedNow.Add(-2*time.Hour).Unix()) + 0.123456
	writeFile(t, filepath.Join(dir, lastRunFile), []byte("  "+strconv.FormatFloat(secs, 'f', -1, 64)+"\n"))
	require.True(t, ShouldRun(dir, "hourly", fixedNow))
	require.False(t, ShouldRun(dir, "daily", fixedNow))
}

func TestUpdateLastRunWritesAFloatThatShouldRunReadsBack(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, UpdateLastRun(dir, fixedNow))
	raw, err := os.ReadFile(filepath.Join(dir, lastRunFile))
	require.NoError(t, err)
	secs, err := strconv.ParseFloat(string(raw), 64)
	require.NoError(t, err)
	require.InDelta(t, float64(fixedNow.Unix()), secs, 0.001)

	require.False(t, ShouldRun(dir, "hourly", fixedNow.Add(30*time.Minute)))
	require.True(t, ShouldRun(dir, "hourly", fixedNow.Add(2*time.Hour)))
}

func TestUpdateLastRunFailsWhenTheDirectoryIsMissing(t *testing.T) {
	err := UpdateLastRun(filepath.Join(t.TempDir(), "nope"), fixedNow)
	require.Error(t, err)
}

// touch creates path with the given mtime.
func touch(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	writeFile(t, path, []byte("x"))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

// downloaded writes a file with its own content and records it in history
// as a download, the way a plugin's AddEntry does.
func downloaded(t *testing.T, h *store.History, path string, mtime time.Time) {
	t.Helper()
	writeFile(t, path, []byte("image "+path))
	_, err := h.AddEntry("https://example.com/"+filepath.Base(path), path, "t")
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

func TestCleanupDeletesOnlyRecordedDownloadsOldestFirst(t *testing.T) {
	h, _ := testStores(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "mine-oldest.jpg"), fixedNow.Add(-9*time.Hour))
	touch(t, filepath.Join(dir, lastRunFile), fixedNow.Add(-8*time.Hour))
	downloaded(t, h, filepath.Join(dir, "a.jpg"), fixedNow.Add(-3*time.Hour))
	downloaded(t, h, filepath.Join(dir, "b.jpg"), fixedNow.Add(-2*time.Hour))
	downloaded(t, h, filepath.Join(dir, "c.jpg"), fixedNow.Add(-1*time.Hour))

	cleanupOldFiles(h, dir, "*", 2, "[T]", events.Events{})

	require.Equal(t, []string{".last_run", "b.jpg", "c.jpg", "mine-oldest.jpg"}, dirNames(t, dir))
}

func TestCleanupIgnoresCopiesAndEditsOfDownloads(t *testing.T) {
	h, _ := testStores(t)
	dl, other := t.TempDir(), t.TempDir()
	downloaded(t, h, filepath.Join(dl, "a.jpg"), fixedNow.Add(-2*time.Hour))
	downloaded(t, h, filepath.Join(other, "b.jpg"), fixedNow.Add(-2*time.Hour))
	// A byte-identical copy of a.jpg in another folder, and b.jpg edited.
	writeFile(t, filepath.Join(other, "a-copy.jpg"), []byte("image "+filepath.Join(dl, "a.jpg")))
	writeFile(t, filepath.Join(other, "b.jpg"), []byte("edited"))

	cleanupOldFiles(h, other, "*", 1, "[T]", events.Events{})
	cleanupOldFiles(h, other, "*", 1, "[T]", events.Events{})

	require.Equal(t, []string{"a-copy.jpg", "b.jpg"}, dirNames(t, other))
}

func TestCleanupWithLimitZeroOrLessDeletesNothing(t *testing.T) {
	for _, limit := range []int{0, -1} {
		h, _ := testStores(t)
		dir := t.TempDir()
		downloaded(t, h, filepath.Join(dir, "a.jpg"), fixedNow)
		rec := newRecorder()
		cleanupOldFiles(h, dir, "*", limit, "[T]", rec.events())
		require.Equal(t, []string{"a.jpg"}, dirNames(t, dir))
		require.True(t, rec.hasLogContaining("Retention is off"))
	}
}

func TestResetDirRemovesFilesAndSubdirectoriesButKeepsTheDirectory(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.jpg"), fixedNow)
	touch(t, filepath.Join(dir, lastRunFile), fixedNow)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o750))
	touch(t, filepath.Join(dir, "sub", "deep", "x"), fixedNow)

	resetDir(dir, "[T]", events.Events{})

	require.Empty(t, dirNames(t, dir))
	require.DirExists(t, dir)
}

func TestDirNonEmptyCountsHiddenFilesLikePathlibGlobDid(t *testing.T) {
	dir := t.TempDir()
	require.False(t, dirNonEmpty(dir))
	touch(t, filepath.Join(dir, lastRunFile), fixedNow)
	require.True(t, dirNonEmpty(dir))
}
