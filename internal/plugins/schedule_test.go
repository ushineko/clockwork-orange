package plugins

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
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

func TestCleanupWithStarPatternEvictsOldestFilesIncludingTheLastRunMarker(t *testing.T) {
	// Wallhaven's retention iterates every file, so .last_run competes with
	// the wallpapers and is deleted when it is among the oldest. Ported as-is.
	dir := t.TempDir()
	base := fixedNow
	touch(t, filepath.Join(dir, lastRunFile), base.Add(-4*time.Hour))
	touch(t, filepath.Join(dir, "a.jpg"), base.Add(-3*time.Hour))
	touch(t, filepath.Join(dir, "b.jpg"), base.Add(-2*time.Hour))
	touch(t, filepath.Join(dir, "c.jpg"), base.Add(-1*time.Hour))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o750))

	cleanupOldFiles(dir, "*", 2, "[T]", events.Events{})

	require.Equal(t, []string{"b.jpg", "c.jpg", "sub"}, dirNames(t, dir), "directories are not counted or removed")
}

func TestCleanupWithJpgPatternLeavesOtherFilesAlone(t *testing.T) {
	dir := t.TempDir()
	base := fixedNow
	touch(t, filepath.Join(dir, lastRunFile), base.Add(-5*time.Hour))
	touch(t, filepath.Join(dir, "old.jpg"), base.Add(-4*time.Hour))
	touch(t, filepath.Join(dir, "keep.png"), base.Add(-3*time.Hour))
	touch(t, filepath.Join(dir, "new.jpg"), base.Add(-1*time.Hour))

	rec := newRecorder()
	cleanupOldFiles(dir, "*.jpg", 1, "[T]", rec.events())

	require.Equal(t, []string{".last_run", "keep.png", "new.jpg"}, dirNames(t, dir))
	require.True(t, rec.hasLogContaining("Removed old.jpg"))
}

func TestCleanupDoesNothingAtOrBelowTheLimit(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.jpg"), fixedNow)
	touch(t, filepath.Join(dir, "b.jpg"), fixedNow)
	cleanupOldFiles(dir, "*", 2, "[T]", events.Events{})
	require.Len(t, dirNames(t, dir), 2)
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
