package store

import (
	"context"
	"crypto/md5" //nolint:gosec // G501: url_hash is md5(url) in the Python schema (R5.5); a lookup key, not security.
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// HistoryFileName is the basename of the download history database.
const HistoryFileName = "history.db"

// historySchema is the DDL from plugins/history.py. The indentation is part
// of the contract: SQLite stores CREATE statements verbatim in sqlite_master,
// and the round-trip test compares that text against the Python fixture.
var historySchema = []string{
	`CREATE TABLE IF NOT EXISTS downloads (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    url_hash TEXT,
                    image_hash TEXT,
                    source TEXT,
                    timestamp REAL,
                    UNIQUE(url_hash)
                )`,
	`CREATE INDEX IF NOT EXISTS idx_image_hash ON downloads(image_hash)`,
}

// History is the download-history store (R5.5): the port of HistoryManager.
type History struct {
	path string
	db   *sql.DB
}

// OpenHistory opens (creating the schema if absent) the history database at
// path. The parent directory must exist, as with HistoryManager(db_path=...).
func OpenHistory(path string) (*History, error) {
	db, err := open(path, historySchema...)
	if err != nil {
		return nil, err
	}
	return &History{path: path, db: db}, nil
}

// OpenDefaultHistory opens config.StateDir()/history.db, creating the
// directory first (R5.5).
func OpenDefaultHistory() (*History, error) {
	dir := config.StateDir()
	if err := config.MkdirAll(dir); err != nil {
		return nil, err
	}
	return OpenHistory(filepath.Join(dir, HistoryFileName))
}

// Close releases the database.
func (h *History) Close() error { return closeDB(h.db, h.path) }

// urlHash is md5 of the URL text, the url_hash column (R5.5).
func urlHash(url string) string {
	sum := md5.Sum([]byte(url)) //nolint:gosec // G401: schema-mandated lookup key, see import note.
	return hex.EncodeToString(sum[:])
}

// SeenURL reports whether url was recorded before.
func (h *History) SeenURL(url string) (bool, error) {
	return h.exists(`SELECT 1 FROM downloads WHERE url_hash = ?`, urlHash(url))
}

// SeenImage reports whether a file with the same bytes was recorded before.
// A missing file is simply unseen (false, nil), as in Python.
func (h *History) SeenImage(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	imageHash, err := imaging.MD5File(path)
	if err != nil {
		return false, err
	}
	return h.exists(`SELECT 1 FROM downloads WHERE image_hash = ?`, imageHash)
}

// exists runs a SELECT 1 lookup.
func (h *History) exists(query string, arg any) (bool, error) {
	var one int
	err := h.db.QueryRowContext(context.Background(), query, arg).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("query history: %w", err)
	}
	return true, nil
}

// AddEntry records a successful download. It returns (false, nil) when the
// URL was already recorded (the UNIQUE(url_hash) constraint), matching
// add_entry's IntegrityError branch.
func (h *History) AddEntry(url, path, source string) (bool, error) {
	imageHash, err := imaging.MD5File(path)
	if err != nil {
		return false, err
	}
	_, err = h.db.ExecContext(context.Background(),
		`INSERT INTO downloads (url_hash, image_hash, source, timestamp) VALUES (?, ?, ?, ?)`,
		urlHash(url), imageHash, source, timestamp())
	if err != nil {
		if isConstraintViolation(err) {
			return false, nil
		}
		return false, fmt.Errorf("insert history entry: %w", err)
	}
	return true, nil
}

// HistoryStats is get_stats(): row count, distinct image hashes and the file
// size on disk (0 when the file cannot be stat'ed).
type HistoryStats struct {
	TotalRecords int
	UniqueImages int
	DBSizeBytes  int64
}

// Stats reports counts and file size.
func (h *History) Stats() (HistoryStats, error) {
	var s HistoryStats
	if fi, err := os.Stat(h.path); err == nil {
		s.DBSizeBytes = fi.Size()
	}
	ctx := context.Background()
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM downloads`).Scan(&s.TotalRecords); err != nil {
		return HistoryStats{}, fmt.Errorf("count history: %w", err)
	}
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT image_hash) FROM downloads`).Scan(&s.UniqueImages); err != nil {
		return HistoryStats{}, fmt.Errorf("count distinct images: %w", err)
	}
	return s, nil
}

// Clear deletes every row and then VACUUMs so the file shrinks, as
// clear_history did.
func (h *History) Clear() error {
	ctx := context.Background()
	if _, err := h.db.ExecContext(ctx, `DELETE FROM downloads`); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}
	if _, err := h.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("vacuum history: %w", err)
	}
	return nil
}
