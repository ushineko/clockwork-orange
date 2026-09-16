package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// goldenExpected mirrors tests/golden/db/expected.json.
type goldenExpected struct {
	Blacklist struct {
		IsBlacklisted map[string]bool `json:"is_blacklisted"`
		Items         []struct {
			Date         string  `json:"date"`
			HasThumbnail bool    `json:"has_thumbnail"`
			Hash         string  `json:"hash"`
			Source       string  `json:"source"`
			Timestamp    float64 `json:"timestamp"`
		} `json:"items"`
	} `json:"blacklist"`
	History struct {
		SeenImage map[string]bool `json:"seen_image"`
		SeenURL   map[string]bool `json:"seen_url"`
		Stats     struct {
			TotalRecords int `json:"total_records"`
			UniqueImages int `json:"unique_images"`
		} `json:"stats"`
	} `json:"history"`
}

func loadExpected(t *testing.T) goldenExpected {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "tests", "golden", "db", "expected.json"))
	require.NoError(t, err)
	var e goldenExpected
	require.NoError(t, json.Unmarshal(raw, &e))
	return e
}

// useUTC makes Items() render Date in UTC, which is how expected.json was
// captured (TZ=UTC). Restored on cleanup.
func useUTC(t *testing.T) {
	t.Helper()
	prev := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = prev })
}

func TestPythonWrittenHistoryDBReadsIdenticallyFromGo(t *testing.T) {
	want := loadExpected(t).History
	h, err := OpenHistory(copyGoldenDB(t, "history.db"))
	require.NoError(t, err)
	defer func() { _ = h.Close() }()

	stats, err := h.Stats()
	require.NoError(t, err)
	require.Equal(t, want.Stats.TotalRecords, stats.TotalRecords)
	require.Equal(t, want.Stats.UniqueImages, stats.UniqueImages)
	require.Positive(t, stats.DBSizeBytes)

	for url, seen := range want.SeenURL {
		got, err := h.SeenURL(url)
		require.NoError(t, err, url)
		require.Equal(t, seen, got, url)
	}
	for name, seen := range want.SeenImage {
		got, err := h.SeenImage(goldenImage(t, name))
		require.NoError(t, err, name)
		require.Equal(t, seen, got, name)
	}
}

func TestPythonWrittenBlacklistDBReadsIdenticallyFromGo(t *testing.T) {
	useUTC(t)
	want := loadExpected(t).Blacklist
	b, err := OpenBlacklist(copyGoldenDB(t, "blacklist.db"))
	require.NoError(t, err)
	defer func() { _ = b.Close() }()

	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, len(want.Items))
	for i, w := range want.Items {
		got := items[i]
		require.Equal(t, w.Hash, got.Hash, i)
		require.Equal(t, w.Source, got.Source, i)
		require.InDelta(t, w.Timestamp, got.Timestamp, 1e-9, i)
		require.Equal(t, w.Date, got.Date, i)
		require.Equal(t, w.HasThumbnail, got.Thumbnail != nil, i)
	}
	for hash, blacklisted := range want.IsBlacklisted {
		require.Equal(t, blacklisted, b.IsBlacklisted(hash), hash)
	}
}

// schemaOf returns every sqlite_master row (NULL sql included, for the
// autoindex) so the comparison covers whitespace, index and sequence table.
func schemaOf(t *testing.T, path string) []string {
	t.Helper()
	db, err := sql.Open(driverName, path)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(context.Background(), `SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY type, name`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var typ, name, tbl string
		var ddl sql.NullString
		require.NoError(t, rows.Scan(&typ, &name, &tbl, &ddl))
		out = append(out, typ+"|"+name+"|"+tbl+"|"+ddl.String)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, out)
	return out
}

func TestGoWrittenHistoryDBHasByteIdenticalSchemaToPython(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	h, err := OpenHistory(path)
	require.NoError(t, err)
	// sqlite_sequence appears on the first AUTOINCREMENT insert, as it did
	// for the Python fixture.
	_, err = h.AddEntry("https://example.com/a.jpg", goldenImage(t, "landscape_1920x1080.png"), "wallhaven")
	require.NoError(t, err)
	require.NoError(t, h.Close())
	require.Equal(t, schemaOf(t, copyGoldenDB(t, "history.db")), schemaOf(t, path))
}

func TestGoWrittenBlacklistDBHasByteIdenticalSchemaToPython(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blacklist.db")
	b, err := OpenBlacklist(path)
	require.NoError(t, err)
	require.NoError(t, b.Add("h", "x", ""))
	require.NoError(t, b.Close())
	require.Equal(t, schemaOf(t, copyGoldenDB(t, "blacklist.db")), schemaOf(t, path))
}

func TestGoCanReplayTheFixtureCaptureAndReachTheSameState(t *testing.T) {
	// Re-run capture_dbs from tests/golden/capture.py against a fresh Go DB
	// and expect the very numbers Python reported.
	useUTC(t)
	fixedClock(t, 1700000000.5)
	want := loadExpected(t)

	h, err := OpenHistory(filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	defer func() { _ = h.Close() }()
	landscape := goldenImage(t, "landscape_1920x1080.png")
	for _, c := range []struct {
		url, img, src string
		added         bool
	}{
		{"https://example.com/a.jpg", landscape, "wallhaven", true},
		{"https://example.com/b.jpg", goldenImage(t, "portrait_1080x1920.png"), "duckduckgo_images", true},
		{"https://example.com/c.jpg", landscape, "imported", true},
		{"https://example.com/a.jpg", goldenImage(t, "square_600x600.png"), "dup", false},
	} {
		added, err := h.AddEntry(c.url, c.img, c.src)
		require.NoError(t, err, c.url)
		require.Equal(t, c.added, added, c.url)
	}
	stats, err := h.Stats()
	require.NoError(t, err)
	require.Equal(t, want.History.Stats.TotalRecords, stats.TotalRecords)
	require.Equal(t, want.History.Stats.UniqueImages, stats.UniqueImages)

	b, err := OpenBlacklist(filepath.Join(t.TempDir(), "blacklist.db"))
	require.NoError(t, err)
	defer func() { _ = b.Close() }()
	require.NoError(t, b.Add("", "local", goldenImage(t, "small_800x600.jpg")))
	require.NoError(t, b.Add("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "manual", ""))
	items, err := b.Items()
	require.NoError(t, err)
	require.Len(t, items, len(want.Blacklist.Items))
	for i, w := range want.Blacklist.Items {
		require.Equal(t, w.Hash, items[i].Hash, i)
		require.Equal(t, w.Source, items[i].Source, i)
		require.Equal(t, w.Date, items[i].Date, i)
		require.InDelta(t, w.Timestamp, items[i].Timestamp, 1e-6, i)
		require.Equal(t, w.HasThumbnail, items[i].Thumbnail != nil, i)
	}
}
