package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// BlacklistFileName is the basename of the blacklist database.
const BlacklistFileName = "blacklist.db"

// Thumbnail parameters from plugins/blacklist.py (R5.6).
const (
	ThumbnailSide    = 128
	ThumbnailQuality = 70
)

// ErrNoHash is returned by Add when neither a hash nor an existing file was
// given; Python printed "Cannot blacklist: no hash or file provided." and
// returned silently.
var ErrNoHash = errors.New("cannot blacklist: no hash or file provided")

// blacklistSchema is the DDL from plugins/blacklist.py, whitespace included
// (see historySchema for why).
var blacklistSchema = []string{
	`CREATE TABLE IF NOT EXISTS blacklist (
                    img_hash TEXT PRIMARY KEY,
                    source TEXT,
                    timestamp REAL,
                    thumbnail BLOB
                )`,
}

// Blacklist is the rejected-image store (R5.6): the port of BlacklistManager.
type Blacklist struct {
	path string
	db   *sql.DB
}

// OpenBlacklist opens (creating the schema if absent) the blacklist database
// at path, which names the .db file itself, not its directory.
func OpenBlacklist(path string) (*Blacklist, error) {
	db, err := open(path, blacklistSchema...)
	if err != nil {
		return nil, err
	}
	return &Blacklist{path: path, db: db}, nil
}

// OpenDefaultBlacklist opens config.StateDir()/blacklist.db, creating the
// directory first (R5.6).
func OpenDefaultBlacklist() (*Blacklist, error) {
	dir := config.StateDir()
	if err := config.MkdirAll(dir); err != nil {
		return nil, err
	}
	return OpenBlacklist(filepath.Join(dir, BlacklistFileName))
}

// Close releases the database.
func (b *Blacklist) Close() error { return closeDB(b.db, b.path) }

/*
Add records an image as rejected (add_to_blacklist). When filePath names an
existing file, hash is computed from it if empty and a thumbnail is stored;
with only a hash the thumbnail is NULL. INSERT OR REPLACE, so re-adding
refreshes source, timestamp and thumbnail.

A thumbnail that cannot be generated is stored as NULL rather than failing
the add, as in Python: the point of the row is the hash.
*/
func (b *Blacklist) Add(hash, plugin, filePath string) error {
	var thumb []byte
	if filePath != "" {
		if _, err := os.Stat(filePath); err == nil {
			if hash == "" {
				hash, err = imaging.SHA256File(filePath)
				if err != nil {
					return err
				}
			}
			thumb, _ = imaging.Thumbnail(filePath, ThumbnailSide, ThumbnailQuality)
		}
	}
	if hash == "" {
		return ErrNoHash
	}
	_, err := b.db.ExecContext(context.Background(),
		`INSERT OR REPLACE INTO blacklist (img_hash, source, timestamp, thumbnail) VALUES (?, ?, ?, ?)`,
		hash, plugin, timestamp(), thumb)
	if err != nil {
		return fmt.Errorf("insert blacklist entry: %w", err)
	}
	return nil
}

// Remove deletes hash from the blacklist; removing an unknown hash is not an
// error.
func (b *Blacklist) Remove(hash string) error {
	if _, err := b.db.ExecContext(context.Background(), `DELETE FROM blacklist WHERE img_hash = ?`, hash); err != nil {
		return fmt.Errorf("delete blacklist entry: %w", err)
	}
	return nil
}

// IsBlacklisted reports whether hash is present. Any error -- including a
// closed database -- reads as false, as in Python: a broken blacklist must
// never stop a download.
func (b *Blacklist) IsBlacklisted(hash string) bool {
	var one int
	err := b.db.QueryRowContext(context.Background(), `SELECT 1 FROM blacklist WHERE img_hash = ?`, hash).Scan(&one)
	return err == nil
}

// BlacklistItem is one row as get_blacklist_items reported it. Thumbnail is
// nil when the column is NULL; Date is Timestamp in local time as
// "2006-01-02 15:04", or "Unknown" for a zero/NULL timestamp.
type BlacklistItem struct {
	Hash      string
	Source    string
	Timestamp float64
	Thumbnail []byte
	Date      string
}

// Items returns every row, newest first (ORDER BY timestamp DESC).
func (b *Blacklist) Items() ([]BlacklistItem, error) {
	return b.items(time.Local)
}

// items is Items with the location Date is rendered in.
func (b *Blacklist) items(loc *time.Location) ([]BlacklistItem, error) {
	rows, err := b.db.QueryContext(context.Background(),
		`SELECT img_hash, source, timestamp, thumbnail FROM blacklist ORDER BY timestamp DESC`)
	if err != nil {
		return nil, fmt.Errorf("query blacklist: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BlacklistItem
	for rows.Next() {
		var (
			it     BlacklistItem
			source sql.NullString
			ts     sql.NullFloat64
		)
		if err := rows.Scan(&it.Hash, &source, &ts, &it.Thumbnail); err != nil {
			return nil, fmt.Errorf("scan blacklist row: %w", err)
		}
		it.Source = source.String
		it.Timestamp = ts.Float64
		it.Date = dateFor(it.Timestamp, loc)
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blacklist: %w", err)
	}
	return out, nil
}

/*
ProcessFiles blacklists each existing file (hash and thumbnail from its
contents, source = plugin) and then deletes it, logging
"Blacklisted and removed: <name>" per file (process_files). It returns the
number of files removed.

Missing paths are logged and skipped. Unlike Python, a failed Add stops the
run before the file is deleted: silently discarding an image that was never
recorded would defeat the blacklist.
*/
func (b *Blacklist) ProcessFiles(paths []string, plugin string, ev events.Events) (int, error) {
	removed := 0
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			ev.Warnf("File not found: %s", p)
			continue
		}
		if err := b.Add("", plugin, p); err != nil {
			return removed, fmt.Errorf("blacklist %s: %w", p, err)
		}
		if err := os.Remove(p); err != nil {
			ev.Errorf("Error removing file %s: %v", p, err)
			continue
		}
		removed++
		ev.Infof("Blacklisted and removed: %s", filepath.Base(p))
	}
	return removed, nil
}
