package gui

import (
	"image"
	"sync"

	xdraw "golang.org/x/image/draw"

	"github.com/ushineko/clockwork-orange/internal/imaging"
)

/*
previewCache holds images scaled down to preview size, by path.

The review's images are 4K JPEGs. Decoding one takes a tenth of a second and
handing the full 3840×2160 frame to a canvas.Image costs more on every redraw
than the decode did, because Fyne re-scales it to the pane each time; and the
mark overlay walked eight million pixels per Space. So each image is decoded
once, scaled to the preview's own size, and kept; the overlay is drawn on the
small copy; and the neighbours are scaled ahead of time so ←/→ land on a
cached frame.

Bounded: sixteen entries at about 1600×900 RGBA is ~90 MB at most, and the
oldest goes when a new one arrives.
*/
type previewCache struct {
	mu    sync.Mutex
	items map[string]image.Image
	order []string
	// inflight de-duplicates concurrent loads of one path.
	inflight map[string]chan struct{}
}

// Preview geometry: the frame is scaled to fit inside this box. Larger than
// the pane so a 1.5× HiDPI scale still gets a pixel per pixel.
const (
	previewMaxW  = 1600
	previewMaxH  = 900
	previewCap   = 16
	prefetchEach = 1 // neighbours on each side to scale ahead
)

func newPreviewCache() *previewCache {
	return &previewCache{items: map[string]image.Image{}, inflight: map[string]chan struct{}{}}
}

// get returns the scaled image for path, loading it when absent. It blocks
// the caller, so it runs off the UI thread.
func (c *previewCache) get(path string) (image.Image, error) {
	c.mu.Lock()
	if img, ok := c.items[path]; ok {
		c.mu.Unlock()
		return img, nil
	}
	if wait, ok := c.inflight[path]; ok {
		c.mu.Unlock()
		<-wait
		return c.get(path)
	}
	done := make(chan struct{})
	c.inflight[path] = done
	c.mu.Unlock()

	img, err := loadPreview(path)

	c.mu.Lock()
	delete(c.inflight, path)
	close(done)
	if err == nil {
		c.put(path, img)
	}
	c.mu.Unlock()
	return img, err
}

// put stores an entry, evicting the oldest past the cap. Caller holds mu.
func (c *previewCache) put(path string, img image.Image) {
	if _, ok := c.items[path]; !ok {
		c.order = append(c.order, path)
	}
	c.items[path] = img
	for len(c.order) > previewCap {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}

// has reports whether path is cached, for the prefetcher.
func (c *previewCache) has(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[path]
	return ok
}

// forget drops entries for paths no longer in keep, after a rescan.
func (c *previewCache) forget(keep map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.order[:0]
	for _, p := range c.order {
		if keep[p] {
			kept = append(kept, p)
		} else {
			delete(c.items, p)
		}
	}
	c.order = kept
}

// loadPreview decodes and scales one file to fit the preview box.
func loadPreview(path string) (image.Image, error) {
	src, _, err := imaging.DecodeFile(path)
	if err != nil {
		return nil, err
	}
	return fitPreview(src), nil
}

/*
fitPreview scales src to fit inside previewMaxW×previewMaxH, keeping its
aspect ratio; an image already small enough is returned as-is. ApproxBiLinear
rather than CatmullRom: this is a thumbnail for a human to glance at, not a
wallpaper for the desktop, and it is four times faster.
*/
func fitPreview(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= previewMaxW && h <= previewMaxH {
		return src
	}
	scale := min(float64(previewMaxW)/float64(w), float64(previewMaxH)/float64(h))
	dw, dh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}
