package platform

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// This file is the OS-independent half of the Windows spanned-wallpaper
// technique (D10, R4.7) so its geometry and control flow are unit-tested on
// every OS. platform_windows.go supplies the real winDesktop.

// winDesktop is the Windows desktop's side effects behind an interface so
// tests exercise stitchAndApply with a fake (spec 010 Phase 3 AC).
type winDesktop interface {
	// monitors enumerates displays in virtual-screen coordinates.
	monitors() ([]Monitor, error)
	// setSpanRegistry sets HKCU\Control Panel\Desktop WallpaperStyle=22,
	// TileWallpaper=0.
	setSpanRegistry() error
	// applyWallpaper is SystemParametersInfoW(SPI_SETDESKWALLPAPER, 0, path,
	// SPIF_UPDATEINIFILE|SPIF_SENDWININICHANGE).
	applyWallpaper(path string) error
}

// spannedFileName is the composite the desktop is pointed at.
const spannedFileName = "clockwork_spanned.jpg"

// spannedDir picks the directory for the composite the way the Python did:
// %TEMP%, else %USERPROFILE%, else the working directory.
func spannedDir(getenv func(string) string) string {
	if v := getenv("TEMP"); v != "" {
		return v
	}
	if v := getenv("USERPROFILE"); v != "" {
		return v
	}
	return "."
}

// canvasBounds is the bounding box of all monitors: its origin (which may be
// negative when a secondary display sits left of or above the primary) and
// its size.
func canvasBounds(monitors []Monitor) (minX, minY, w, h int) {
	if len(monitors) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY = monitors[0].X, monitors[0].Y
	maxX, maxY := monitors[0].X+monitors[0].W, monitors[0].Y+monitors[0].H
	for _, m := range monitors[1:] {
		minX = min(minX, m.X)
		minY = min(minY, m.Y)
		maxX = max(maxX, m.X+m.W)
		maxY = max(maxY, m.Y+m.H)
	}
	return minX, minY, maxX - minX, maxY - minY
}

// composite draws paths[i] cover-resized and centre-cropped onto monitor i
// of a black canvas spanning all monitors. Monitors beyond len(paths) stay
// black; images beyond len(monitors) are ignored; an image that fails to
// decode is logged and its monitor stays black -- all as in the Python.
func composite(monitors []Monitor, paths []string, ev events.Events) *image.RGBA {
	minX, minY, w, h := canvasBounds(monitors)
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{A: 0xff}), image.Point{}, draw.Src)
	for i, m := range monitors {
		if i >= len(paths) {
			break
		}
		src, _, err := imaging.DecodeFile(paths[i])
		if err != nil {
			ev.Errorf("Failed to process image %s: %v", paths[i], err)
			continue
		}
		tile := imaging.CoverResizeCrop(imaging.ToRGB(src), m.W, m.H)
		at := image.Pt(m.X-minX, m.Y-minY)
		draw.Draw(canvas, image.Rectangle{Min: at, Max: at.Add(image.Pt(m.W, m.H))}, tile, image.Point{}, draw.Src)
	}
	return canvas
}

// stitchAndApply is set_wallpaper_multi_monitor's Windows body (R4.7):
// composite the images over the monitor layout, save the JPEG at outDir,
// switch the desktop to "span" style and apply. A registry failure is logged
// and the apply still attempted, as in the Python; paths must already be
// resolved and existing.
func stitchAndApply(d winDesktop, outDir string, paths []string, ev events.Events) error {
	monitors, err := d.monitors()
	if err != nil {
		return fmt.Errorf("enumerate monitors: %w", err)
	}
	if len(monitors) == 0 {
		return errors.New("no monitors detected")
	}
	ev.Debugf("Stitching wallpapers for %d monitor(s)", len(monitors))

	canvas := composite(monitors, paths, ev)
	out := filepath.Join(outDir, spannedFileName)
	if err := imaging.SaveJPEG(out, canvas, 90); err != nil {
		return fmt.Errorf("save spanned wallpaper: %w", err)
	}
	if err := d.setSpanRegistry(); err != nil {
		ev.Errorf("Failed to set registry for spanning: %v", err)
	}
	ev.Debugf("Applying spanned wallpaper: %s", out)
	if err := d.applyWallpaper(out); err != nil {
		return fmt.Errorf("apply spanned wallpaper: %w", err)
	}
	return nil
}
