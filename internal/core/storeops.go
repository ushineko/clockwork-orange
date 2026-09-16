package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// BlacklistList returns every blacklisted image, newest first (R7.8).
func BlacklistList(_ context.Context, req Request) ([]store.BlacklistItem, error) {
	d := req.deps()
	if err := d.ensureStores(); err != nil {
		return nil, err
	}
	return d.Blacklist.Items()
}

// BlacklistRemoveRequest names hashes to unblock.
type BlacklistRemoveRequest struct {
	Request
	Hashes []string
}

// BlacklistRemove removes each hash; the first error aborts.
func BlacklistRemove(_ context.Context, req BlacklistRemoveRequest) error {
	d := req.deps()
	if err := d.ensureStores(); err != nil {
		return err
	}
	for _, h := range req.Hashes {
		if err := d.Blacklist.Remove(h); err != nil {
			return err
		}
	}
	return nil
}

// BlacklistAddRequest blacklists files and deletes them (process_files).
type BlacklistAddRequest struct {
	Request
	Paths []string
	// Plugin is recorded as the source; "manual" when empty.
	Plugin string
}

// BlacklistAdd is the manual review outcome: hash, thumbnail, store, remove.
func BlacklistAdd(_ context.Context, req BlacklistAddRequest) (int, error) {
	d := req.deps()
	if err := d.ensureStores(); err != nil {
		return 0, err
	}
	if req.Plugin == "" {
		req.Plugin = "manual"
	}
	return d.Blacklist.ProcessFiles(req.Paths, req.Plugin, req.Events)
}

// HistoryStats returns the download history statistics (R7.7).
func HistoryStats(_ context.Context, req Request) (store.HistoryStats, error) {
	d := req.deps()
	if err := d.ensureStores(); err != nil {
		return store.HistoryStats{}, err
	}
	return d.History.Stats()
}

// HistoryClear empties and vacuums the history database.
func HistoryClear(_ context.Context, req Request) error {
	d := req.deps()
	if err := d.ensureStores(); err != nil {
		return err
	}
	return d.History.Clear()
}

// HistoryImportRequest scans a directory of JPEGs into the history.
type HistoryImportRequest struct {
	Request
	// Dir defaults to plugins.duckduckgo_images.download_dir, then
	// ~/Pictures/Wallpapers/DuckDuckGo (R7.7).
	Dir string
	// OnProgress, when set, receives (done, total) per file.
	OnProgress func(done, total int)
}

// HistoryImportResult counts what the scan did.
type HistoryImportResult struct {
	Dir      string
	Imported int
	Skipped  int
}

// HistoryImport inserts every `*.jp*g` file as `file://imported/<name>` with
// source "imported"; duplicates count as skipped.
func HistoryImport(ctx context.Context, req HistoryImportRequest) (HistoryImportResult, error) {
	d := req.deps()
	if err := d.ensureStores(); err != nil {
		return HistoryImportResult{}, err
	}
	dir := req.Dir
	if dir == "" {
		loaded, err := LoadConfig(ctx, req.Request)
		if err != nil {
			return HistoryImportResult{}, err
		}
		if block := loaded.Doc.Plugins["duckduckgo_images"]; block != nil {
			if s, ok := block["download_dir"].(string); ok && s != "" {
				dir = s
			}
		}
		if dir == "" {
			dir = filepath.Join(config.HomeDir(), "Pictures", "Wallpapers", "DuckDuckGo")
		}
	}
	dir = config.ExpandPath(dir)
	res := HistoryImportResult{Dir: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		// Python glob `*.[jJ][pP]*[gG]`: .jpg, .jpeg, .jpXg ...
		if ext := filepath.Ext(lower); strings.HasPrefix(ext, ".jp") && strings.HasSuffix(ext, "g") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return res, fmt.Errorf("import interrupted: %w", err)
		}
		added, err := d.History.AddEntry("file://imported/"+filepath.Base(f), f, "imported")
		switch {
		case err != nil:
			req.Events.Warnf("Import %s: %v", f, err)
			res.Skipped++
		case added:
			res.Imported++
		default:
			res.Skipped++
		}
		if req.OnProgress != nil {
			req.OnProgress(i+1, len(files))
		}
	}
	return res, nil
}
