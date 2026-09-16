package plugins

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// lastRunFile is the marker the download plugins keep in their download
// directory (R5.3).
const lastRunFile = ".last_run"

/*
ShouldRun decides whether a scheduled download plugin is due (R5.3). It is
the _should_run method shared by wallhaven.py and duckduckgo_images.py:

  - "always" runs every time;
  - no .last_run in dir runs;
  - a .last_run that does not parse as a float, or holds NaN/Inf, runs
    (Python's float()/fromtimestamp raised and the except branch returned
    True);
  - hourly/daily/weekly run when strictly more than 1 h / 1 d / 7 d have
    passed since the stored timestamp;
  - any other interval never runs — the Python method fell off the end of its
    if/elif chain and returned False. Kept as-is per the spec's deviations
    preamble.

interval is compared lower-cased so "Daily" (the schema default) works.
*/
func ShouldRun(dir string, interval string, now time.Time) bool {
	interval = strings.ToLower(interval)
	if interval == "always" {
		return true
	}
	raw, err := os.ReadFile(filepath.Join(dir, lastRunFile))
	if err != nil {
		return true
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil || math.IsNaN(secs) || math.IsInf(secs, 0) {
		return true
	}
	lastRun := time.Unix(0, int64(secs*float64(time.Second)))
	elapsed := now.Sub(lastRun)
	switch interval {
	case "hourly":
		return elapsed > time.Hour
	case "daily":
		return elapsed > 24*time.Hour
	case "weekly":
		return elapsed > 7*24*time.Hour
	}
	return false
}

// UpdateLastRun writes now to dir/.last_run as a float number of seconds
// since the epoch, the format Python's str(time.time()) produced and
// float() reads back (R5.3).
func UpdateLastRun(dir string, now time.Time) error {
	secs := float64(now.UnixNano()) / float64(time.Second)
	text := strconv.FormatFloat(secs, 'f', -1, 64)
	path := filepath.Join(dir, lastRunFile)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

/*
cleanupOldFiles enforces the retention limit (R5.3, R5.4): the regular files
in dir matching pattern are sorted by modification time and the oldest
len-maxFiles are removed. Errors are logged and otherwise ignored, as the
Python `except: pass` did.

Wallhaven passes "*", which — like pathlib's iterdir — includes .last_run, so
the marker itself can be evicted; DuckDuckGo passes "*.jpg". Both quirks are
ported as-is.
*/
func cleanupOldFiles(dir, pattern string, maxFiles int, logPrefix string, ev events.Events) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		ev.Warnf("%s Cleanup failed: %v", logPrefix, err)
		return
	}
	type entry struct {
		path  string
		mtime time.Time
	}
	files := make([]entry, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, entry{path: m, mtime: info.ModTime()})
	}
	if len(files) <= maxFiles {
		return
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })
	toRemove := len(files) - maxFiles
	if toRemove > len(files) {
		toRemove = len(files)
	}
	ev.Infof("%s Cleaning up %d old images...", logPrefix, toRemove)
	for _, f := range files[:toRemove] {
		if err := os.Remove(f.path); err != nil {
			ev.Warnf("%s Cleanup failed: %v", logPrefix, err)
			return
		}
		ev.Infof("%s Removed %s", logPrefix, filepath.Base(f.path))
	}
}

// resetDir empties dir (the `reset` runtime key, R5.3/R5.4): files are
// unlinked and sub-directories removed recursively. dir itself is kept.
// Failures are logged; the Python method aborted at the first error, the
// port keeps going so one stubborn entry does not shield the rest.
func resetDir(dir, logPrefix string, ev events.Events) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		ev.Warnf("%s Reset failed: %v", logPrefix, err)
		return
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			ev.Warnf("%s Reset failed: %v", logPrefix, err)
		}
	}
}

// dirNonEmpty reports whether dir has any entry at all, hidden files
// included — pathlib's glob("*") matched dot-files (R5.4).
func dirNonEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}
