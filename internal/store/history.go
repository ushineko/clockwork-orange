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
const HistoryFileName = "history2.db"

// LegacyHistoryFileName is the 2.9.x/4.x history database (no paths), which
// OpenDefaultHistory copies into HistoryFileName once and then leaves alone.
const LegacyHistoryFileName = "history.db"

// historySchema is plugins/history.py's table plus where each download was
// saved (spec 023): retention may only delete a file whose exact path is
// recorded here. Rows migrated from history.db have no path until a
// retention pass adopts them (adopted_dirs records which folders were).
var historySchema = []string{
	`CREATE TABLE IF NOT EXISTS downloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url_hash TEXT,
		image_hash TEXT,
		source TEXT,
		timestamp REAL,
		path TEXT,
		UNIQUE(url_hash)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_image_hash ON downloads(image_hash)`,
	`CREATE INDEX IF NOT EXISTS idx_path ON downloads(path)`,
	`CREATE TABLE IF NOT EXISTS adopted_dirs (dir TEXT PRIMARY KEY, timestamp REAL)`,
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

// OpenDefaultHistory opens config.StateDir()/history2.db, creating the
// directory first (R5.5) and migrating history.db into it the first time.
func OpenDefaultHistory() (*History, error) {
	dir := config.StateDir()
	if err := config.MkdirAll(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, HistoryFileName)
	if err := migrateLegacyHistory(filepath.Join(dir, LegacyHistoryFileName), path); err != nil {
		return nil, err
	}
	return OpenHistory(path)
}

// migrateLegacyHistory copies every row of the legacy database into a new
// history2.db with no path (spec 023). It runs only when history2.db does not
// exist and history.db does; the copy is built under a temporary name and
// renamed, so an interrupted migration is simply redone next time.
func migrateLegacyHistory(legacy, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if _, err := os.Stat(legacy); err != nil {
		return nil //nolint:nilerr // no legacy database: nothing to migrate
	}
	tmp := path + ".migrating"
	_ = os.Remove(tmp)
	h, err := OpenHistory(tmp)
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, err = h.db.ExecContext(ctx, `ATTACH DATABASE ? AS legacy`, legacy)
	if err == nil {
		_, err = h.db.ExecContext(ctx, `INSERT OR IGNORE INTO downloads (url_hash, image_hash, source, timestamp)
			SELECT url_hash, image_hash, source, timestamp FROM legacy.downloads ORDER BY id`)
	}
	if err == nil {
		_, err = h.db.ExecContext(ctx, `DETACH DATABASE legacy`)
	}
	if cerr := h.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("migrate %s: %w", legacy, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("migrate %s: %w", legacy, err)
	}
	return nil
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

// AddEntry records a successful download and where it was saved. It returns
// (false, nil) when the URL was already recorded (the UNIQUE(url_hash)
// constraint), matching add_entry's IntegrityError branch.
func (h *History) AddEntry(url, path, source string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve %s: %w", path, err)
	}
	return h.add(url, path, source, abs)
}

// ImportedSource is the source of rows `history import` writes. Those files
// were the user's before the app saw them, so they are recorded without a
// path and never adopted: retention never deletes them (spec 023).
const ImportedSource = "imported"

// AddImported records an existing file so plugins skip its URL and content,
// without making it eligible for retention.
func (h *History) AddImported(url, path string) (bool, error) {
	return h.add(url, path, ImportedSource, nil)
}

// add inserts one row; savedAt is the absolute path, or nil for none.
func (h *History) add(url, path, source string, savedAt any) (bool, error) {
	imageHash, err := imaging.MD5File(path)
	if err != nil {
		return false, err
	}
	_, err = h.db.ExecContext(context.Background(),
		`INSERT INTO downloads (url_hash, image_hash, source, timestamp, path) VALUES (?, ?, ?, ?, ?)`,
		urlHash(url), imageHash, source, timestamp(), savedAt)
	if err != nil {
		if isConstraintViolation(err) {
			return false, nil
		}
		return false, fmt.Errorf("insert history entry: %w", err)
	}
	return true, nil
}

// Downloaded reports whether the file at path is one the app downloaded and
// that is still as it was saved: its absolute path is recorded and its bytes
// hash to the recorded image. Retention deletes nothing else (spec 023), so a
// copy of a download elsewhere, or an edited one, is never eligible.
func (h *History) Downloaded(path string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve %s: %w", path, err)
	}
	var want string
	err = h.db.QueryRowContext(context.Background(),
		`SELECT image_hash FROM downloads WHERE path = ? ORDER BY id DESC LIMIT 1`, abs).Scan(&want)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("query history: %w", err)
	}
	got, err := imaging.MD5File(abs)
	if err != nil {
		return false, nil //nolint:nilerr // unreadable or gone: not deletable
	}
	return got == want, nil
}

// Adopt grandfathers the files already in dir into retention (spec 023): the
// first time a folder is seen, each regular file whose content matches a
// download recorded without a path (migrated from history.db) takes that
// row's path. Imported rows are not downloads and are never adopted. Each row is adopted at most once, and a folder only once, so a
// later copy of a download does not become deletable. It returns how many
// files were adopted.
func (h *History) Adopt(dir string) (int, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, fmt.Errorf("resolve %s: %w", dir, err)
	}
	ctx := context.Background()
	done, err := h.exists(`SELECT 1 FROM adopted_dirs WHERE dir = ?`, abs)
	if err != nil || done {
		return 0, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", abs, err)
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("adopt %s: %w", abs, err)
	}
	defer func() { _ = tx.Rollback() }()
	adopted := 0
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		file := filepath.Join(abs, e.Name())
		sum, err := imaging.MD5File(file)
		if err != nil {
			continue
		}
		res, err := tx.ExecContext(ctx, `UPDATE downloads SET path = ? WHERE id =
			(SELECT id FROM downloads WHERE image_hash = ? AND path IS NULL AND source != ? ORDER BY id LIMIT 1)`,
			file, sum, ImportedSource)
		if err != nil {
			return 0, fmt.Errorf("adopt %s: %w", file, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			adopted++
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO adopted_dirs (dir, timestamp) VALUES (?, ?)`, abs, timestamp()); err != nil {
		return 0, fmt.Errorf("adopt %s: %w", abs, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("adopt %s: %w", abs, err)
	}
	return adopted, nil
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
	if _, err := h.db.ExecContext(ctx, `DELETE FROM adopted_dirs`); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}
	if _, err := h.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("vacuum history: %w", err)
	}
	return nil
}
