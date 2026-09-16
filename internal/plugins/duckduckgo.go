package plugins

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // filename derivation only; the Python plugin used md5(url) and existing files carry those names
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// DuckDuckGo endpoints, headers, timeouts and thresholds (R5.4, D9).
const (
	ddgBaseURL         = "https://duckduckgo.com"
	ddgLandingTimeout  = 15 * time.Second
	ddgResultsTimeout  = 15 * time.Second
	ddgDownloadTimeout = 10 * time.Second
	ddgUserAgent       = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36"
	ddgAcceptLanguage = "en-US,en;q=0.9"
	ddgDefaultQuery   = "4k nature wallpapers"
	ddgLog            = "[DuckDuckGo]"
	ddgMinWidth       = 1920
	ddgMinHeight      = 1080
	// Wallpapers are landscape (2.9.6, validation-reports/2026-07-09-ddg-
	// content-filter.md): rejecting portrait and near-square sources drops
	// portraits of people, square product shots and most ad banners, which a
	// resolution gate alone lets through. The band spans ~4:3 to ~21:9 with
	// a little slack; sources outside it are rejected, not force-cropped.
	ddgMinAspect = 1.2
	ddgMaxAspect = 2.5
	// DuckDuckGo relevance decays sharply past the first pages; deep in a
	// 200-result set a clean query bleeds into tangential images.
	ddgMaxResults = 60
	// ddgResultFilter is i.js's `f`: photo (no clipart/gif/transparent),
	// large, landscape. Honoured server-side, which is the on-topic filter.
	ddgResultFilter = "type:photo,size:Large,layout:Wide"
	ddgTargetWidth  = 3840
	ddgTargetHeight = 2160
	ddgJPEGQuality  = 90
)

// The two vqd token patterns tried in order against the landing page.
var (
	ddgVqdAttr = regexp.MustCompile(`vqd=["'](\d-[\d-]+)["']`)
	ddgVqdJSON = regexp.MustCompile(`"vqd":"(\d-[\d-]+)"`)
)

// errNoImages is the R5.4 failure result when nothing was downloaded and the
// directory is empty.
var errNoImages = errors.New("No images found or downloaded") //nolint:staticcheck // ST1005: the Python plugin's message text (R5.4)

// ddgCandidate is one discovered image: the file URL and the page it was
// found on, sent as the Referer when downloading so hosts with hotlink
// protection return the indexed image rather than an ad or placeholder.
type ddgCandidate struct {
	image   string
	referer string
}

// duckduckgo is the port of plugins/duckduckgo_images.py (R5.4) restricted
// to its direct-scrape path (D9/DV9), at the 2.9.9 behaviour.
type duckduckgo struct {
	deps    Deps
	baseURL string // overridable by tests to point at an httptest.Server
}

func newDuckDuckGo(d Deps) *duckduckgo {
	return &duckduckgo{deps: d, baseURL: ddgBaseURL}
}

func (*duckduckgo) Name() string { return "duckduckgo_images" }

func (*duckduckgo) Description() string {
	return "Download wallpapers from DuckDuckGo Images"
}

// ddgDefaultDir is str(Path.home() / "Pictures" / "Wallpapers" / "DuckDuckGo").
func ddgDefaultDir() string {
	return filepath.Join(config.HomeDir(), "Pictures", "Wallpapers", "DuckDuckGo")
}

// Schema is plugins/duckduckgo_images.py get_config_schema, in declaration order.
func (*duckduckgo) Schema() []Field {
	return []Field{
		{
			Key: "query", Type: TypeStringList,
			Default:     []Term{{Term: ddgDefaultQuery, Enabled: true}},
			Description: "Search Terms",
			Suggestions: []string{
				"4k nature wallpapers",
				"4k space wallpapers",
				"site:reddit.com r/SpacePorn",
				"site:reddit.com r/EarthPorn",
				"site:reddit.com r/SkyPorn",
				"site:reddit.com r/Animals",
				"site:reddit.com r/Wallpapers",
				"4k cityscapes",
				"4k abstract wallpapers",
				"4k landscape wallpapers",
			},
		},
		{
			Key: "download_dir", Type: TypeString, Default: ddgDefaultDir(),
			Description: "Download Path", Widget: WidgetDirectoryPath,
		},
		{
			Key: "interval", Type: TypeString, Default: "Daily", Description: "Check Interval",
			Enum: []string{"Hourly", "Daily", "Weekly"},
		},
		{Key: "limit", Type: TypeInteger, Default: 10, Description: "Max Downloads (HQ)"},
		{Key: "max_files", Type: TypeInteger, Default: 50, Description: "Retention Limit"},
	}
}

// Run is plugins/duckduckgo_images.py run(), in the same order: parse
// queries, create the directory, handle process_blacklist, honour the
// interval unless forced, handle reset, scrape and download per query, stamp
// .last_run, apply *.jpg retention, then succeed when anything was
// downloaded or the directory is non-empty and fail otherwise.
func (p *duckduckgo) Run(ctx context.Context, cfg map[string]any, ev events.Events) (Result, error) {
	queries := ParseQueries(cfg["query"], ddgDefaultQuery)

	downloadDir := config.ExpandPath(getString(cfg, "download_dir", ddgDefaultDir()))
	interval := strings.ToLower(getString(cfg, "interval", "Daily"))
	limit := getInt(cfg, "limit", 10)
	maxFiles := getInt(cfg, "max_files", 50)

	if err := os.MkdirAll(downloadDir, 0o750); err != nil {
		return Result{}, fmt.Errorf("create download dir: %w", err)
	}

	if getString(cfg, "action", "") == "process_blacklist" {
		return processBlacklist(cfg, p.deps, p.Name(), ddgLog, ev)
	}
	if p.deps.History == nil {
		return Result{}, errMissingStore(p.Name(), "history")
	}
	if p.deps.Blacklist == nil {
		return Result{}, errMissingStore(p.Name(), "blacklist")
	}

	if !getBool(cfg, "force", false) && !ShouldRun(downloadDir, interval, p.deps.Now()) {
		ev.Infof("%s Skipping run (interval: %s)", ddgLog, interval)
		return Result{Path: downloadDir}, nil
	}

	ev.Infof("%s Starting download...", ddgLog)

	if getBool(cfg, "reset", false) {
		ev.Progress(0, "Resetting directory...")
		ev.Infof("%s Reset requested. Clearing %s...", ddgLog, downloadDir)
		resetDir(downloadDir, ddgLog, ev)
		ev.Infof("%s Directory cleared.", ddgLog)
	}

	total := p.processBatch(ctx, queries, downloadDir, limit, ev)

	if err := UpdateLastRun(downloadDir, p.deps.Now()); err != nil {
		return Result{}, err
	}

	ev.Progress(95, "Cleaning up old files...")
	cleanupOldFiles(downloadDir, "*.jpg", maxFiles, ddgLog, ev)

	ev.Progress(100, "Done!")

	if total > 0 || dirNonEmpty(downloadDir) {
		return Result{Path: downloadDir}, nil
	}
	return Result{}, errNoImages
}

// processBatch is _process_batch: one progress slice per query, summing the
// per-query download counts.
func (p *duckduckgo) processBatch(ctx context.Context, queries []string, downloadDir string,
	limit int, ev events.Events,
) int {
	total := 0
	n := len(queries)
	for i, query := range queries {
		base := int(float64(i) / float64(n) * 90)
		ev.Progress(base, fmt.Sprintf("Scraping '%s'...", query))
		ev.Infof("%s Processing query: '%s'", ddgLog, query)
		total += p.downloadImagesForTerm(ctx, query, downloadDir, limit, base, n, ev)
	}
	return total
}

// downloadImagesForTerm is _download_images_for_term: walk the candidate
// URLs until `limit` images were saved.
func (p *duckduckgo) downloadImagesForTerm(ctx context.Context, query, downloadDir string,
	limit, progressBase, totalTerms int, ev events.Events,
) int {
	candidates := p.scrapeImageURLs(ctx, query, ev)
	ev.Infof("%s Found %d potential images for '%s'", ddgLog, len(candidates), query)

	count := 0
	for j, c := range candidates {
		if count >= limit {
			break
		}
		termSlice := 90 / float64(totalTerms)
		pct := int(float64(progressBase) + float64(j)/float64(max(len(candidates), 1))*termSlice)
		ev.Progress(pct, fmt.Sprintf("Checking candidate %d for image %d/%d...", j+1, count+1, limit))

		if p.processImage(ctx, c, downloadDir, ev) {
			count++
		} else {
			ev.Progress(pct, "Skipped low-quality/duplicate image...")
		}
	}
	return count
}

// sessionHeaders are sent on every request, as the Python requests.Session
// was configured.
func sessionHeaders(extra map[string]string) map[string]string {
	h := map[string]string{
		"User-Agent":      ddgUserAgent,
		"Accept-Language": ddgAcceptLanguage,
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// scrapeImageURLs is _scrape_image_urls (D9): fetch the landing page for
// the vqd token, then the i.js results. Every failure is logged and yields
// no candidates. The landing request's status is not checked, as in the
// Python; i.js's is, since 2.9.9 (a 403 there is the TLS-block symptom).
func (p *duckduckgo) scrapeImageURLs(ctx context.Context, query string, ev events.Events) []ddgCandidate {
	out, err := p.scrapeDirect(ctx, query, ev)
	if err != nil {
		ev.Errorf("%s Scraping failed for '%s': %v", ddgLog, query, err)
		return nil
	}
	return out
}

func (p *duckduckgo) scrapeDirect(ctx context.Context, query string, ev events.Events) ([]ddgCandidate, error) {
	_, landing, err := httpGet(ctx, p.deps.HTTP, p.baseURL+"/",
		url.Values{"q": {query}, "iax": {"images"}, "ia": {"images"}},
		ddgLandingTimeout, sessionHeaders(nil))
	if err != nil {
		return nil, err
	}
	vqd := extractVqd(landing)
	if vqd == "" {
		ev.Errorf("%s Failed to extract vqd token for '%s'", ddgLog, query)
		return nil, nil
	}

	status, body, err := httpGet(ctx, p.deps.HTTP, p.baseURL+"/i.js",
		url.Values{
			"l":   {"us-en"},
			"o":   {"json"},
			"q":   {query},
			"vqd": {vqd},
			"f":   {ddgResultFilter},
			"p":   {"1"},
		},
		ddgResultsTimeout, sessionHeaders(map[string]string{"Referer": ddgBaseURL + "/"}))
	if err != nil {
		return nil, err
	}
	if status != 200 {
		ev.Errorf("%s i.js returned HTTP %d for '%s'", ddgLog, status, query)
		return nil, nil
	}
	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		ev.Errorf("%s Non-JSON response for '%s': %v", ddgLog, query, err)
		return nil, nil
	}
	if len(payload.Results) > ddgMaxResults {
		payload.Results = payload.Results[:ddgMaxResults]
	}
	return filterResults(payload.Results), nil
}

// extractVqd tries the attribute pattern then the JSON pattern (R5.4).
func extractVqd(page []byte) string {
	if m := ddgVqdAttr.FindSubmatch(page); m != nil {
		return string(m[1])
	}
	if m := ddgVqdJSON.FindSubmatch(page); m != nil {
		return string(m[1])
	}
	return ""
}

// filterResults is _filter_results (2.9.9): unique image URLs with their
// source page, dropping any whose reported width and height are both present
// and either below 1920×1080 or outside the wallpaper aspect band. Missing
// or unparsable dimensions pass and are checked after download.
func filterResults(results []map[string]any) []ddgCandidate {
	out := []ddgCandidate{}
	seen := map[string]bool{}
	for _, r := range results {
		u, _ := r["image"].(string)
		if u == "" || seen[u] {
			continue
		}
		w, h := dimension(r["width"]), dimension(r["height"])
		if w < 0 || h < 0 {
			// int() raised on one of them: Python zeroed both.
			w, h = 0, 0
		}
		if w != 0 && h != 0 {
			if w < ddgMinWidth || h < ddgMinHeight {
				continue
			}
			if !isWallpaperShaped(w, h) {
				continue
			}
		}
		referer, _ := r["url"].(string)
		out = append(out, ddgCandidate{image: u, referer: referer})
		seen[u] = true
	}
	return out
}

// isWallpaperShaped is _is_wallpaper_shaped: landscape, within the band.
func isWallpaperShaped(w, h int) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	ratio := float64(w) / float64(h)
	return ratio >= ddgMinAspect && ratio <= ddgMaxAspect
}

// dimension is int(value or 0): -1 signals a value int() would have raised on.
func dimension(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return int(t)
	case string:
		if t == "" {
			return 0
		}
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return -1
		}
		return n
	case bool:
		if t {
			return 1
		}
		return 0
	}
	return -1
}

// DDGFilename is md5(url).jpg, the name the Python plugin gave every saved
// image; existing download directories are full of them (R5.4).
func DDGFilename(rawURL string) string {
	sum := md5.Sum([]byte(rawURL)) //nolint:gosec // not a security hash; kept for filename compatibility
	return hex.EncodeToString(sum[:]) + ".jpg"
}

/*
processImage is _process_image (R5.4). It returns true only when a new file
was saved and recorded:

  - URL already in history, or file already present → false;
  - download (10 s) with the source page as Referer; non-200 → false;
  - decode; flatten to RGB; reject below 1920×1080; reject a non-landscape
    shape (the authoritative check: it catches placeholders and lied-about
    dimensions before they are force-cropped into a wallpaper);
  - cover-resize-and-crop to 3840×2160, save as JPEG q90;
  - SHA-256 blacklisted → delete, false;
  - content already in history → delete, add the URL to history, false;
  - else add to history, ImageSaved, true.

Any error is logged as "Failed to process image" and yields false.
*/
func (p *duckduckgo) processImage(ctx context.Context, c ddgCandidate, downloadDir string, ev events.Events) bool {
	saved, err := p.processImageErr(ctx, c, downloadDir, ev)
	if err != nil {
		ev.Errorf("%s Failed to process image: %v", ddgLog, err)
		return false
	}
	return saved
}

func (p *duckduckgo) processImageErr(ctx context.Context, c ddgCandidate, downloadDir string, ev events.Events) (bool, error) {
	rawURL := c.image
	seen, err := p.deps.History.SeenURL(rawURL)
	if err != nil {
		return false, err
	}
	if seen {
		return false, nil
	}

	filePath := filepath.Join(downloadDir, DDGFilename(rawURL))
	if _, err := os.Stat(filePath); err == nil {
		return false, nil
	}

	ev.Infof("%s Downloading %s...", ddgLog, rawURL)
	var extra map[string]string
	if c.referer != "" {
		extra = map[string]string{"Referer": c.referer}
	}
	status, body, err := httpGet(ctx, p.deps.HTTP, rawURL, nil, ddgDownloadTimeout, sessionHeaders(extra))
	if err != nil {
		return false, err
	}
	if status != 200 {
		return false, nil
	}

	img, _, err := imaging.Decode(bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	rgb := imaging.ToRGB(img)
	w, h := rgb.Bounds().Dx(), rgb.Bounds().Dy()
	if w < ddgMinWidth || h < ddgMinHeight {
		ev.Infof("%s Rejected low-res image: %dx%d (needs %dx%d)", ddgLog, w, h, ddgMinWidth, ddgMinHeight)
		return false, nil
	}
	if !isWallpaperShaped(w, h) {
		ev.Infof("%s Rejected non-landscape image: %dx%d (aspect must be %g-%g)", ddgLog, w, h, ddgMinAspect, ddgMaxAspect)
		return false, nil
	}
	ev.Infof("%s Processing image: %dx%d", ddgLog, w, h)

	cropped := imaging.CoverResizeCrop(rgb, ddgTargetWidth, ddgTargetHeight)
	if err := imaging.SaveJPEG(filePath, cropped, ddgJPEGQuality); err != nil {
		return false, err
	}

	hash, err := imaging.SHA256File(filePath)
	if err != nil {
		return false, err
	}
	if p.deps.Blacklist.IsBlacklisted(hash) {
		ev.Infof("%s Image is blacklisted. Removing %s", ddgLog, filepath.Base(filePath))
		return false, removeFile(filePath)
	}

	dup, err := p.deps.History.SeenImage(filePath)
	if err != nil {
		return false, err
	}
	if dup {
		ev.Infof("%s Image content already in history. Skipping.", ddgLog)
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
	ev.Infof("%s Saved processed image to %s", ddgLog, filePath)
	ev.ImageSaved(filePath)
	return true, nil
}
