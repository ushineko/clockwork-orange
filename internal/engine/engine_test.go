package engine

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
)

type fakeSetter struct {
	monitors int
	single   []string
	multi    [][]string
	lock     []string
	lockErr  error
}

func (f *fakeSetter) MonitorCount(context.Context) int { return f.monitors }
func (f *fakeSetter) SetWallpaper(_ context.Context, p string) error {
	f.single = append(f.single, p)
	return nil
}

func (f *fakeSetter) SetWallpaperMulti(_ context.Context, p []string) error {
	f.multi = append(f.multi, p)
	return nil
}

func (f *fakeSetter) SetLockscreen(_ context.Context, p string) error {
	f.lock = append(f.lock, p)
	return f.lockErr
}

func makeDir(t *testing.T, n int, prefix string) string {
	t.Helper()
	dir := t.TempDir()
	for i := range n {
		require.NoError(t, os.WriteFile(filepath.Join(dir, prefix+string(rune('a'+i%26))+string(rune('a'+i/26))+".jpg"), []byte("x"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	return dir
}

func seed(t *testing.T) {
	t.Helper()
	old := Rand
	Rand = rand.New(rand.NewPCG(1, 2))
	t.Cleanup(func() { Rand = old })
}

// A ten-image source and a two-hundred-image source are chosen equally often:
// fairness is per source, not per image (R3.2). The Python documented this as a
// feature; a port that flattened the candidates would let big libraries dominate.
func TestSmallAndLargeSourcesAreChosenEquallyOften(t *testing.T) {
	seed(t)
	small := makeDir(t, 10, "s")
	large := makeDir(t, 200, "l")
	counts := map[string]int{}
	const draws = 4000
	for range draws {
		img, err := RandomImageFromSources([]string{small, large}, events.Events{})
		require.NoError(t, err)
		counts[filepath.Dir(img)]++
	}
	ratio := float64(counts[small]) / draws
	require.InDelta(t, 0.5, ratio, 0.03, "small source share %v", ratio)
}

// A source that is a single image file is itself the candidate; a text file
// is not, and a missing source is skipped with a warning rather than an error.
func TestFileSourcesAndMissingSources(t *testing.T) {
	seed(t)
	dir := t.TempDir()
	img := filepath.Join(dir, "one.png")
	require.NoError(t, os.WriteFile(img, []byte("x"), 0o600))
	txt := filepath.Join(dir, "one.txt")
	require.NoError(t, os.WriteFile(txt, []byte("x"), 0o600))
	var warned []string
	ev := events.Events{OnLog: func(l events.Level, m string) {
		if l == events.LevelWarn {
			warned = append(warned, m)
		}
	}}
	got, err := RandomImageFromSources([]string{filepath.Join(dir, "missing"), img}, ev)
	require.NoError(t, err)
	require.Equal(t, img, got)
	require.Len(t, warned, 1)

	_, err = RandomImageFromSources([]string{txt}, ev)
	require.Error(t, err)
	_, err = RandomImageFromSources([]string{filepath.Join(dir, "missing")}, ev)
	require.ErrorIs(t, err, ErrNoValidSources)
}

// With two monitors and two images, de-dup yields both images; with one image
// the retry budget runs out and the duplicate is allowed, as in the Python.
func TestPerMonitorDedupRetriesFiveTimesThenGivesUp(t *testing.T) {
	seed(t)
	two := makeDir(t, 2, "t")
	imgs, err := PickDistinct([]string{two}, 2, events.Events{})
	require.NoError(t, err)
	require.NotEqual(t, imgs[0], imgs[1])

	one := makeDir(t, 1, "o")
	imgs, err = PickDistinct([]string{one}, 2, events.Events{})
	require.NoError(t, err)
	require.Len(t, imgs, 2)
	require.Equal(t, imgs[0], imgs[1], "duplicate allowed after retries")
}

// SetRandomFromSources uses the multi-monitor call only when more than one
// image was drawn; a single monitor goes through the plain setter.
func TestSetRandomUsesMultiOnlyForSeveralImages(t *testing.T) {
	seed(t)
	dir := makeDir(t, 5, "m")
	s := &fakeSetter{monitors: 1}
	require.NoError(t, SetRandomFromSources(context.Background(), s, []string{dir}, events.Events{}))
	require.Len(t, s.single, 1)
	require.Empty(t, s.multi)

	s = &fakeSetter{monitors: 3}
	require.NoError(t, SetRandomFromSources(context.Background(), s, []string{dir}, events.Events{}))
	require.Len(t, s.multi, 1)
	require.Len(t, s.multi[0], 3)
}

// Dual mode never gives the lock screen a desktop image when enough images
// exist, and reports the lock-screen failure while still setting the desktop.
func TestDualModeLockscreenDistinctAndErrorsJoined(t *testing.T) {
	seed(t)
	dir := makeDir(t, 6, "d")
	s := &fakeSetter{monitors: 2, lockErr: errors.New("kwriteconfig6 failed")}
	err := SetDualFromSources(context.Background(), s, []string{dir}, events.Events{})
	require.ErrorContains(t, err, "kwriteconfig6 failed")
	require.Len(t, s.multi, 1)
	require.Len(t, s.lock, 1)
	require.NotContains(t, s.multi[0], s.lock[0])
}

// DownloadToTemp writes the body to a .jpg temp file and cleanup removes it
// (DV4); a non-200 status is an error and leaves no file behind.
func TestDownloadToTempCleansUpAndRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("jpegbytes"))
	}))
	defer srv.Close()
	path, cleanup, err := DownloadToTemp(context.Background(), srv.Client(), srv.URL+"/ok")
	require.NoError(t, err)
	require.Equal(t, ".jpg", filepath.Ext(path))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "jpegbytes", string(b))
	cleanup()
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))

	_, _, err = DownloadToTemp(context.Background(), srv.Client(), srv.URL+"/bad")
	require.ErrorContains(t, err, "HTTP status: 404")
}

// TwoDifferentImagesFromDirectory never returns the same file twice and
// refuses a directory with fewer than two images.
func TestTwoDifferentImagesAreDistinct(t *testing.T) {
	seed(t)
	dir := makeDir(t, 2, "p")
	for range 50 {
		a, b, err := TwoDifferentImagesFromDirectory(dir)
		require.NoError(t, err)
		require.NotEqual(t, a, b)
	}
	_, _, err := TwoDifferentImagesFromDirectory(makeDir(t, 1, "q"))
	require.Error(t, err)
}
