package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/store"
)

func TestWallhavenBuildAPIParamsMatchesThePythonGoldenTable(t *testing.T) {
	var golden map[string]map[string]any
	loadGolden(t, "wallhaven_params.json", &golden)

	// The same inputs tests/golden/capture.py fed _build_api_params.
	cases := map[string]map[string]any{
		"defaults":              {},
		"toplist":               {"sorting": "toplist", "top_range": "1w"},
		"toplist_default_range": {"sorting": "toplist"},
		"no_people_nsfw":        {"category_people": false, "purity_nsfw": true, "api_key": "KEY"},
		"empty_optionals":       {"resolutions": "  ", "atleast": "", "ratios": " "},
		"all_optionals":         {"resolutions": "1920x1080,2560x1440", "atleast": "3840x2160", "ratios": "16x9,21x9"},
	}
	require.Len(t, golden, len(cases), "golden table and inputs must cover the same cases")

	for name, cfg := range cases {
		want, ok := golden[name]
		require.True(t, ok, "golden case %s", name)
		got := buildAPIParams(cfg, "landscape")

		flat := map[string]string{}
		for k, vs := range got {
			require.Len(t, vs, 1, "case %s key %s", name, k)
			flat[k] = vs[0]
		}
		wantFlat := map[string]string{}
		for k, v := range want {
			wantFlat[k] = fmt.Sprint(v) // page is the JSON number 1
		}
		require.Equal(t, wantFlat, flat, "case %s", name)
	}
}

func TestWallhavenBuildAPIParamsAcceptsStringBooleansFromYAML(t *testing.T) {
	got := buildAPIParams(map[string]any{
		"category_general": "false", "category_anime": "no", "purity_sketchy": "yes", "purity_nsfw": 1,
	}, "q")
	require.Equal(t, "001", got.Get("categories"))
	require.Equal(t, "111", got.Get("purity"))
}

func TestWallhavenRedactedParamsNeverContainTheAPIKey(t *testing.T) {
	params := buildAPIParams(map[string]any{"api_key": "SECRET-KEY-123"}, "landscape")
	line := redactParams(params)
	require.NotContains(t, line, "SECRET-KEY-123")
	require.Contains(t, line, "apikey=%2A%2A%2A")
	require.Contains(t, line, "q=landscape")

	empty := redactParams(buildAPIParams(map[string]any{}, "q"))
	require.Contains(t, empty, "apikey=&")
}

// fakeWallhaven serves a search endpoint and the image files it references.
type fakeWallhaven struct {
	srv    *httptest.Server
	images map[string][]byte // path -> bytes
	items  []map[string]any
	status int

	mu       sync.Mutex
	requests []*http.Request
}

func newFakeWallhaven(t *testing.T) *fakeWallhaven {
	t.Helper()
	f := &fakeWallhaven{images: map[string][]byte{}, status: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if f.status != http.StatusOK {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte("nope"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": f.items})
	})
	mux.HandleFunc("/full/", func(w http.ResponseWriter, r *http.Request) {
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
	return f
}

func (f *fakeWallhaven) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clone := r.Clone(context.Background())
	f.requests = append(f.requests, clone)
}

// addItem registers an image at /full/<name> and a search result pointing
// at it.
func (f *fakeWallhaven) addItem(id, name string, body []byte) string {
	p := "/full/" + name
	f.images[p] = body
	u := f.srv.URL + p
	f.items = append(f.items, map[string]any{"id": id, "path": u})
	return u
}

func (f *fakeWallhaven) requestsFor(path string) []*http.Request {
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

func (f *fakeWallhaven) plugin(h *store.History, b *store.Blacklist) *wallhaven {
	p := newWallhaven(Deps{History: h, Blacklist: b, HTTP: f.srv.Client(), Now: func() time.Time { return fixedNow }})
	p.searchURL = f.srv.URL + "/search"
	return p
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestWallhavenRunDownloadsNewItemsAndSkipsHistoryBlacklistAndDuplicates(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)

	fresh := []byte("fresh-image-bytes")
	freshURL := f.addItem("aaa111", "wallhaven-aaa111.jpg", fresh)
	seenURL := f.addItem("bbb222", "wallhaven-bbb222.png", []byte("never fetched"))
	banned := []byte("banned-image-bytes")
	bannedURL := f.addItem("ccc333", "wallhaven-ccc333.jpg", banned)
	dupURL := f.addItem("ddd444", "wallhaven-ddd444.jpg", fresh) // same bytes as fresh

	// bbb222 is already in history by URL.
	seed := filepath.Join(t.TempDir(), "seed.png")
	writeFile(t, seed, []byte("seed"))
	added, err := h.AddEntry(seenURL, seed, "wallhaven")
	require.NoError(t, err)
	require.True(t, added)
	// ccc333 is blacklisted by SHA-256 of its bytes.
	require.NoError(t, bl.Add(sha256Hex(banned), "test", ""))

	dir := filepath.Join(t.TempDir(), "dl")
	rec := newRecorder()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{
		"download_dir": dir,
		"query":        "landscape",
		"api_key":      "KEY",
		"limit":        10,
		"max_files":    100,
	}, rec.events())
	require.NoError(t, err)
	require.Equal(t, Result{Path: dir}, res)

	// Files on disk: only the fresh image and the marker survive.
	require.Equal(t, []string{".last_run", "wallhaven-aaa111.jpg"}, dirNames(t, dir))
	got, err := os.ReadFile(filepath.Join(dir, "wallhaven-aaa111.jpg"))
	require.NoError(t, err)
	require.Equal(t, fresh, got)

	// History rows: fresh recorded, seen unchanged, banned NOT recorded,
	// duplicate URL recorded so it is not fetched again.
	for u, want := range map[string]bool{freshURL: true, seenURL: true, bannedURL: false, dupURL: true} {
		seen, err := h.SeenURL(u)
		require.NoError(t, err)
		require.Equal(t, want, seen, "history for %s", u)
	}
	stats, err := h.Stats()
	require.NoError(t, err)
	require.Equal(t, 3, stats.TotalRecords)

	// Network: search once with the built params; seen URL never downloaded.
	searches := f.requestsFor("/search")
	require.Len(t, searches, 1)
	q := searches[0].URL.Query()
	require.Equal(t, "landscape", q.Get("q"))
	require.Equal(t, "KEY", q.Get("apikey"))
	require.Equal(t, "relevance", q.Get("sorting"))
	require.Equal(t, "desc", q.Get("order"))
	require.Equal(t, "1", q.Get("page"))
	require.Equal(t, "111", q.Get("categories"))
	require.Equal(t, "100", q.Get("purity"))
	require.Empty(t, q.Get("seed"))
	require.Len(t, f.requestsFor("/full/wallhaven-aaa111.jpg"), 1)
	require.Empty(t, f.requestsFor("/full/wallhaven-bbb222.png"))
	require.Len(t, f.requestsFor("/full/wallhaven-ccc333.jpg"), 1)
	require.Len(t, f.requestsFor("/full/wallhaven-ddd444.jpg"), 1)

	// Events: one saved image; progress 0 (search), 0/22/45/67 (items), 95, 100.
	require.Equal(t, []string{filepath.Join(dir, "wallhaven-aaa111.jpg")}, rec.saved)
	require.Equal(t, []int{0, 0, 22, 45, 67, 95, 100}, rec.pcts())
	require.Equal(t, "Searching API for 'landscape'...", rec.progress[0].msg)
	require.Equal(t, "Processing image 1/4...", rec.progress[1].msg)
	require.Equal(t, "Cleaning up old files...", rec.progress[5].msg)
	require.Equal(t, "Done!", rec.progress[6].msg)
	require.True(t, rec.hasLogContaining("Blacklisted image detected. Removing."))
	require.True(t, rec.hasLogContaining("Duplicate content detected. Removing."))
	require.True(t, rec.hasLogContaining("Saved wallhaven-aaa111.jpg"))
	require.True(t, rec.hasLogContaining("Found 4 wallpapers for 'landscape'"))

	// The API key never reaches the log.
	for _, l := range rec.logs {
		require.NotContains(t, l, "apikey=KEY")
	}

	// .last_run holds the injected clock.
	require.False(t, ShouldRun(dir, "daily", fixedNow.Add(time.Hour)))
	require.True(t, ShouldRun(dir, "daily", fixedNow.Add(25*time.Hour)))
}

func TestWallhavenRunSkipsWhenTheIntervalHasNotElapsedAndForceOverridesIt(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	f.addItem("x1", "wallhaven-x1.jpg", []byte("x"))
	dir := t.TempDir()
	writeLastRun(t, dir, fixedNow.Add(-time.Hour))

	rec := newRecorder()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "interval": "Daily"}, rec.events())
	require.NoError(t, err)
	require.Equal(t, Result{Path: dir}, res)
	require.Empty(t, f.requests, "no network traffic on an interval skip")
	require.True(t, rec.hasLogContaining("Skipping run (interval: daily)"))
	require.Empty(t, rec.pcts())

	_, err = f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "interval": "Daily", "force": true}, events.Events{})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "wallhaven-x1.jpg"))
}

func TestWallhavenRunContinuesWithTheNextQueryAfterANon200Search(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	f.status = http.StatusTooManyRequests
	dir := t.TempDir()

	rec := newRecorder()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{
		"download_dir": dir,
		"query":        []any{map[string]any{"term": "a"}, map[string]any{"term": "b"}},
	}, rec.events())
	require.NoError(t, err, "API failures are per query; the run still succeeds with the directory")
	require.Equal(t, dir, res.Path)
	require.Len(t, f.requestsFor("/search"), 2, "both queries were attempted")
	require.True(t, rec.hasLogContaining("API Error for 'a': HTTP 429: nope"))
	require.True(t, rec.hasLogContaining("API Error for 'b': HTTP 429: nope"))
	require.Equal(t, []int{0, 45, 95, 100}, rec.pcts())
	require.FileExists(t, filepath.Join(dir, ".last_run"), "the marker is stamped even when every search failed")
}

func TestWallhavenRandomSortingSendsASixHexSeed(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	dir := t.TempDir()
	_, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "sorting": "random"}, events.Events{})
	require.NoError(t, err)
	searches := f.requestsFor("/search")
	require.Len(t, searches, 1)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{6}$`), searches[0].URL.Query().Get("seed"))
}

func TestWallhavenLimitAppliesPerQueryNotPerRun(t *testing.T) {
	// Known quirk (spec deviations preamble): two queries with limit 1 yield
	// two downloads.
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	f.addItem("p1", "wallhaven-p1.jpg", []byte("1"))
	f.addItem("p2", "wallhaven-p2.jpg", []byte("2"))
	dir := t.TempDir()

	rec := newRecorder()
	_, err := f.plugin(h, bl).Run(context.Background(), map[string]any{
		"download_dir": dir, "query": "a, b", "limit": 1,
	}, rec.events())
	require.NoError(t, err)
	// First query saves p1 and stops at the limit; second query sees p1 in
	// history and saves p2.
	require.Equal(t, []string{".last_run", "wallhaven-p1.jpg", "wallhaven-p2.jpg"}, dirNames(t, dir))
	require.Len(t, rec.saved, 2)
}

func TestWallhavenResetWipesTheDirectoryBeforeDownloading(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	f.addItem("n1", "wallhaven-n1.jpg", []byte("n"))
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "stale.jpg"), []byte("stale"))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o750))
	writeLastRun(t, dir, fixedNow.Add(-time.Minute)) // would otherwise skip

	// reset does not bypass the interval on its own...
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "reset": true}, events.Events{})
	require.NoError(t, err)
	require.Equal(t, dir, res.Path)
	// ...but the wipe removed the marker, so the interval check now passes.
	require.Equal(t, []string{".last_run", "wallhaven-n1.jpg"}, dirNames(t, dir))
}

func TestWallhavenRetentionRunsAfterDownloadingAndCountsEveryFile(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	f.addItem("r1", "wallhaven-r1.jpg", []byte("r1"))
	dir := t.TempDir()
	old := filepath.Join(dir, "ancient.jpg")
	writeFile(t, old, []byte("old"))
	require.NoError(t, os.Chtimes(old, fixedNow.Add(-48*time.Hour), fixedNow.Add(-48*time.Hour)))

	_, err := f.plugin(h, bl).Run(context.Background(), map[string]any{"download_dir": dir, "max_files": 2}, events.Events{})
	require.NoError(t, err)
	// Three files (ancient, r1, .last_run), limit 2: the oldest goes.
	require.Equal(t, []string{".last_run", "wallhaven-r1.jpg"}, dirNames(t, dir))
}

func TestWallhavenProcessBlacklistActionRunsBeforeAnyNetworkOrIntervalLogic(t *testing.T) {
	h, bl := testStores(t)
	f := newFakeWallhaven(t)
	dir := t.TempDir()
	victim := filepath.Join(dir, "wallhaven-v.jpg")
	writeFile(t, victim, []byte("victim"))

	rec := newRecorder()
	res, err := f.plugin(h, bl).Run(context.Background(), map[string]any{
		"download_dir": dir, "action": "process_blacklist", "targets": []any{victim},
	}, rec.events())
	require.NoError(t, err)
	require.Equal(t, Result{Message: "Blacklist processed"}, res)
	require.NoFileExists(t, victim)
	require.True(t, bl.IsBlacklisted(sha256Hex([]byte("victim"))))
	require.Empty(t, f.requests)
	require.NoFileExists(t, filepath.Join(dir, ".last_run"))
	require.True(t, rec.hasLogContaining("[Wallhaven] Processing blacklist for 1 files..."))
}

func TestWallhavenFilenameUsesTheURLSuffixAndFormatsNumericIDs(t *testing.T) {
	require.Equal(t, "94x38z", wallhavenItem{ID: "94x38z"}.idString())
	require.Equal(t, "123", wallhavenItem{ID: float64(123)}.idString())
	require.Equal(t, "None", wallhavenItem{}.idString())
}

func TestWallhavenRunWithoutStoresFailsInsteadOfDownloadingUnchecked(t *testing.T) {
	f := newFakeWallhaven(t)
	p := newWallhaven(Deps{HTTP: f.srv.Client(), Now: time.Now})
	p.searchURL = f.srv.URL + "/search"
	_, err := p.Run(context.Background(), map[string]any{"download_dir": t.TempDir()}, events.Events{})
	require.ErrorContains(t, err, "history store not configured")
}

func TestWallhavenSearchTimesOutViaContextRatherThanHangingForever(t *testing.T) {
	// A server that never answers must not block a plugin run beyond the
	// request deadline; the parent context is honoured too.
	h, bl := testStores(t)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { <-block }))
	t.Cleanup(func() { close(block); srv.Close() })

	p := newWallhaven(Deps{History: h, Blacklist: bl, HTTP: srv.Client(), Now: time.Now})
	p.searchURL = srv.URL + "/search"
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := p.searchAPI(ctx, url.Values{"sorting": {"relevance"}})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
