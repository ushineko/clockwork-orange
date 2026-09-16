package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
)

func TestLocalReturnsTheAbsoluteExpandedPathOfAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	res, err := newLocal(Deps{}).Run(context.Background(), map[string]any{"path": dir}, events.Events{})
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	require.Equal(t, want, res.Path)
	require.Empty(t, res.Message)
}

func TestLocalExpandsATildePathAgainstHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.Mkdir(filepath.Join(home, "Pictures"), 0o750))

	res, err := newLocal(Deps{}).Run(context.Background(), map[string]any{"path": "~/Pictures"}, events.Events{})
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(filepath.Join(home, "Pictures"))
	require.NoError(t, err)
	require.Equal(t, want, res.Path)
}

func TestLocalResolvesSymlinksLikePathResolve(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	require.NoError(t, os.Mkdir(target, 0o750))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))

	res, err := newLocal(Deps{}).Run(context.Background(), map[string]any{"path": link}, events.Events{})
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	require.Equal(t, want, res.Path)
}

func TestLocalAcceptsAFileAsWellAsADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "w.jpg")
	writeFile(t, file, []byte("x"))
	res, err := newLocal(Deps{}).Run(context.Background(), map[string]any{"path": file}, events.Events{})
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(file)
	require.NoError(t, err)
	require.Equal(t, want, res.Path)
}

func TestLocalRejectsAMissingOrEmptyPathBeforeAnythingElse(t *testing.T) {
	p := newLocal(Deps{})
	for _, cfg := range []map[string]any{
		{},
		{"path": ""},
		{"path": nil},
		{"path": "", "action": "process_blacklist", "targets": []any{"/x"}},
	} {
		_, err := p.Run(context.Background(), cfg, events.Events{})
		require.ErrorIs(t, err, errMissingPath, "%v", cfg)
		require.EqualError(t, err, "Missing 'path' in configuration")
	}
}

func TestLocalReportsANonexistentPathWithTheResolvedPathInTheMessage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, err := newLocal(Deps{}).Run(context.Background(), map[string]any{"path": missing}, events.Events{})
	require.EqualError(t, err, "Path not found: "+missing)
}

func TestLocalProcessBlacklistRunsEvenWhenThePathDoesNotExist(t *testing.T) {
	// Python checked the action before the existence check, so the GUI's
	// blacklist review works even if the configured source has moved.
	_, bl := testStores(t)
	dir := t.TempDir()
	victim := filepath.Join(dir, "bad.png")
	writeFile(t, victim, syntheticPNG(t, 8, 8))
	missingSource := filepath.Join(dir, "gone")

	rec := newRecorder()
	res, err := newLocal(Deps{Blacklist: bl}).Run(context.Background(), map[string]any{
		"path":    missingSource,
		"action":  "process_blacklist",
		"targets": []any{victim},
	}, rec.events())
	require.NoError(t, err)
	require.Equal(t, Result{Message: "Blacklist processed"}, res)
	require.NoFileExists(t, victim)
	items, err := bl.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "local", items[0].Source)
	require.True(t, rec.hasLogContaining("[Local] Processing blacklist for 1 files..."))
}

func TestLocalProcessBlacklistWithoutAStoreIsAnErrorNotASilentNoop(t *testing.T) {
	_, err := newLocal(Deps{}).Run(context.Background(), map[string]any{
		"path": t.TempDir(), "action": "process_blacklist", "targets": []any{"/x"},
	}, events.Events{})
	require.ErrorContains(t, err, "blacklist store not configured")
}
