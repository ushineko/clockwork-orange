package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveLeavesOnlyTheConfigFileBehind(t *testing.T) {
	// The temp file must be renamed away, never left beside the config where
	// the watcher and the user would see it.
	cfg := tempHome(t)
	require.NoError(t, Save(cfg, Document{DefaultWait: 120, Desktop: true}))

	entries, err := os.ReadDir(filepath.Dir(cfg))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, FileName, entries[0].Name())

	d, err := Load(cfg)
	require.NoError(t, err)
	require.Equal(t, 120, d.DefaultWait)
	require.True(t, d.Desktop)
}

func TestSaveCreatesTheMissingParentDirectory(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "deep", "er", FileName)
	require.NoError(t, Save(cfg, Document{DefaultWait: 1}))
	_, err := os.Stat(cfg)
	require.NoError(t, err)
}

func TestSaveRefusesABareTildeDirectory(t *testing.T) {
	err := Save(filepath.Join(t.TempDir(), "~", FileName), Document{})
	require.ErrorIs(t, err, ErrTildeComponent)
}

func TestSaveKeepsTheExistingFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	cfg := tempHome(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg), 0o750))
	require.NoError(t, os.WriteFile(cfg, []byte("default_wait: 1\n"), 0o644))

	require.NoError(t, Save(cfg, Document{DefaultWait: 2}))
	st, err := os.Stat(cfg)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), st.Mode().Perm())
}

func TestSaveReplacesTheFileWholeUnderConcurrentWriters(t *testing.T) {
	// DV3: auto-save, migration persist and the logs toggle used to race; the
	// file must always parse and hold exactly one writer's document.
	cfg := tempHome(t)
	const writers = 16
	var wg sync.WaitGroup
	for i := 1; i <= writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			require.NoError(t, Save(cfg, Document{DefaultWait: n, Extra: map[string]any{"marker": n}}))
		}(i)
	}
	wg.Wait()

	d, err := Load(cfg)
	require.NoError(t, err)
	require.Equal(t, d.DefaultWait, d.Extra["marker"], "the file must be one writer's document, not a splice")
	entries, err := os.ReadDir(filepath.Dir(cfg))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}
