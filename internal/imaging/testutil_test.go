package imaging

import (
	"os"
	"path/filepath"
	"testing"

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

// goldenImages is tests/golden/images/ under the repo root.
func goldenImages(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "tests", "golden", "images")
}
