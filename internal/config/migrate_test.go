package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// goldenHome is the $HOME the Python capture ran under; the pinned
// download_dir in post_migration.yml embeds it.
const goldenHome = "/home/nverenin"

// installGolden copies a golden config into a fresh $HOME and returns the
// config path Load will use.
func installGolden(t *testing.T, name string) string {
	t.Helper()
	cfg := tempHome(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg), 0o750))
	b, err := os.ReadFile(goldenPath(t, "config", name))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg, b, 0o600))
	return cfg
}

// expectedGolden loads a post-migration golden and rewrites the capture
// machine's home directory to this test's, the way the platform goldens
// substitute paths.
func expectedGolden(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(goldenPath(t, "config", name))
	require.NoError(t, err)
	text := strings.ReplaceAll(string(b), goldenHome, HomeDir())
	return parseYAMLMap(t, []byte(text))
}

func TestLoadMigratesGoogleImagesAndKeepsTheStableDiffusionBlock(t *testing.T) {
	// Phase 2 acceptance criterion: the golden diff. 2.9.x clean_config would
	// have deleted stable_diffusion and unknown_top_level on the way through.
	cfg := installGolden(t, "pre_migration.yml")

	var d Document
	stderr := captureStderr(t, func() {
		var err error
		d, err = Load(cfg)
		require.NoError(t, err)
	})
	require.Contains(t, stderr, "[config-migration] Persisted migrated config to "+cfg)

	require.Contains(t, d.Plugins, "duckduckgo_images")
	require.NotContains(t, d.Plugins, "google_images")
	require.Equal(t, filepath.Join(HomeDir(), "Pictures", "Wallpapers", "GoogleImages"),
		d.Plugins["duckduckgo_images"]["download_dir"])
	require.Contains(t, d.Plugins, "stable_diffusion")
	require.Equal(t, map[string]any{"keep": "me"}, d.Extra["unknown_top_level"])

	onDisk := readYAMLMap(t, cfg)
	require.Equal(t, expectedGolden(t, "post_migration.yml"), onDisk)
	text, err := os.ReadFile(cfg)
	require.NoError(t, err)
	require.Contains(t, string(text), "stable_diffusion:")
	require.Contains(t, string(text), "unknown_top_level:")

	// The Go writer must match what Marshal produces for the same document.
	want, err := Marshal(d)
	require.NoError(t, err)
	require.Equal(t, string(want), string(text))
}

func TestLoadDoesNotRepinDownloadDirWhenTheOldBlockHadOne(t *testing.T) {
	cfg := installGolden(t, "pre_migration_with_dir.yml")
	_ = captureStderr(t, func() {
		d, err := Load(cfg)
		require.NoError(t, err)
		require.Equal(t, "/data/wp", d.Plugins["duckduckgo_images"]["download_dir"])
	})
	require.Equal(t, expectedGolden(t, "post_migration_with_dir.yml"), readYAMLMap(t, cfg))
}

func TestMigrateIsIdempotentAndLeavesAnAlreadyMigratedFileAlone(t *testing.T) {
	// The list runs on every load; a second pass must be a no-op or every
	// start would rewrite the file and wake the watcher.
	raw := parseYAMLMap(t, []byte("plugins:\n  google_images:\n    enabled: true\n"))
	require.True(t, Migrate(raw))
	require.False(t, Migrate(raw))

	both := parseYAMLMap(t, []byte("plugins:\n  google_images: {enabled: true}\n  duckduckgo_images: {enabled: false}\n"))
	require.False(t, Migrate(both), "an existing duckduckgo_images block wins; nothing is clobbered")
	require.Contains(t, both["plugins"], "google_images")

	require.False(t, Migrate(map[string]any{}))
	require.False(t, Migrate(map[string]any{"plugins": []any{"google_images"}}))
}

func TestMigratePinsDownloadDirOnlyWhenAbsentOrEmpty(t *testing.T) {
	raw := parseYAMLMap(t, []byte("plugins:\n  google_images:\n    download_dir: ''\n"))
	require.True(t, Migrate(raw))
	block := raw["plugins"].(map[string]any)["duckduckgo_images"].(map[string]any)
	require.Equal(t, oldGoogleImagesDefaultDir(), block["download_dir"])
}

func TestLoadDoesNotRewriteAFileThatNeedsNoMigration(t *testing.T) {
	cfg := installGolden(t, "post_migration.yml")
	before, err := os.Stat(cfg)
	require.NoError(t, err)
	stderr := captureStderr(t, func() {
		_, err := Load(cfg)
		require.NoError(t, err)
	})
	require.Empty(t, stderr)
	after, err := os.Stat(cfg)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
}

func TestLoadReturnsTheMigratedDocumentWhenPersistingFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	cfg := installGolden(t, "pre_migration.yml")
	dir := filepath.Dir(cfg)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	var d Document
	stderr := captureStderr(t, func() {
		var err error
		d, err = Load(cfg)
		require.NoError(t, err)
	})
	require.Contains(t, stderr, "[config-migration] Failed to persist migrated config to "+cfg)
	require.Contains(t, d.Plugins, "duckduckgo_images")
	require.Contains(t, readYAMLMap(t, cfg)["plugins"], "google_images", "disk still has the old key for the retry")
}

func TestLoadOfAMissingFileReturnsDefaultsWithoutError(t *testing.T) {
	cfg := tempHome(t)
	d, err := Load(cfg)
	require.NoError(t, err)
	require.Equal(t, Defaults(), d)
}

func TestLoadDefaultReportsWhichCandidateWasUsed(t *testing.T) {
	cfg := tempHome(t)
	d, used, err := LoadDefault()
	require.NoError(t, err)
	require.Equal(t, "", used)
	require.Equal(t, Defaults(), d)

	require.NoError(t, Save(cfg, Document{DefaultWait: 42}))
	d, used, err = LoadDefault()
	require.NoError(t, err)
	require.Equal(t, cfg, used)
	require.Equal(t, 42, d.DefaultWait)
}

func TestLoadReportsAMalformedFileWithItsPath(t *testing.T) {
	cfg := tempHome(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg), 0o750))
	require.NoError(t, os.WriteFile(cfg, []byte("desktop: [\n"), 0o600))
	_, err := Load(cfg)
	require.ErrorContains(t, err, cfg)
}
