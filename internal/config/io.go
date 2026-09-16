package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// saveMu serialises every write this process makes to a config file (DV3).
// The GUI auto-save, the migration persist and the auto_update_logs toggle
// used to race each other in 2.9.x; one lock removes the write storm.
var saveMu sync.Mutex

// Load reads and types the file at path, runs Migrate and, when a migration
// changed something, persists the result and says so on stderr (R2.4). A
// persist failure is logged and the in-memory document is returned: the file
// still has the old key, so the next start retries.
//
// A missing file yields Defaults() and no error.
func Load(path string) (Document, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("read config %s: %w", path, err)
	}
	raw, err := parseRaw(b)
	if err != nil {
		return Document{}, fmt.Errorf("%s: %w", path, err)
	}
	changed := Migrate(raw)
	d, err := fromRaw(raw)
	if err != nil {
		return Document{}, fmt.Errorf("%s: %w", path, err)
	}
	if changed {
		if err := Save(path, d); err != nil {
			fmt.Fprintf(os.Stderr, "[config-migration] Failed to persist migrated config to %s: %v. "+
				"Next startup will retry the migration.\n", path, err)
		} else {
			fmt.Fprintf(os.Stderr, "[config-migration] Persisted migrated config to %s\n", path)
		}
	}
	return d, nil
}

// LoadDefault probes CandidatePaths() in order and loads the first file that
// exists (R2.1). The path used is returned, "" when no candidate exists, in
// which case the document is Defaults().
func LoadDefault() (Document, string, error) {
	for _, p := range CandidatePaths() {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		d, err := Load(p)
		return d, p, err
	}
	return Defaults(), "", nil
}

// Save writes d to path atomically: the bytes go to a temp file in the same
// directory, which is then renamed over the target (R2.3, DV3). The parent
// directory is created when missing, as every Python writer did. Writes are
// serialised process-wide.
func Save(path string, d Document) error {
	b, err := Marshal(d)
	if err != nil {
		return err
	}
	saveMu.Lock()
	defer saveMu.Unlock()

	dir := filepath.Dir(path)
	if err := MkdirAll(dir); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp config %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp config %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp config %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp config %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}
