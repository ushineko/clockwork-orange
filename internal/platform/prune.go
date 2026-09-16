package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// This file is OS-independent so the prune decision is unit-tested on Linux
// with a FakeRunner standing in for `du` (spec 010 Phase 3 AC); only the
// darwin Platform calls it.

// MacWallpaperCacheMaxMB is the threshold above which the macOS wallpaper
// cache is emptied (R4.10).
const MacWallpaperCacheMaxMB = 500

// wallpaperCacheDir is the directory macOS's wallpaper agent fills and never
// prunes; a variable so tests point it at a temp dir.
var wallpaperCacheDir = defaultWallpaperCacheDir()

func defaultWallpaperCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Containers", "com.apple.wallpaper.agent",
		"Data", "Library", "Caches", "com.apple.wallpaper.caches")
}

const duTimeout = 10 * time.Second

// PruneWallpaperCache empties the macOS wallpaper cache when `du -sk` reports
// more than maxMB (R4.10). macOS regenerates the current wallpaper's entry
// immediately, so deleting the contents is safe. Nothing here is fatal to a
// wallpaper set: the returned error is for the caller to log as a warning,
// and entries that fail to delete are skipped and reported together.
func PruneWallpaperCache(ctx context.Context, r Runner, maxMB int) error {
	dir := wallpaperCacheDir
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, duTimeout)
	defer cancel()
	stdout, _, err := r.Run(ctx, "du", "-sk", dir)
	if err != nil {
		return fmt.Errorf("du -sk %s: %w", dir, err)
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return fmt.Errorf("du -sk %s: empty output", dir)
	}
	sizeKB, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return fmt.Errorf("du -sk %s: parse %q: %w", dir, fields[0], err)
	}
	if sizeKB <= int64(maxMB)*1024 {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}
	var failures []error
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			failures = append(failures, fmt.Errorf("remove cache entry %s: %w", e.Name(), err))
		}
	}
	return errors.Join(failures...)
}
