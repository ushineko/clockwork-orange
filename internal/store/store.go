/*
Package store holds the two SQLite files the plugins share (spec 010 R5.5,
R5.6): history.db, which stops a URL or an image being downloaded twice, and
blacklist.db, which remembers images the user rejected.

Both files are opened with modernc.org/sqlite (D5: pure Go, CGO-free) and use
the exact DDL plugins/history.py and plugins/blacklist.py issued, whitespace
included, so a database written by either implementation reads identically in
the other (R5.7). The golden tests under tests/golden/db pin that.
*/
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// driverName is the name modernc.org/sqlite registers with database/sql.
const driverName = "sqlite"

// Now is the clock the stores stamp rows with; tests replace it.
var Now = time.Now

// open connects to the SQLite file at path and runs the schema statements.
//
// One connection at a time: Python opened a fresh connection per call and
// modernc's driver is happiest with a single writer, so the pool is capped at
// one, which also serialises VACUUM against everything else.
func open(path string, schema ...string) (*sql.DB, error) {
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range schema {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("init schema in %s: %w", path, err)
		}
	}
	return db, nil
}

// closeDB wraps db.Close with the file name for context.
func closeDB(db *sql.DB, path string) error {
	if err := db.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

// timestamp is Python's datetime.now().timestamp(): float seconds since the
// epoch, taken from the injectable clock.
func timestamp() float64 {
	return float64(Now().UnixNano()) / 1e9
}

// isConstraintViolation reports whether err is what Python surfaced as
// sqlite3.IntegrityError: any SQLITE_CONSTRAINT result, of which the UNIQUE
// extended code is the one the downloads table can raise.
func isConstraintViolation(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	code := se.Code()
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code&0xff == sqlite3.SQLITE_CONSTRAINT
}

// dateFor formats a float timestamp the way plugins/blacklist.py did
// (datetime.fromtimestamp(ts).strftime("%Y-%m-%d %H:%M")); a zero timestamp
// (Python: falsy) is "Unknown". loc is time.Local in production.
func dateFor(ts float64, loc *time.Location) string {
	if ts == 0 {
		return "Unknown"
	}
	sec := math.Floor(ts)
	nsec := (ts - sec) * 1e9
	return time.Unix(int64(sec), int64(nsec)).In(loc).Format("2006-01-02 15:04")
}
