package platform

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// repoRoot walks up from the working directory to the module's go.mod so
// tests can find tests/golden regardless of where `go test` was invoked.
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

// captureEvents returns an Events that appends formatted lines (with their
// level prefix) to the returned slice.
func captureEvents() (events.Events, func() []string) {
	var mu sync.Mutex
	var lines []string
	ev := events.Events{OnLog: func(level events.Level, msg string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, level.String()+" "+msg)
	}}
	return ev, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), lines...)
	}
}

// resolvedTempDir is t.TempDir with symlinks resolved, so paths compare
// equal to what resolvePath produces.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// touch creates an empty file and returns its path.
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	return p
}

// writeSolidPNG writes a w x h PNG filled with c.
func writeSolidPNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 0xff
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())
}
