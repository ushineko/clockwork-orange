package sdlib

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func libFS(version string) fstest.MapFS {
	return fstest.MapFS{
		"VERSION":                {Data: []byte(version + "\n")},
		"libstable-diffusion.so": {Data: []byte("native")},
		"libggml.so.0":           {Data: []byte("ggml")},
		".gitkeep":               {Data: nil},
	}
}

func TestExtractCopiesFilesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	dir, err := Extract(libFS("v1"), root, "v1")
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, "libstable-diffusion.so"))
	require.NoError(t, err)
	require.Equal(t, "native", string(b))
	require.NoFileExists(t, filepath.Join(dir, ".gitkeep"))

	again, err := Extract(libFS("v1"), root, "v1")
	require.NoError(t, err)
	require.Equal(t, dir, again)

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no staging directory is left behind")
}

func TestExtractRefusesVersionMismatch(t *testing.T) {
	_, err := Extract(libFS("v1"), t.TempDir(), "v2")
	require.ErrorContains(t, err, `needs "v2"`)
}

func TestExtractNeedsVersionFile(t *testing.T) {
	fsys := libFS("v1")
	delete(fsys, "VERSION")
	_, err := Extract(fsys, t.TempDir(), "v1")
	require.ErrorContains(t, err, "no VERSION file")
}

func TestExtractDirectoryChangesWithContent(t *testing.T) {
	root := t.TempDir()
	a, err := Extract(libFS("v1"), root, "v1")
	require.NoError(t, err)
	changed := libFS("v1")
	changed["libggml.so.0"] = &fstest.MapFile{Data: []byte("different")}
	b, err := Extract(changed, root, "v1")
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestExtractEmptyLibraryIsNoLibrary(t *testing.T) {
	_, err := Extract(fstest.MapFS{"VERSION": {Data: []byte("v1")}}, t.TempDir(), "v1")
	require.ErrorIs(t, err, ErrNoLibrary)
}

func TestResolvePrefersEnvDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	got, err := Resolve()
	require.NoError(t, err)
	require.Equal(t, dir, got)
}

func TestResolveRejectsBadEnvDir(t *testing.T) {
	t.Setenv(EnvDir, filepath.Join(t.TempDir(), "missing"))
	_, err := Resolve()
	require.ErrorContains(t, err, "not a directory")
}

func TestResolveWithoutEmbeddedLibrary(t *testing.T) {
	if Embedded() {
		t.Skip("built with sdembed")
	}
	t.Setenv(EnvDir, "")
	_, err := Resolve()
	require.ErrorIs(t, err, ErrNoLibrary)
}

// The Makefile stages a VERSION file from SD_VERSION; it must be the version
// the binding is pinned to or the embedded library would be refused at run time.
func TestMakefileVersionMatchesThePin(t *testing.T) {
	mk, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^SD_VERSION\?=(\S+)$`).FindSubmatch(mk)
	require.NotNil(t, m, "Makefile has no SD_VERSION")
	require.Equal(t, PinnedVersion, string(m[1]))
}
