/*
Package engine is the wallpaper selection logic ported from clockwork-orange.py
(spec 010 R3): fair selection across sources, per-monitor de-duplication, dual
desktop/lock-screen selection, and URL download.

"Fair" means what the Python meant (R3.2): sources are shuffled and the first
source that yields a candidate wins, then one image is drawn uniformly from
that source. A ten-image folder is therefore as likely to be chosen as a
ten-thousand-image one. That is the documented feature, not an accident.
*/
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// Rand is the source of randomness. Tests replace it with a seeded generator.
var Rand = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)) //nolint:gosec // wallpaper choice is not security-sensitive

// dedupAttempts is the retry budget when a draw collides with an image already
// chosen for another monitor. Hardcoded to 5 in four places in the Python
// (R3.3); exceeding it silently allows a duplicate.
const dedupAttempts = 5

// downloadTimeout is the URL fetch timeout (R3.5).
const downloadTimeout = 30 * time.Second

// ErrNoValidSources is returned when every source path is missing.
var ErrNoValidSources = errors.New("no valid sources found")

// Setter is what the engine needs from a platform: satisfied by
// platform.Platform.
type Setter interface {
	MonitorCount(ctx context.Context) int
	SetWallpaper(ctx context.Context, path string) error
	SetWallpaperMulti(ctx context.Context, paths []string) error
	SetLockscreen(ctx context.Context, path string) error
}

// GatherValidSources resolves and keeps the sources that exist, warning about
// the rest (R3.2, _gather_valid_sources).
func GatherValidSources(paths []string, ev events.Events) []string {
	valid := make([]string, 0, len(paths))
	for _, s := range paths {
		p, err := filepath.Abs(s)
		if err != nil {
			p = s
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if _, err := os.Stat(p); err != nil {
			ev.Warnf("Source does not exist: %s", p)
			continue
		}
		valid = append(valid, p)
	}
	return valid
}

// RandomImageFromSources picks one image fairly across sources (R3.2).
func RandomImageFromSources(sources []string, ev events.Events) (string, error) {
	ev.Debugf("Selecting from %d sources...", len(sources))
	valid := GatherValidSources(sources, ev)
	if len(valid) == 0 {
		return "", ErrNoValidSources
	}
	Rand.Shuffle(len(valid), func(i, j int) { valid[i], valid[j] = valid[j], valid[i] })
	for _, source := range valid {
		if candidate := selectCandidate(source, ev); candidate != "" {
			ev.Debugf("Selected image from source %s: %s", source, candidate)
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no image files found in any of the %d provided sources", len(valid))
}

// selectCandidate returns an image from one source: the file itself, or a
// uniform draw over the directory's non-recursive image children.
func selectCandidate(source string, ev events.Events) string {
	info, err := os.Stat(source)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		if imaging.IsImageFile(source) {
			return source
		}
		return ""
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		ev.Errorf("Failed to scan %s: %v", source, err)
		return ""
	}
	candidates := make([]string, 0, len(entries))
	for _, e := range entries {
		p := filepath.Join(source, e.Name())
		if imaging.IsImageFile(p) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[Rand.IntN(len(candidates))]
}

// pickAvoiding draws one image, retrying up to dedupAttempts times while the
// draw is in avoid (R3.3).
func pickAvoiding(sources, avoid []string, ev events.Events) (string, error) {
	img, err := RandomImageFromSources(sources, ev)
	if err != nil {
		return "", err
	}
	for attempts := 0; contains(avoid, img) && attempts < dedupAttempts; attempts++ {
		if img, err = RandomImageFromSources(sources, ev); err != nil {
			return "", err
		}
	}
	return img, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// PickDistinct draws n images, one per monitor, de-duplicating with the retry
// budget (R3.3). It stops early, without error, when a draw fails and at least
// one image was already chosen -- set_random_wallpaper_from_sources broke out
// of its loop on ValueError.
func PickDistinct(sources []string, n int, ev events.Events) ([]string, error) {
	images := make([]string, 0, n)
	for range n {
		img, err := pickAvoiding(sources, images, ev)
		if err != nil {
			if len(images) == 0 {
				return nil, err
			}
			break
		}
		images = append(images, img)
	}
	return images, nil
}

// PickLockscreenDistinct draws the lock-screen image, retrying while it
// collides with any desktop image (R3.3).
func PickLockscreenDistinct(sources, avoid []string, ev events.Events) (string, error) {
	return pickAvoiding(sources, avoid, ev)
}

// SetRandomFromSources sets one random image per monitor (R3.3,
// set_random_wallpaper_from_sources).
func SetRandomFromSources(ctx context.Context, s Setter, sources []string, ev events.Events) error {
	images, err := PickDistinct(sources, s.MonitorCount(ctx), ev)
	if err != nil {
		return fmt.Errorf("no images found in sources: %w", err)
	}
	if len(images) > 1 {
		return s.SetWallpaperMulti(ctx, images)
	}
	return setWallpaper(ctx, s, images[0], ev)
}

// SetDualFromSources sets per-monitor desktop images and a distinct lock-screen
// image (R3.3, set_dual_wallpaper_from_sources). Unlike SetRandomFromSources a
// failed desktop draw is an error, as it was in the Python.
func SetDualFromSources(ctx context.Context, s Setter, sources []string, ev events.Events) error {
	n := s.MonitorCount(ctx)
	desktop := make([]string, 0, n)
	for range n {
		img, err := pickAvoiding(sources, desktop, ev)
		if err != nil {
			return err
		}
		desktop = append(desktop, img)
	}
	lock, err := PickLockscreenDistinct(sources, desktop, ev)
	if err != nil {
		return err
	}
	var desktopErr error
	if len(desktop) > 1 {
		ev.Debugf("Setting desktop wallpapers (multi-monitor): %v", desktop)
		desktopErr = s.SetWallpaperMulti(ctx, desktop)
	} else {
		ev.Debugf("Setting desktop wallpaper: %s", desktop[0])
		desktopErr = setWallpaper(ctx, s, desktop[0], ev)
	}
	ev.Debugf("Setting lock screen wallpaper: %s", lock)
	lockErr := s.SetLockscreen(ctx, lock)
	return errors.Join(desktopErr, lockErr)
}

// SetLocalFile sets a single explicit file after checking it exists and is an
// image (set_local_wallpaper).
func SetLocalFile(ctx context.Context, s Setter, path string, ev events.Events) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	ev.Debugf("Setting wallpaper from local file: %s", abs)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("file does not exist: %s", abs)
	}
	if !imaging.IsImageFile(abs) {
		return fmt.Errorf("file is not a supported image format: %s", abs)
	}
	return setWallpaper(ctx, s, abs, ev)
}

// setWallpaper is set_wallpaper: resolve, check, log size, delegate.
func setWallpaper(ctx context.Context, s Setter, path string, ev events.Events) error {
	ev.Debugf("Setting wallpaper from path: %s", path)
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("file does not exist: %s", abs)
	}
	ev.Debugf("File size: %d bytes", info.Size())
	return s.SetWallpaper(ctx, abs)
}

// DownloadToTemp fetches url into a temp file with a .jpg suffix (R3.5). The
// returned cleanup removes the file (DV4: the Python leaked one per run).
func DownloadToTemp(ctx context.Context, client *http.Client, url string) (string, func(), error) {
	if client == nil {
		client = &http.Client{}
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("network error while downloading image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("failed to download image. HTTP status: %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "clockwork-*.jpg")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary file: %w", err)
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("write image to temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close temporary file: %w", err)
	}
	return path, cleanup, nil
}

// TwoDifferentImagesFromDirectory returns two distinct images from dir
// (R3.4, get_two_different_images_from_directory); errors with fewer than 2.
func TwoDifferentImagesFromDirectory(dir string) (string, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", fmt.Errorf("read directory %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if !e.IsDir() && imaging.IsImageFile(p) {
			files = append(files, p)
		}
	}
	if len(files) < 2 {
		return "", "", fmt.Errorf("need at least 2 images in %s, found %d", dir, len(files))
	}
	i := Rand.IntN(len(files))
	j := Rand.IntN(len(files) - 1)
	if j >= i {
		j++
	}
	return files[i], files[j], nil
}
