package plugins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// Wallhaven endpoint and timeouts (R5.3).
const (
	wallhavenSearchURL       = "https://wallhaven.cc/api/v1/search"
	wallhavenSearchTimeout   = 10 * time.Second
	wallhavenDownloadTimeout = 20 * time.Second
	wallhavenDefaultQuery    = "landscape"
	wallhavenLog             = "[Wallhaven]"
)

// wallhaven is the port of plugins/wallhaven.py (R5.3).
type wallhaven struct {
	deps      Deps
	searchURL string // overridable by tests to point at an httptest.Server
}

func newWallhaven(d Deps) *wallhaven {
	return &wallhaven{deps: d, searchURL: wallhavenSearchURL}
}

func (*wallhaven) Name() string { return "wallhaven" }

func (*wallhaven) Description() string {
	return "Download wallpapers from Wallhaven.cc (API v1)"
}

// wallhavenDefaultDir is str(Path.home() / "Pictures" / "Wallpapers" / "Wallhaven").
func wallhavenDefaultDir() string {
	return filepath.Join(config.HomeDir(), "Pictures", "Wallpapers", "Wallhaven")
}

// Schema is plugins/wallhaven.py get_config_schema, in declaration order.
func (*wallhaven) Schema() []Field {
	resolutionSuggestions := []string{"1920x1080", "2560x1440", "3840x2160"}
	return []Field{
		{Key: "api_key", Type: TypeString, Description: "API Key (Optional, required for NSFW)", Default: ""},
		{
			Key: "query", Type: TypeStringList, Description: "Search Query",
			Default:     []Term{{Term: wallhavenDefaultQuery, Enabled: true}},
			Suggestions: []string{"landscape", "cyberpunk", "pixel art", "4k", "toplist"},
		},
		{
			Key: "sorting", Type: TypeString, Description: "Sort Order", Default: "relevance",
			Enum: []string{"relevance", "random", "date_added", "views", "favorites", "toplist"},
		},
		{
			Key: "top_range", Type: TypeString, Description: "Top Range (for toplist sorting)", Default: "1M",
			Enum: []string{"1d", "3d", "1w", "1M", "3M", "6M", "1y"},
		},
		{Key: "category_general", Type: TypeBoolean, Description: "General", Group: "Categories", Default: true},
		{Key: "category_anime", Type: TypeBoolean, Description: "Anime", Group: "Categories", Default: true},
		{Key: "category_people", Type: TypeBoolean, Description: "People", Group: "Categories", Default: true},
		{Key: "purity_sfw", Type: TypeBoolean, Description: "SFW", Group: "Purity", Default: true},
		{Key: "purity_sketchy", Type: TypeBoolean, Description: "Sketchy", Group: "Purity", Default: false},
		{Key: "purity_nsfw", Type: TypeBoolean, Description: "NSFW", Group: "Purity", Default: false},
		{
			Key: "resolutions", Type: TypeString, Description: "Exact Resolutions (comma-separated)", Default: "",
			Suggestions: resolutionSuggestions,
		},
		{
			Key: "atleast", Type: TypeString, Description: "Minimum Resolution (WxH)", Default: "2560x1440",
			Suggestions: resolutionSuggestions,
		},
		{
			Key: "ratios", Type: TypeString, Description: "Aspect Ratios (comma-separated)", Default: "16x9",
			Suggestions: []string{"16x9", "21x9", "16x10", "portrait"},
		},
		{
			Key: "download_dir", Type: TypeString, Description: "Download Directory",
			Default: wallhavenDefaultDir(), Widget: WidgetDirectoryPath,
		},
		{
			Key: "interval", Type: TypeString, Default: "Daily", Description: "Check Interval",
			Enum: []string{"Hourly", "Daily", "Weekly"},
		},
		{Key: "limit", Type: TypeInteger, Description: "Max Downloads per run", Default: 10},
		{Key: "max_files", Type: TypeInteger, Description: "Retention Limit (Max Files)", Default: 100},
	}
}

// Run is plugins/wallhaven.py run(), in the same order: create the download
// directory, handle process_blacklist, handle reset, honour the interval
// unless forced, then search and download per query, stamp .last_run, apply
// retention and report the directory.
func (p *wallhaven) Run(ctx context.Context, cfg map[string]any, ev events.Events) (Result, error) {
	downloadDir := config.ExpandPath(getString(cfg, "download_dir", wallhavenDefaultDir()))
	if err := os.MkdirAll(downloadDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("create download dir: %w", err)
	}
	limit := getInt(cfg, "limit", 10)
	maxFiles := getInt(cfg, "max_files", 100)

	if getString(cfg, "action", "") == "process_blacklist" {
		return processBlacklist(cfg, p.deps, p.Name(), wallhavenLog, ev)
	}
	if p.deps.History == nil {
		return Result{}, errMissingStore(p.Name(), "history")
	}
	if p.deps.Blacklist == nil {
		return Result{}, errMissingStore(p.Name(), "blacklist")
	}

	if getBool(cfg, "reset", false) {
		ev.Infof("%s Resetting directory %s...", wallhavenLog, downloadDir)
		resetDir(downloadDir, wallhavenLog, ev)
	}

	interval := strings.ToLower(getString(cfg, "interval", "Daily"))
	if !getBool(cfg, "force", false) && !ShouldRun(downloadDir, interval, p.deps.Now()) {
		ev.Infof("%s Skipping run (interval: %s)", wallhavenLog, interval)
		return Result{Path: downloadDir}, nil
	}

	queries := ParseQueries(cfg["query"], wallhavenDefaultQuery)
	p.processQueries(ctx, queries, cfg, downloadDir, limit, ev)

	if err := UpdateLastRun(downloadDir, p.deps.Now()); err != nil {
		return Result{}, err
	}

	ev.Progress(95, "Cleaning up old files...")
	cleanupOldFiles(downloadDir, "*", maxFiles, wallhavenLog, ev)

	ev.Progress(100, "Done!")
	return Result{Path: downloadDir}, nil
}

// processQueries is _process_queries: each query gets an equal slice of the
// 0–90 progress range; an API failure for one query is logged and the next
// query proceeds; `limit` applies per query (known quirk, kept).
func (p *wallhaven) processQueries(ctx context.Context, queries []string, cfg map[string]any,
	downloadDir string, limit int, ev events.Events,
) {
	n := len(queries)
	for i, query := range queries {
		base := int(float64(i) / float64(n) * 90)
		ev.Progress(base, fmt.Sprintf("Searching API for '%s'...", query))

		params := buildAPIParams(cfg, query)
		ev.Infof("%s Starting search for '%s' with params: %s", wallhavenLog, query, redactParams(params))

		results, err := p.searchAPI(ctx, params)
		if err != nil {
			ev.Errorf("%s API Error for '%s': %v", wallhavenLog, query, err)
			continue
		}
		ev.Infof("%s Found %d wallpapers for '%s'", wallhavenLog, len(results), query)

		count := 0
		total := len(results)
		for j, item := range results {
			if count >= limit {
				break
			}
			termSlice := 90 / float64(n)
			pct := int(float64(base) + float64(j)/float64(total)*termSlice)
			ev.Progress(pct, fmt.Sprintf("Processing image %d/%d...", j+1, total))

			if p.processItem(ctx, item, downloadDir, ev) {
				count++
			}
		}
	}
}

// buildAPIParams is _build_api_params (R5.3): the query string for one
// search, without the random seed (searchAPI adds that). The golden table
// tests/golden/plugins/wallhaven_params.json pins its output.
func buildAPIParams(cfg map[string]any, query string) url.Values {
	params := url.Values{}
	params.Set("q", query)
	params.Set("apikey", getString(cfg, "api_key", ""))
	sorting := getString(cfg, "sorting", "relevance")
	params.Set("sorting", sorting)
	params.Set("order", "desc")
	params.Set("page", "1")

	if sorting == "toplist" {
		params.Set("topRange", getString(cfg, "top_range", "1M"))
	}

	// Categories: General/Anime/People (111)
	params.Set("categories", bit(getBool(cfg, "category_general", true))+
		bit(getBool(cfg, "category_anime", true))+
		bit(getBool(cfg, "category_people", true)))

	// Purity: SFW/Sketchy/NSFW (100)
	params.Set("purity", bit(getBool(cfg, "purity_sfw", true))+
		bit(getBool(cfg, "purity_sketchy", false))+
		bit(getBool(cfg, "purity_nsfw", false)))

	for _, key := range []string{"resolutions", "atleast", "ratios"} {
		if v := strings.TrimSpace(getString(cfg, key, "")); v != "" {
			params.Set(key, v)
		}
	}
	return params
}

// bit renders a boolean as the "1"/"0" the Wallhaven bitstrings use.
func bit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// redactParams renders params for a log line with the API key masked. The
// Python plugin printed the key; the port never logs it.
func redactParams(params url.Values) string {
	clone := url.Values{}
	for k, vs := range params {
		if k == "apikey" {
			if len(vs) > 0 && vs[0] != "" {
				clone.Set(k, "***")
			} else {
				clone.Set(k, "")
			}
			continue
		}
		clone[k] = vs
	}
	return clone.Encode()
}

// wallhavenItem is the subset of a Wallhaven search result the plugin uses.
type wallhavenItem struct {
	ID   any    `json:"id"`
	Path string `json:"path"`
}

// idString renders an item id as f"{img_id}" would: strings verbatim,
// numbers without a trailing ".0", a missing id as "None".
func (it wallhavenItem) idString() string {
	switch v := it.ID.(type) {
	case nil:
		return "None"
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// searchAPI is _search_api: GET the search endpoint (10 s), adding a random
// 6-hex seed when sorting is random; a non-200 status is an error carrying
// the body, as the Python exception did.
func (p *wallhaven) searchAPI(ctx context.Context, params url.Values) ([]wallhavenItem, error) {
	if params.Get("sorting") == "random" {
		seed, err := randomSeed()
		if err != nil {
			return nil, err
		}
		params.Set("seed", seed)
	}
	status, body, err := httpGet(ctx, p.deps.HTTP, p.searchURL, params, wallhavenSearchTimeout, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", status, string(body))
	}
	var payload struct {
		Data []wallhavenItem `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	return payload.Data, nil
}

// randomSeed is the 6-character lower-case hex seed the Python plugin drew.
func randomSeed() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random seed: %w", err)
	}
	return hex.EncodeToString(b), nil
}

/*
processItem is _process_item (R5.3). It returns true only when a new file was
saved and recorded:

  - no path → false;
  - URL already in history, or file already present → false;
  - download (20 s); non-200 → false;
  - SHA-256 blacklisted → delete, false;
  - content already in history → delete, add the URL to history, false;
  - else add to history, ImageSaved, true.

The filename is wallhaven-<id><ext> where ext is the suffix of the URL path
as Path(url).suffix computed it.
*/
func (p *wallhaven) processItem(ctx context.Context, item wallhavenItem, downloadDir string, ev events.Events) bool {
	rawURL := item.Path
	if rawURL == "" {
		return false
	}
	filename := "wallhaven-" + item.idString() + path.Ext(rawURL)
	filePath := filepath.Join(downloadDir, filename)

	seen, err := p.deps.History.SeenURL(rawURL)
	if err != nil {
		ev.Errorf("%s Error downloading %s: %v", wallhavenLog, rawURL, err)
		return false
	}
	if seen {
		return false
	}
	if _, err := os.Stat(filePath); err == nil {
		return false
	}

	ev.Infof("%s Downloading %s...", wallhavenLog, rawURL)
	saved, err := p.downloadItem(ctx, rawURL, filePath, ev)
	if err != nil {
		ev.Errorf("%s Error downloading %s: %v", wallhavenLog, rawURL, err)
		return false
	}
	return saved
}

// downloadItem is the try-block of _process_item; any error aborts the item.
func (p *wallhaven) downloadItem(ctx context.Context, rawURL, filePath string, ev events.Events) (bool, error) {
	status, body, err := httpGet(ctx, p.deps.HTTP, rawURL, nil, wallhavenDownloadTimeout, nil)
	if err != nil {
		return false, err
	}
	if status != 200 {
		return false, nil
	}
	if err := os.WriteFile(filePath, body, 0o644); err != nil { //nolint:gosec // wallpapers are meant to be world-readable
		return false, fmt.Errorf("write %s: %w", filePath, err)
	}

	hash, err := imaging.SHA256File(filePath)
	if err != nil {
		return false, err
	}
	if p.deps.Blacklist.IsBlacklisted(hash) {
		ev.Infof("%s Blacklisted image detected. Removing.", wallhavenLog)
		return false, removeFile(filePath)
	}

	dup, err := p.deps.History.SeenImage(filePath)
	if err != nil {
		return false, err
	}
	if dup {
		ev.Infof("%s Duplicate content detected. Removing.", wallhavenLog)
		// Record the URL before deleting the file: AddEntry hashes the file,
		// and the Python order (unlink, then add_entry) made add_entry raise
		// on the missing file so the URL was never recorded. R5.3 asks for
		// the entry, so the port records it first.
		if _, err := p.deps.History.AddEntry(rawURL, filePath, p.Name()); err != nil {
			return false, err
		}
		return false, removeFile(filePath)
	}

	if _, err := p.deps.History.AddEntry(rawURL, filePath, p.Name()); err != nil {
		return false, err
	}
	ev.Infof("%s Saved %s", wallhavenLog, filepath.Base(filePath))
	ev.ImageSaved(filePath)
	return true, nil
}

// removeFile is os.Remove with a wrapped error.
func removeFile(p string) error {
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("remove %s: %w", p, err)
	}
	return nil
}
