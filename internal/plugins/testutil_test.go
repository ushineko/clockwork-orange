package plugins

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/store"
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

// loadGolden decodes tests/golden/plugins/<name> into out.
func loadGolden(t *testing.T, name string, out any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "tests", "golden", "plugins", name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, out))
}

// testStores opens a fresh history and blacklist in a temp dir.
func testStores(t *testing.T) (*store.History, *store.Blacklist) {
	t.Helper()
	dir := t.TempDir()
	h, err := store.OpenHistory(filepath.Join(dir, "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	b, err := store.OpenBlacklist(filepath.Join(dir, "blacklist.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })
	return h, b
}

// fixedNow is a deterministic clock for tests.
var fixedNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

// recorder captures everything a plugin reports through events.Events.
type recorder struct {
	mu       sync.Mutex
	logs     []string
	progress []progressCall
	saved    []string
}

type progressCall struct {
	pct int
	msg string
}

func newRecorder() *recorder { return &recorder{} }

func (r *recorder) events() events.Events {
	return events.Events{
		OnLog: func(_ events.Level, msg string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.logs = append(r.logs, msg)
		},
		OnProgress: func(pct int, msg string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.progress = append(r.progress, progressCall{pct, msg})
		},
		OnImageSaved: func(path string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.saved = append(r.saved, path)
		},
	}
}

func (r *recorder) pcts() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int, len(r.progress))
	for i, p := range r.progress {
		out[i] = p.pct
	}
	return out
}

func (r *recorder) hasLogContaining(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.logs {
		if bytes.Contains([]byte(l), []byte(sub)) {
			return true
		}
	}
	return false
}

// syntheticPNG encodes a w×h RGBA image with a smooth gradient so the JPEG
// re-encode has something to chew on.
func syntheticPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// writeFile creates a file with content, failing the test on error.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
}

// dirNames lists the entry names of dir, sorted by os.ReadDir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
