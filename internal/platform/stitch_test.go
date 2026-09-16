package platform

import (
	"errors"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/imaging"
)

var (
	red   = color.RGBA{R: 0xff, A: 0xff}
	blue  = color.RGBA{B: 0xff, A: 0xff}
	black = color.RGBA{A: 0xff}
)

func TestCanvasBoundsSpanAllMonitorsIncludingNegativeOrigins(t *testing.T) {
	minX, minY, w, h := canvasBounds([]Monitor{
		{X: 0, Y: 0, W: 1920, H: 1080},
		{X: 1920, Y: -400, W: 1080, H: 1920},
	})
	require.Equal(t, 0, minX)
	require.Equal(t, -400, minY)
	require.Equal(t, 3000, w)
	require.Equal(t, 1920, h)

	minX, minY, w, h = canvasBounds([]Monitor{{X: -1920, Y: 0, W: 1920, H: 1080}, {X: 0, Y: 0, W: 1920, H: 1080}})
	require.Equal(t, -1920, minX)
	require.Equal(t, 0, minY)
	require.Equal(t, 3840, w)
	require.Equal(t, 1080, h)

	_, _, w, h = canvasBounds(nil)
	require.Zero(t, w)
	require.Zero(t, h)
}

func TestCompositePlacesEachImageAtItsMonitorOffsetOnABlackCanvas(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.png")
	writeSolidPNG(t, a, 400, 300, red)  // landscape, will be cover-resized
	writeSolidPNG(t, b, 100, 400, blue) // portrait
	monitors := []Monitor{
		{X: -200, Y: 0, W: 200, H: 100}, // left of the primary: negative X
		{X: 0, Y: 20, W: 100, H: 200},
	}
	ev, lines := captureEvents()
	canvas := composite(monitors, []string{a, b}, ev)
	require.Empty(t, lines())

	require.Equal(t, 300, canvas.Bounds().Dx(), "canvas width is the bounding box")
	require.Equal(t, 220, canvas.Bounds().Dy())
	// monitor 0 lands at (0,0)-(200,100)
	require.Equal(t, red, canvas.RGBAAt(0, 0))
	require.Equal(t, red, canvas.RGBAAt(199, 99))
	// monitor 1 lands at (200,20)-(300,220)
	require.Equal(t, blue, canvas.RGBAAt(200, 20))
	require.Equal(t, blue, canvas.RGBAAt(299, 219))
	// everything else is black
	require.Equal(t, black, canvas.RGBAAt(0, 150))
	require.Equal(t, black, canvas.RGBAAt(250, 5))
}

func TestCompositeLeavesMonitorsWithoutImagesBlackAndIgnoresExtraImages(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	writeSolidPNG(t, a, 50, 50, red)
	monitors := []Monitor{{X: 0, Y: 0, W: 50, H: 50}, {X: 50, Y: 0, W: 50, H: 50}}
	ev, _ := captureEvents()

	one := composite(monitors, []string{a}, ev)
	require.Equal(t, red, one.RGBAAt(10, 10))
	require.Equal(t, black, one.RGBAAt(60, 10), "second monitor has no image: stays black")

	three := composite(monitors[:1], []string{a, a, a}, ev)
	require.Equal(t, 50, three.Bounds().Dx(), "images beyond the monitor count are ignored")
}

func TestCompositeLogsAndSkipsAnUndecodableImage(t *testing.T) {
	dir := t.TempDir()
	bad := touch(t, dir, "bad.png")
	ev, lines := captureEvents()
	canvas := composite([]Monitor{{X: 0, Y: 0, W: 10, H: 10}}, []string{bad}, ev)
	require.Equal(t, black, canvas.RGBAAt(5, 5))
	require.Contains(t, strings.Join(lines(), "\n"), "[ERROR] Failed to process image "+bad)
}

// fakeDesktop records the Windows side effects.
type fakeDesktop struct {
	mons        []Monitor
	monErr      error
	registryErr error
	applyErr    error
	registrySet int
	applied     []string
}

func (f *fakeDesktop) monitors() ([]Monitor, error) { return f.mons, f.monErr }
func (f *fakeDesktop) setSpanRegistry() error       { f.registrySet++; return f.registryErr }
func (f *fakeDesktop) applyWallpaper(p string) error {
	f.applied = append(f.applied, p)
	return f.applyErr
}

func TestStitchAndApplySavesQ90JPEGSetsRegistryThenAppliesIt(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	writeSolidPNG(t, a, 64, 64, red)
	d := &fakeDesktop{mons: []Monitor{{X: 0, Y: 0, W: 32, H: 32}, {X: 32, Y: 0, W: 32, H: 32}}}
	ev, lines := captureEvents()

	require.NoError(t, stitchAndApply(d, dir, []string{a, a}, ev))
	want := filepath.Join(dir, "clockwork_spanned.jpg")
	require.Equal(t, []string{want}, d.applied)
	require.Equal(t, 1, d.registrySet)
	img, format, err := imaging.DecodeFile(want)
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 64, img.Bounds().Dx())
	require.Equal(t, 32, img.Bounds().Dy())
	joined := strings.Join(lines(), "\n")
	require.Contains(t, joined, "[DEBUG] Stitching wallpapers for 2 monitor(s)")
	require.Contains(t, joined, "[DEBUG] Applying spanned wallpaper: "+want)
}

func TestStitchAndApplyTreatsRegistryFailureAsNonFatalButApplyFailureAsFatal(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	writeSolidPNG(t, a, 8, 8, red)
	ev, lines := captureEvents()

	d := &fakeDesktop{mons: []Monitor{{W: 8, H: 8}}, registryErr: errors.New("access denied")}
	require.NoError(t, stitchAndApply(d, dir, []string{a}, ev))
	require.Len(t, d.applied, 1)
	require.Contains(t, strings.Join(lines(), "\n"), "[ERROR] Failed to set registry for spanning: access denied")

	d = &fakeDesktop{mons: []Monitor{{W: 8, H: 8}}, applyErr: errors.New("SPI failed")}
	require.ErrorContains(t, stitchAndApply(d, dir, []string{a}, ev), "SPI failed")

	d = &fakeDesktop{monErr: errors.New("enum failed")}
	require.ErrorContains(t, stitchAndApply(d, dir, []string{a}, ev), "enum failed")
	d = &fakeDesktop{}
	require.ErrorContains(t, stitchAndApply(d, dir, []string{a}, ev), "no monitors detected")
	require.Empty(t, d.applied)
}

func TestSpannedDirPrefersTEMPThenUSERPROFILEThenCwd(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	require.Equal(t, `C:\Temp`, spannedDir(env(map[string]string{"TEMP": `C:\Temp`, "USERPROFILE": `C:\Users\x`})))
	require.Equal(t, `C:\Users\x`, spannedDir(env(map[string]string{"USERPROFILE": `C:\Users\x`})))
	require.Equal(t, ".", spannedDir(env(nil)))
}
