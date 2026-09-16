package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// goldenPath locates a file under tests/golden/ by walking up from the test's
// working directory to the module root (the directory holding go.mod).
func goldenPath(t *testing.T, rel ...string) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(append([]string{dir, "tests", "golden"}, rel...)...)
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above %s", dir)
		dir = parent
	}
}

// readYAMLMap parses a YAML file to a plain mapping for semantic comparison;
// the Python and Go emitters differ in quoting and sequence indentation, so
// goldens are never compared byte-for-byte.
func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return parseYAMLMap(t, b)
}

func parseYAMLMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, yaml.Unmarshal(b, &m))
	return m
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	require.NoError(t, w.Close())
	return <-out
}

// tempHome points HomeDir() at a fresh directory and returns the config path
// under it.
func tempHome(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return DefaultPath()
}
