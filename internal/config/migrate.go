package config

import "path/filepath"

// oldGoogleImagesDefaultDir is where the retired google_images plugin saved
// its downloads when the user never set download_dir.
func oldGoogleImagesDefaultDir() string {
	return filepath.Join(HomeDir(), "Pictures", "Wallpapers", "GoogleImages")
}

// migration mutates the decoded mapping and reports whether it changed
// anything. Every migration is idempotent: a second run after the change has
// landed is a no-op, which is what lets Load run the whole list on every start.
type migration func(raw map[string]any) bool

var migrations = []migration{
	migrateGoogleToDuckDuckGo,
}

// Migrate runs every registered migration over the decoded top-level mapping
// and reports whether any of them mutated it (R2.4). Callers persist the
// mapping when it did.
func Migrate(raw map[string]any) (changed bool) {
	for _, m := range migrations {
		if m(raw) {
			changed = true
		}
	}
	return changed
}

// migrateGoogleToDuckDuckGo renames plugins.google_images to
// plugins.duckduckgo_images (spec 008, R2.4).
//
// Every field of the block is kept. When the old block had no explicit
// download_dir it is pinned to the old Google Images default, so wallpapers
// the user already downloaded stay visible to the engine, which iterates the
// enabled plugins' download_dirs.
func migrateGoogleToDuckDuckGo(raw map[string]any) bool {
	plugins, ok := raw[keyPlugins].(map[string]any)
	if !ok {
		return false
	}
	block, hasOld := plugins["google_images"]
	if !hasOld {
		return false
	}
	if _, hasNew := plugins["duckduckgo_images"]; hasNew {
		return false
	}
	delete(plugins, "google_images")
	if b, isMap := block.(map[string]any); isMap && !truthy(b["download_dir"]) {
		b["download_dir"] = oldGoogleImagesDefaultDir()
	}
	plugins["duckduckgo_images"] = block
	return true
}
