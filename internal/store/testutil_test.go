package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// repoRoot walks up from the test's working directory to the first directory
// holding go.mod, so fixture paths do not depend on how `go test` was invoked.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above %s", dir)
		dir = parent
	}
}

// goldenImage is the absolute path of a fixture image.
func goldenImage(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "tests", "golden", "images", name)
}

// copyGoldenDB copies tests/golden/db/<name> into a temp dir and returns the
// copy's path, so tests can never modify the committed fixture.
func copyGoldenDB(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join(repoRoot(t), "tests", "golden", "db", name)
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	dst := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(dst, data, 0o600))
	return dst
}

// fixedClock pins Now to ts float seconds for the test's duration.
func fixedClock(t *testing.T, ts float64) {
	t.Helper()
	prev := Now
	sec := int64(ts)
	nsec := int64((ts - float64(sec)) * 1e9)
	Now = func() time.Time { return time.Unix(sec, nsec) }
	t.Cleanup(func() { Now = prev })
}

// copyFile writes a copy of src to a fresh temp path and returns it.
func copyFile(t *testing.T, src, name string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	dst := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(dst, data, 0o600))
	return dst
}
