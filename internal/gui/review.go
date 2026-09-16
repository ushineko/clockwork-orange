package gui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/fsnotify/fsnotify"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// --- Review mode (R7.6) -------------------------------------------------------

// reviewExts are the files a review shows (plugins_tab.py scan_for_review).
var reviewExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".bmp": true}

// reviewDebounce is how long the directory watcher waits for events to
// settle before re-scanning.
const reviewDebounce = 500 * time.Millisecond

/*
reviewModel is the image review of one plugin's directory: the files newest
first, the one on screen, and the ones marked for the blacklist. ←/→ move,
Space marks; a marked image is drawn with a red border and both diagonals;
Apply hands the marked paths to the plugin's process_blacklist action.

The model is separate from the widgets so the keyboard rules can be tested
without a window; the widgets are rebuilt with the section and re-attached.
*/
type reviewModel struct {
	plugin string
	dir    string
	images []string
	index  int
	marked map[int]bool
	// err is why the directory could not be read, shown in place of an image.
	err string

	// live widgets, nil when the section is not on screen
	preview  *canvas.Image
	info     *widget.Label
	applyBtn *widget.Button

	// cache holds the scaled previews; seq drops a load that finished after
	// the user moved on.
	cache   *previewCache
	mu      sync.Mutex
	seq     int
	watcher *fsnotify.Watcher
	stop    chan struct{}
}

// reviewFor returns the review for this plugin, scanning its directory when
// the section is first shown or the directory changed (plugins_tab.py
// scanned on every tab click; the watcher covers the rest).
func (u *ui) reviewFor(name string, info core.PluginInfo, block map[string]any) *reviewModel {
	dir := config.ExpandPath(pluginDir(info, block))
	if u.review == nil || u.review.plugin != name || u.review.dir != dir {
		if u.review != nil {
			u.review.detach()
		}
		u.review = &reviewModel{plugin: name, dir: dir, marked: map[int]bool{}, cache: newPreviewCache()}
		u.review.scan()
	}
	return u.review
}

// scan re-reads the directory: image files sorted by mtime, newest first. The
// marks are cleared, as they were in the Python: they index a list that has
// just changed.
func (r *reviewModel) scan() {
	r.images, r.err = scanReviewDir(r.dir)
	r.index = 0
	r.marked = map[int]bool{}
	if r.cache != nil {
		keep := make(map[string]bool, len(r.images))
		for _, p := range r.images {
			keep[p] = true
		}
		r.cache.forget(keep)
	}
}

// scanReviewDir lists the reviewable images in dir, newest first.
func scanReviewDir(dir string) ([]string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Sprintf("Directory not found: %s", dir)
	}
	type file struct {
		path  string
		mtime time.Time
	}
	var files []file
	for _, e := range entries {
		if e.IsDir() || !reviewExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue // vanished between the listing and the stat
		}
		files = append(files, file{filepath.Join(dir, e.Name()), fi.ModTime()})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.path)
	}
	return out, ""
}

// The keyboard rules (plugins_tab.py keyPressEvent).
func (r *reviewModel) next() bool {
	if r.index+1 >= len(r.images) {
		return false
	}
	r.index++
	return true
}

func (r *reviewModel) prev() bool {
	if r.index == 0 {
		return false
	}
	r.index--
	return true
}

func (r *reviewModel) toggle() {
	if len(r.images) == 0 {
		return
	}
	if r.marked[r.index] {
		delete(r.marked, r.index)
		return
	}
	r.marked[r.index] = true
}

func (r *reviewModel) markedCount() int { return len(r.marked) }

// markedPaths are the marked images, in review order.
func (r *reviewModel) markedPaths() []string {
	idx := make([]int, 0, len(r.marked))
	for i := range r.marked {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		if i < len(r.images) {
			out = append(out, r.images[i])
		}
	}
	return out
}

// current is the image on screen, or "".
func (r *reviewModel) current() string {
	if r.index < 0 || r.index >= len(r.images) {
		return ""
	}
	return r.images[r.index]
}

// infoText is the panel beside the preview: position, file, age, mark.
func (r *reviewModel) infoText(now time.Time) string {
	if r.err != "" {
		return r.err
	}
	if len(r.images) == 0 {
		return "No images found for review."
	}
	path := r.current()
	when := "Unknown"
	if fi, err := os.Stat(path); err == nil {
		when = fmt.Sprintf("%s (%s)", relativeTime(fi.ModTime(), now), fi.ModTime().Format("2006-01-02 15:04"))
	}
	text := fmt.Sprintf("Image %d of %d\nFile: %s\nDate: %s", r.index+1, len(r.images), filepath.Base(path), when)
	if r.marked[r.index] {
		text += "\n[MARKED FOR DELETION]"
	}
	return text + "\n\n←/→: navigate · Space: mark/unmark"
}

// relativeTime is get_relative_time from plugins_tab.py.
func relativeTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "Just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "min")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 7*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	}
	return t.Format("2006-01-02")
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s ago", n, unit)
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}

// --- the overlay ---------------------------------------------------------------

// markColor is the review's "marked" red.
var markColor = color.RGBA{R: 255, A: 255}

/*
markOverlay paints the "marked for deletion" overlay the Python drew with
QPainter: a red border and both diagonals, pen width max(5, 2 % of the shorter
side). It composites onto a copy; the file is not touched until Apply.
*/
func markOverlay(src image.Image) *image.RGBA {
	img := imaging.ToRGB(src)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	pen := max(5, min(w, h)*2/100)
	// The border is four strips, not a test of every pixel.
	fill := func(r image.Rectangle) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetRGBA(x, y, markColor)
			}
		}
	}
	fill(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+pen))
	fill(image.Rect(b.Min.X, b.Max.Y-pen, b.Max.X, b.Max.Y))
	fill(image.Rect(b.Min.X, b.Min.Y, b.Min.X+pen, b.Max.Y))
	fill(image.Rect(b.Max.X-pen, b.Min.Y, b.Max.X, b.Max.Y))
	drawThickLine(img, 0, 0, w-1, h-1, pen)
	drawThickLine(img, w-1, 0, 0, h-1, pen)
	return img
}

// drawThickLine draws a line of the given thickness with Bresenham steps and
// a square brush; exact enough for an overlay whose job is to be unmissable.
func drawThickLine(img *image.RGBA, x0, y0, x1, y1, pen int) {
	b := img.Bounds()
	half := pen / 2
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		for oy := -half; oy <= half; oy++ {
			for ox := -half; ox <= half; ox++ {
				px, py := b.Min.X+x0+ox, b.Min.Y+y0+oy
				if px >= b.Min.X && py >= b.Min.Y && px < b.Max.X && py < b.Max.Y {
					img.SetRGBA(px, py, markColor)
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// --- widgets -------------------------------------------------------------------

// previewHeight is the preview pane's fixed height. It has a tab to itself,
// so it can be tall.
const previewHeight = 640

// widget builds the review pane: the preview and the info panel side by side.
func (r *reviewModel) widget(u *ui) fyne.CanvasObject {
	r.preview = canvas.NewImageFromResource(nil)
	r.preview.FillMode = canvas.ImageFillContain
	r.info = widget.NewLabel("")
	r.info.Wrapping = fyne.TextWrapWord
	r.draw(u)
	r.watch(u)
	body := container.NewBorder(nil, nil, nil, fixedWidth(r.info, 260), r.preview)
	return card("Review", fixedHeight(body, previewHeight))
}

// draw shows the current image (decoded off the UI thread) and the panel.
func (r *reviewModel) draw(u *ui) {
	if r.info != nil {
		r.info.SetText(r.infoText(time.Now()))
	}
	if r.applyBtn != nil {
		r.applyBtn.SetText(fmt.Sprintf("Apply blacklist (%d)", r.markedCount()))
		if r.markedCount() == 0 || u.working() {
			r.applyBtn.Disable()
		} else {
			r.applyBtn.Enable()
		}
	}
	path := r.current()
	if r.preview == nil {
		return
	}
	if path == "" {
		r.preview.Resource, r.preview.Image = nil, nil
		r.preview.Refresh()
		return
	}
	r.mu.Lock()
	r.seq++
	seq := r.seq
	r.mu.Unlock()
	marked := r.marked[r.index]
	if r.cache == nil {
		r.cache = newPreviewCache()
	}
	cache := r.cache
	// Captured on the UI thread: the goroutine must not read the model.
	idx, images := r.index, r.images
	load := func() {
		img, err := cache.get(path)
		var shown image.Image
		if err == nil {
			shown = img
			if marked {
				shown = markOverlay(img)
			}
		}
		fyne.Do(func() {
			r.mu.Lock()
			stale := seq != r.seq
			r.mu.Unlock()
			if stale || r.preview == nil {
				return
			}
			if err != nil {
				r.preview.Image = nil
				if r.info != nil {
					r.info.SetText("Error loading: " + filepath.Base(path))
				}
			} else {
				r.preview.Image = shown
			}
			r.preview.Refresh()
		})
		cache.prefetch(idx, images)
	}
	if !u.onScreen() {
		load()
		return
	}
	go load()
}

// prefetch scales the neighbours of image idx so the next arrow key lands on
// a cached frame. Off the UI thread; takes copies, never the model.
func (c *previewCache) prefetch(idx int, images []string) {
	for d := 1; d <= prefetchEach; d++ {
		for _, i := range []int{idx + d, idx - d} {
			if i < 0 || i >= len(images) || c.has(images[i]) {
				continue
			}
			_, _ = c.get(images[i])
		}
	}
}

// handleKey applies ←/→/Space; true when the key was one of ours.
func (r *reviewModel) handleKey(u *ui, key fyne.KeyName) bool {
	if len(r.images) == 0 {
		return false
	}
	switch key {
	case fyne.KeyLeft:
		r.prev()
	case fyne.KeyRight:
		r.next()
	case fyne.KeySpace:
		r.toggle()
	default:
		return false
	}
	r.draw(u)
	return true
}

// onTypedKey is the window's key handler: F5 reloads; the arrows and Space go
// to the review when a plugin section is on screen.
func (u *ui) onTypedKey(e *fyne.KeyEvent) {
	if e.Name == fyne.KeyF5 {
		u.invalidate()
		return
	}
	if u.review == nil || u.review.preview == nil {
		return
	}
	if _, isPlugin := pluginForTitle(u.currentTitle()); !isPlugin || u.pluginTab != 1 {
		return // the keys belong to the Review tab; on Configuration they would move an unseen image
	}
	u.review.handleKey(u, e.Name)
}

/*
watch re-scans when the directory changes, 500 ms after the events settle
(plugins_tab.py on_directory_changed). A plugin run writes several files a
second, and one scan per file would decode the newest image over and over.
*/
func (r *reviewModel) watch(u *ui) {
	if r.watcher != nil || r.dir == "" || !u.onScreen() {
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil || w.Add(r.dir) != nil {
		if w != nil {
			_ = w.Close()
		}
		return
	}
	r.watcher = w
	r.stop = make(chan struct{})
	stop := r.stop
	go func() {
		var timer *time.Timer
		var fire <-chan time.Time
		for {
			select {
			case <-stop:
				if timer != nil {
					timer.Stop()
				}
				return
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(reviewDebounce)
				fire = timer.C
			case <-fire:
				fire = nil
				fyne.Do(func() {
					r.scan()
					r.draw(u)
				})
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// detach stops the watcher and forgets the live widgets.
func (r *reviewModel) detach() {
	if r.stop != nil {
		close(r.stop)
		r.stop = nil
	}
	if r.watcher != nil {
		_ = r.watcher.Close()
		r.watcher = nil
	}
	r.preview, r.info, r.applyBtn = nil, nil, nil
}

/*
applyBlacklist runs the plugin with action=process_blacklist over the marked
paths (R7.6): the plugin hashes each, stores the hash with a thumbnail and
deletes the file. force=true, as the Python sent, so an interval-gated plugin
does not skip the action.
*/
func (u *ui) applyBlacklist(name string, rv *reviewModel) {
	targets := rv.markedPaths()
	if len(targets) == 0 {
		return
	}
	u.perform(fmt.Sprintf("Blacklisting %d image(s)…", len(targets)), func(ctx context.Context) error {
		_, err := core.RunPlugin(ctx, core.RunPluginRequest{
			Request: u.requestWithEvents(u.activity.events()),
			Name:    name,
			Override: map[string]any{
				"action":  "process_blacklist",
				"targets": anySlice(targets),
				"force":   true,
			},
		})
		if err != nil {
			return err
		}
		fyne.Do(func() {
			rv.scan()
			u.blOK = false
		})
		u.ok(fmt.Sprintf("Blacklisted and removed %d image(s).", len(targets)))
		return nil
	})
}

func anySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
