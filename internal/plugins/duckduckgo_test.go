package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
	"github.com/ushineko/clockwork-orange/internal/store"
)

func TestDDGFilenameMatchesThePythonMD5Golden(t *testing.T) {
	var golden map[string]string
	loadGolden(t, "ddg_filenames.json", &golden)
	require.NotEmpty(t, golden)
	for u, want := range golden {
		require.Equal(t, want, DDGFilename(u), "url %s", u)
	}
}

func TestExtractVqdTriesTheAttributeFormThenTheJSONForm(t *testing.T) {
	require.Equal(t, "4-123456789", extractVqd([]byte(`<script>x = "vqd='4-123456789'";</script>`)))
	require.Equal(t, "4-987", extractVqd([]byte(`vqd="4-987"`)))
	require.Equal(t, "3-55-66", extractVqd([]byte(`{"vqd":"3-55-66"}`)))
	require.Equal(t, "4-1", extractVqd([]byte(`vqd="4-1" and later "vqd":"9-9"`)), "attribute form wins")
	require.Empty(t, extractVqd([]byte(`<html>nothing here</html>`)))
	require.Empty(t, extractVqd([]byte(`vqd="abc"`)), "token must start with a digit and a dash")
}

func TestFilterResultsDedupesAndRejectsSmallReportedDimensionsOnly(t *testing.T) {
	results := []map[string]any{
		{"image": "https://a/big.jpg", "width": float64(3840), "height": float64(2160)},
		{"image": "https://a/big.jpg", "width": float64(3840), "height": float64(2160)}, // dup
		{"image": "https://a/small.jpg", "width": float64(1280), "height": float64(720)},
		{"image": "https://a/tall.jpg", "width": float64(1920), "height": float64(1000)},
		{"image": "https://a/nodims.jpg"},
		{"image": "https://a/strdims.jpg", "width": "1920", "height": "1080"},
		{"image": "https://a/baddims.jpg", "width": "wide", "height": float64(10)}, // int() raised: both zeroed, passes
		{"image": "https://a/onlyw.jpg", "width": float64(100)},                    // height missing: passes
		{"image": "", "width": float64(4000), "height": float64(4000)},
		{"width": float64(4000), "height": float64(4000)},
	}
	require.Equal(t, []string{
		"https://a/big.jpg", "https://a/nodims.jpg", "https://a/strdims.jpg", "https://a/baddims.jpg", "https://a/onlyw.jpg",
	}, filterResults(results))
	require.Equal(t, []string{}, filterResults(nil))
}

// fakeDDG serves the landing page, i.js and image files.
type fakeDDG struct {
	srv     *httptest.Server
	landing string
	results []map[string]any
	images  map[string][]byte

	mu       sync.Mutex
	requests []*http.Request
}

func newFakeDDG(t *testing.T) *fakeDDG {
	t.Helper()
	f := &fakeDDG{images: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(f.landing))
	})
	mux.HandleFunc("/i.js", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.results})
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		body, ok := f.images[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	f.landing = `<html><script>nrj('/i.js?q=x&vqd=4-111222333444'); var vqd='4-111222333444';</script></html>`
	return f
}

func (f *fakeDDG) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Clone(context.Background()))
}

func (f *fakeDDG) addImage(name string, body []byte, dims ...int) string {
	p := "/img/" + name
	f.images[p] = body
	u := f.srv.URL + p
	r := map[string]any{"image": u}
	if len(dims) == 2 {
		r["width"], r["height"] = dims[0], dims[1]
	}
	f.results = append(f.results, r)
	return u
}

func (f *fakeDDG) requestsFor(path string) []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*http.Request
	for _, r := range f.requests {
		if r.URL.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeDDG) plugin(h *store.History, b *store.Blacklist) *duckduckgo {
	p := newDuckDuckGo(Deps{History: h, Blacklist: b, HTTP: f.srv.Client(), Now: func() time.Time { return fixedNow }})
	p.baseURL = f.srv.URL
	return p
}

func TestDuckDuckGoRunScrapesDownloadsProcessesAndRecordsA4KJPEG(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	bigURL := f.addImage("big.png", syntheticPNG(t, 1920, 1080), 1920, 1080)
	smallReportedURL := f.addImage("small-reported.png", syntheticPNG(t, 1920, 1080), 800, 600) // filtered before download
	smallActualURL := f.addImage("small-actual.png", syntheticPNG(t, 800, 600))                 // no dims: fetched then rejected
	missingURL := f.addImage("missing.png", nil)                                                // 404 after registration
	delete(f.images, "/img/missing.png")

	dir := filepath.Join(t.TempDir(), "ddg")
	rec := newRecorder()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{
		"download_dir": dir,
		"query":        []any{map[string]any{"term": "4k nature wallpapers", "enabled": true}},
		"limit":        10,
		"max_files":    50,
	}, rec.events())
	require.NoError(t, err)
	require.Equal(t, Result{Path: dir}, res)

	// One 3840×2160 JPEG named md5(url).jpg, plus the marker.
	want := DDGFilename(bigURL)
	require.Equal(t, []string{".last_run", want}, dirNames(t, dir))
	img, format, err := imaging.DecodeFile(filepath.Join(dir, want))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	require.Equal(t, 3840, img.Bounds().Dx())
	require.Equal(t, 2160, img.Bounds().Dy())

	// History: only the saved image.
	for u, wantSeen := range map[string]bool{bigURL: true, smallReportedURL: false, smallActualURL: false, missingURL: false} {
		seen, err := h.SeenURL(u)
		require.NoError(t, err)
		require.Equal(t, wantSeen, seen, "history for %s", u)
	}

	// Network: landing then i.js with the exact params and headers.
	landings := f.requestsFor("/")
	require.Len(t, landings, 1)
	lq := landings[0].URL.Query()
	require.Equal(t, "4k nature wallpapers", lq.Get("q"))
	require.Equal(t, "images", lq.Get("iax"))
	require.Equal(t, "images", lq.Get("ia"))
	ijs := f.requestsFor("/i.js")
	require.Len(t, ijs, 1)
	q := ijs[0].URL.Query()
	require.Equal(t, "us-en", q.Get("l"))
	require.Equal(t, "json", q.Get("o"))
	require.Equal(t, "4k nature wallpapers", q.Get("q"))
	require.Equal(t, "4-111222333444", q.Get("vqd"))
	require.Equal(t, ",,,size:Large,,", q.Get("f"))
	require.Equal(t, "1", q.Get("p"))
	require.Equal(t, "https://duckduckgo.com/", ijs[0].Header.Get("Referer"))
	for _, r := range f.requests {
		require.Equal(t, ddgUserAgent, r.Header.Get("User-Agent"), "%s", r.URL)
		require.Equal(t, "en-US,en;q=0.9", r.Header.Get("Accept-Language"), "%s", r.URL)
	}
	require.Empty(t, landings[0].Header.Get("Referer"), "Referer is only sent to i.js")
	require.Len(t, f.requestsFor("/img/big.png"), 1)
	require.Empty(t, f.requestsFor("/img/small-reported.png"), "reported dimensions filter before download")
	require.Len(t, f.requestsFor("/img/small-actual.png"), 1)
	require.Len(t, f.requestsFor("/img/missing.png"), 1)

	// Events.
	require.Equal(t, []string{filepath.Join(dir, want)}, rec.saved)
	// 0 (scrape), then per candidate over 3 urls (0, 30, 60) with "Skipped"
	// repeats for the two rejects, then 95 and 100.
	require.Equal(t, []int{0, 0, 30, 30, 60, 60, 95, 100}, rec.pcts())
	require.Equal(t, "Scraping '4k nature wallpapers'...", rec.progress[0].msg)
	require.Equal(t, "Checking candidate 1 for image 1/10...", rec.progress[1].msg)
	require.Equal(t, "Checking candidate 2 for image 2/10...", rec.progress[2].msg)
	require.Equal(t, "Skipped low-quality/duplicate image...", rec.progress[3].msg)
	require.True(t, rec.hasLogContaining("Found 3 potential images for '4k nature wallpapers'"))
	require.True(t, rec.hasLogContaining("Rejected low-res image: 800x600 (needs 1920x1080)"))
	require.True(t, rec.hasLogContaining("Processing image: 1920x1080"))
	require.True(t, rec.hasLogContaining("Saved processed image to "+filepath.Join(dir, want)))
}

func TestDuckDuckGoBlacklistedContentIsDeletedAndNotRecorded(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	u := f.addImage("big.png", syntheticPNG(t, 1920, 1080), 1920, 1080)
	dir := t.TempDir()
	p := f.plugin(h, bl)

	// First run saves the processed JPEG; take its SHA-256 as the blacklist key.
	_, err := p.Run(context.Background(), map[string]any{"download_dir": dir, "force": true}, events.Events{})
	require.NoError(t, err)
	saved := filepath.Join(dir, DDGFilename(u))
	hash, err := imaging.SHA256File(saved)
	require.NoError(t, err)
	require.NoError(t, bl.Add(hash, "test", ""))
	require.NoError(t, os.Remove(saved))
	require.NoError(t, h.Clear())

	rec := newRecorder()
	_, err = p.Run(context.Background(), map[string]any{"download_dir": dir, "force": true}, rec.events())
	require.NoError(t, err)
	require.NoFileExists(t, saved)
	seen, err := h.SeenURL(u)
	require.NoError(t, err)
	require.False(t, seen)
	require.Empty(t, rec.saved)
	require.True(t, rec.hasLogContaining("Image is blacklisted. Removing "+DDGFilename(u)))
}

func TestDuckDuckGoDuplicateContentUnderANewURLIsDeletedButTheURLIsRecorded(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	first := f.addImage("one.png", syntheticPNG(t, 1920, 1080), 1920, 1080)
	dir := t.TempDir()
	p := f.plugin(h, bl)

	_, err := p.Run(context.Background(), map[string]any{"download_dir": dir, "force": true}, events.Events{})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, DDGFilename(first)))

	// Same bytes served under a second URL.
	second := f.addImage("two.png", f.images["/img/one.png"], 1920, 1080)
	rec := newRecorder()
	_, err = p.Run(context.Background(), map[string]any{"download_dir": dir, "force": true}, rec.events())
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, DDGFilename(second)))
	seen, err := h.SeenURL(second)
	require.NoError(t, err)
	require.True(t, seen, "the duplicate's URL is recorded so it is not fetched again")
	require.Empty(t, rec.saved)
	require.True(t, rec.hasLogContaining("Image content already in history. Skipping."))

	// Third run: neither URL is fetched any more.
	f.mu.Lock()
	f.requests = nil
	f.mu.Unlock()
	_, err = p.Run(context.Background(), map[string]any{"download_dir": dir, "force": true}, events.Events{})
	require.NoError(t, err)
	require.Empty(t, f.requestsFor("/img/one.png"))
	require.Empty(t, f.requestsFor("/img/two.png"))
}

func TestDuckDuckGoIntervalSkipReturnsSuccessWithTheDirectory(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	dir := t.TempDir()
	writeLastRun(t, dir, fixedNow.Add(-time.Minute))

	rec := newRecorder()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir}, rec.events())
	require.NoError(t, err)
	require.Equal(t, Result{Path: dir}, res)
	require.Empty(t, f.requests)
	require.True(t, rec.hasLogContaining("Skipping run (interval: daily)"))
}

func TestDuckDuckGoRunWithNoResultsStillSucceedsBecauseTheMarkerFillsTheDirectory(t *testing.T) {
	// Python's "No images found or downloaded" error checked glob("*") after
	// writing .last_run, and pathlib's glob matched dot-files, so the branch
	// was unreachable on a healthy filesystem. Ported as-is.
	h, bl := testStores(t)
	f := newFakeDDG(t)
	dir := t.TempDir()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir}, events.Events{})
	require.NoError(t, err)
	require.Equal(t, dir, res.Path)
	require.Equal(t, []string{".last_run"}, dirNames(t, dir))
}

func TestDuckDuckGoMissingVqdOrNonJSONResultsYieldNoURLsAndKeepRunning(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	f.landing = "<html>no token here</html>"
	dir := t.TempDir()
	rec := newRecorder()
	_, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "query": "x"}, rec.events())
	require.NoError(t, err)
	require.True(t, rec.hasLogContaining("Failed to extract vqd token for 'x'"))
	require.Empty(t, f.requestsFor("/i.js"))
	require.True(t, rec.hasLogContaining("Found 0 potential images for 'x'"))

	// Non-JSON i.js body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i.js" {
			_, _ = w.Write([]byte("<html>blocked</html>"))
			return
		}
		_, _ = w.Write([]byte(`vqd="4-1"`))
	}))
	t.Cleanup(srv.Close)
	p := newDuckDuckGo(Deps{History: h, Blacklist: bl, HTTP: srv.Client(), Now: time.Now})
	p.baseURL = srv.URL
	rec = newRecorder()
	urls := p.scrapeImageURLs(context.Background(), "y", rec.events())
	require.Empty(t, urls)
	require.True(t, rec.hasLogContaining("Non-JSON response for 'y'"))
}

func TestDuckDuckGoLimitStopsAfterEnoughSavesPerQuery(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	png := syntheticPNG(t, 1920, 1080)
	f.addImage("a.png", png, 1920, 1080)
	f.addImage("b.png", png, 1920, 1080) // same bytes as a: duplicate content, does not count
	f.addImage("c.png", syntheticPNG(t, 2000, 1200), 2000, 1200)
	f.addImage("d.png", syntheticPNG(t, 2560, 1440), 2560, 1440)
	dir := t.TempDir()

	rec := newRecorder()
	_, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "limit": 2}, rec.events())
	require.NoError(t, err)
	require.Len(t, rec.saved, 2)
	require.Empty(t, f.requestsFor("/img/d.png"), "the limit stops the walk before the fourth candidate")
}

func TestDuckDuckGoResetAndRetentionOnlyTouchJPEGs(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	f.addImage("a.png", syntheticPNG(t, 1920, 1080), 1920, 1080)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old1.jpg"), []byte("1"))
	writeFile(t, filepath.Join(dir, "old2.jpg"), []byte("2"))
	writeFile(t, filepath.Join(dir, "notes.txt"), []byte("keep"))
	past := fixedNow.Add(-72 * time.Hour)
	for _, n := range []string{"old1.jpg", "old2.jpg", "notes.txt"} {
		require.NoError(t, os.Chtimes(filepath.Join(dir, n), past, past))
	}

	// Retention: max 1 jpg → the two old jpgs go, notes.txt stays.
	_, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "max_files": 1, "force": true}, events.Events{})
	require.NoError(t, err)
	names := dirNames(t, dir)
	require.Contains(t, names, "notes.txt")
	require.Contains(t, names, ".last_run")
	require.NotContains(t, names, "old1.jpg")
	require.NotContains(t, names, "old2.jpg")
	require.Len(t, names, 3)

	// Reset wipes everything, including notes.txt, before downloading anew.
	require.NoError(t, h.Clear())
	rec := newRecorder()
	_, err = f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "reset": true, "force": true}, rec.events())
	require.NoError(t, err)
	require.NotContains(t, dirNames(t, dir), "notes.txt")
	require.Equal(t, 0, rec.progress[0].pct)
	require.Equal(t, "Resetting directory...", rec.progress[0].msg)
}

func TestDuckDuckGoProcessBlacklistActionRunsBeforeIntervalCheck(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeDDG(t)
	dir := t.TempDir()
	writeLastRun(t, dir, fixedNow)
	victim := filepath.Join(dir, "v.jpg")
	writeFile(t, victim, []byte("v"))

	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{
		"download_dir": dir, "action": "process_blacklist", "targets": []any{victim},
	}, events.Events{})
	require.NoError(t, err)
	require.Equal(t, Result{Message: "Blacklist processed"}, res)
	require.NoFileExists(t, victim)
	items, err := bl.Items()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "duckduckgo_images", items[0].Source)
	require.Empty(t, f.requests)
}
