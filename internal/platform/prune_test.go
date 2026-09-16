package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func populatedCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "deep", "x.bin"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte("y"), 0o600))
	old := wallpaperCacheDir
	wallpaperCacheDir = dir
	t.Cleanup(func() { wallpaperCacheDir = old })
	return dir
}

func entries(t *testing.T, dir string) int {
	t.Helper()
	es, err := os.ReadDir(dir)
	require.NoError(t, err)
	return len(es)
}

func TestPruneRemovesEveryEntryOnlyWhenDuReportsMoreThanTheThreshold(t *testing.T) {
	dir := populatedCacheDir(t)
	ctx := context.Background()

	exactly := &FakeRunner{Stdout: "512000\t" + dir + "\n"} // 500 MB exactly: keep
	require.NoError(t, PruneWallpaperCache(ctx, exactly, 500))
	require.Equal(t, 2, entries(t, dir))
	require.Equal(t, [][]string{{"du", "-sk", dir}}, exactly.Calls())

	over := &FakeRunner{Stdout: "512001\t" + dir + "\n"}
	require.NoError(t, PruneWallpaperCache(ctx, over, 500))
	require.Equal(t, 0, entries(t, dir), "the directory itself stays; its contents go")
	require.DirExists(t, dir)
}

func TestPruneDoesNothingWhenDuFailsOrPrintsGarbage(t *testing.T) {
	dir := populatedCacheDir(t)
	ctx := context.Background()
	require.Error(t, PruneWallpaperCache(ctx, &FakeRunner{Err: errors.New("exit status 1")}, 500))
	require.Error(t, PruneWallpaperCache(ctx, &FakeRunner{Stdout: "lots\n"}, 500))
	require.Error(t, PruneWallpaperCache(ctx, &FakeRunner{Stdout: ""}, 500))
	require.Equal(t, 2, entries(t, dir))
}

func TestPruneSkipsSilentlyWhenTheCacheDirectoryDoesNotExist(t *testing.T) {
	old := wallpaperCacheDir
	wallpaperCacheDir = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { wallpaperCacheDir = old })
	r := &FakeRunner{}
	require.NoError(t, PruneWallpaperCache(context.Background(), r, 500))
	require.Empty(t, r.Calls(), "du must not run for a missing directory")
}

func TestPruneRunsDuWithATenSecondDeadline(t *testing.T) {
	populatedCacheDir(t)
	r := &FakeRunner{Handler: func(ctx context.Context, _ []string) (string, string, error) {
		dl, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, duTimeout, time.Until(dl), float64(time.Second))
		return "1\n", "", nil
	}}
	require.NoError(t, PruneWallpaperCache(context.Background(), r, 500))
}
