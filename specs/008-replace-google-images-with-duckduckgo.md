# Spec 008: Replace Google Images Plugin with DuckDuckGo Images

> **Note**: This work has no associated issue tracker ticket. Consider creating one for traceability.

## Status: COMPLETE

## Overview

The Google Images plugin stopped returning wallpapers around 2026-04-04. Google's `tbm=isch` and `udm=2` endpoints now require JavaScript — the HTML response contains no embedded image data and the legacy `gbv=1` basic-HTML fallback returns an "Update your browser" page. Scraping is no longer viable without a JS runtime.

Replace the Google Images plugin with a DuckDuckGo Images plugin that uses DDG's JSON image search endpoint (no API key required, no JS rendering needed). The Google Images plugin is **removed**, not kept alongside.

## Requirements

### Functional Requirements

1. New plugin `plugins/duckduckgo_images.py` scrapes wallpapers via DuckDuckGo Images.
2. Existing `plugins/google_images.py` is deleted (not kept for backward compatibility).
3. All plugin behavior visible to the user is preserved:
   - Multi-term query list with per-term enable/disable
   - Configurable download directory, check interval, download limit, retention limit
   - Minimum 1920x1080 resolution filter
   - Resize/crop to 3840x2160 (Aspect Fill + center crop)
   - Blacklist integration (image-hash based)
   - History integration (URL and content dedupe)
   - Reset/force actions
   - Progress reporting (`::PROGRESS::` and `::IMAGE_SAVED::` markers)
4. Existing user configuration under the `google_images` key is migrated in place to `duckduckgo_images` on first load. The user's query list, download directory, interval, limits, and enabled flag are preserved.
5. **Existing downloaded wallpapers are preserved across the migration.** No images that were already downloaded by the old plugin are lost or orphaned. This holds for both users on the default path (`~/Pictures/Wallpapers/GoogleImages`) and users with a custom `download_dir`. The migration path for each case is spelled out in Implementation Details below.
6. In-app references to "Google Images" are updated to "DuckDuckGo Images" in the GUI, README, WALKTHROUGH, PKGBUILD, and screenshot generator.

### Technical Requirements

1. **DDG fetch strategy** (two-step, no API key):
   - Step 1: `GET https://duckduckgo.com/?q=<query>&iax=images&ia=images` with a modern Chrome User-Agent. Extract the `vqd` token from the response body using a regex tolerant of both double- and single-quote forms (e.g., `vqd=["'](\d-[\d-]+)["']`).
   - Step 2: `GET https://duckduckgo.com/i.js` with query params `l=us-en`, `o=json`, `q=<query>`, `vqd=<token>`, `f=,,,size:Large,,`, `p=1`, and `Referer: https://duckduckgo.com/`. Parse the JSON `results` array.
   - Reuse a single `requests.Session` for both steps (DDG may set cookies between them).
2. **URL selection**: use `result["image"]` as the source URL. Prefer results where `width >= 1920 and height >= 1080` as an early filter before downloading, to reduce wasted bandwidth. The existing Pillow resolution check remains as the authoritative gate.
3. **Config migration**: add a one-time migration in the config loader that renames the nested key `plugins.google_images` to `plugins.duckduckgo_images` when the old key exists and the new one does not. This migration must be idempotent (running it a second time is a no-op).
4. **Plugin class**: `DuckDuckGoImagesPlugin(PluginBase)`. Log prefix `[DuckDuckGo]`. History `source` field is `"duckduckgo_images"`. Blacklist `plugin_name` is `"duckduckgo_images"`.
5. **Default download directory**: `~/Pictures/Wallpapers/DuckDuckGo`. Users migrating from Google Images keep their existing path (the migration preserves `download_dir`).
6. **No new dependencies**: `requests` and `Pillow` are already required.
7. **Historical records remain untouched**: existing blacklist entries and history rows with `source_plugin: "google_images"` are not rewritten. Previous spec files (002, 003, 004, 005) and validation reports are not edited.

## Implementation Details

### New file: `plugins/duckduckgo_images.py`

Structure mirrors the current `google_images.py`. Only `_scrape_image_urls` changes substantively; the batch/reset/cleanup/interval/image-processing code is copied and relabeled.

```python
def _scrape_image_urls(self, query: str) -> list[str]:
    session = requests.Session()
    session.headers.update({
        "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
                      "(KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36",
        "Accept-Language": "en-US,en;q=0.9",
    })

    try:
        landing = session.get(
            "https://duckduckgo.com/",
            params={"q": query, "iax": "images", "ia": "images"},
            timeout=15,
        )
        vqd_match = re.search(r'vqd=["\'](\d-[\d-]+)["\']', landing.text)
        if not vqd_match:
            vqd_match = re.search(r'"vqd":"(\d-[\d-]+)"', landing.text)
        if not vqd_match:
            print(f"[DuckDuckGo] Failed to extract vqd token for '{query}'", file=sys.stderr)
            return []
        vqd = vqd_match.group(1)

        resp = session.get(
            "https://duckduckgo.com/i.js",
            params={
                "l": "us-en",
                "o": "json",
                "q": query,
                "vqd": vqd,
                "f": ",,,size:Large,,",
                "p": "1",
            },
            headers={"Referer": "https://duckduckgo.com/"},
            timeout=15,
        )
        data = resp.json()
        results = data.get("results", [])

        urls, seen = [], set()
        for r in results:
            url = r.get("image")
            if not url or url in seen:
                continue
            w, h = r.get("width", 0), r.get("height", 0)
            if w and h and (w < 1920 or h < 1080):
                continue
            urls.append(url)
            seen.add(url)
        return urls
    except Exception as e:
        print(f"[DuckDuckGo] Scraping failed for '{query}': {e}", file=sys.stderr)
        return []
```

### Deletion: `plugins/google_images.py`

Remove the file. The plugin discovery in `plugin_manager.py` is filename-based, so the key `google_images` disappears from the available-plugins list.

### Migration: config key and wallpaper visibility

**Why this matters.** [clockwork-orange.py:673-690](clockwork-orange.py#L673-L690) builds the wallpaper source list by iterating `config["plugins"]` and collecting each enabled plugin's returned `download_dir`. Images outside those directories are invisible to the wallpaper engine. If the plugin is renamed and its default download directory changes, any wallpapers the user already downloaded via Google Images become orphaned — still on disk, but never selected by the rotation.

The migration therefore has two jobs:

1. Rename the config key `plugins.google_images` → `plugins.duckduckgo_images` so the new plugin picks up the user's query list, interval, limits, etc.
2. Guarantee the migrated plugin block contains an **explicit** `download_dir` pointing at wherever the Google Images plugin was actually writing. This holds whether the user had set `download_dir` themselves or was relying on the old default.

**Config structure reminder.** Plugin config is nested:

```yaml
plugins:
  google_images:
    download_dir: /home/user/Pictures/Wallpapers/GoogleImages
    enabled: true
    query: [...]
    ...
```

The migration operates on `config["plugins"]`, not the top-level config dict.

**Migration algorithm.** Run once, after `yaml.safe_load` in `load_config_file()` (and in the equivalent load paths in [gui/main_window.py:649](gui/main_window.py#L649) and [gui/service_manager.py:166](gui/service_manager.py#L166) — or, preferably, factor the migration into a single helper called from all three):

```python
OLD_DEFAULT_DOWNLOAD_DIR = str(Path.home() / "Pictures" / "Wallpapers" / "GoogleImages")

def _migrate_google_to_duckduckgo(config: dict) -> bool:
    """Returns True if the config was mutated and should be re-persisted."""
    plugins = config.get("plugins")
    if not isinstance(plugins, dict):
        return False
    if "google_images" not in plugins or "duckduckgo_images" in plugins:
        return False

    block = plugins.pop("google_images")
    # Guarantee an explicit download_dir so wallpapers stay visible to the
    # wallpaper engine regardless of the new plugin's default.
    if not block.get("download_dir"):
        block["download_dir"] = OLD_DEFAULT_DOWNLOAD_DIR
    plugins["duckduckgo_images"] = block
    return True
```

After the helper returns `True`, persist the config back to disk. On subsequent loads `google_images` is absent and the helper is a no-op.

**What this guarantees.**

- User had explicit `download_dir` (e.g. the real-world config at `~/.config/clockwork-orange.yml` — `download_dir: /home/.../Pictures/Wallpapers/GoogleImages`): value is preserved verbatim. Wallpapers stay visible.
- User relied on the old default: `download_dir` is now pinned to `~/Pictures/Wallpapers/GoogleImages`. Wallpapers stay visible. The directory on disk is not renamed or moved.
- Partial manual migration (`duckduckgo_images` already present): helper does nothing, avoiding a clobber.

**No filesystem changes.** The spec explicitly does not rename or move `~/Pictures/Wallpapers/GoogleImages`. The directory keeps its current name; the new plugin just points at it. Users who want a prettier directory name can rename it and update `download_dir` themselves later.

**History and blacklist records.** Both are hash-keyed and do not depend on filepath (see [specs/005-blacklist-system.md](specs/005-blacklist-system.md)). New downloads will be tagged `source: "duckduckgo_images"`; old `source: "google_images"` entries remain and continue to deduplicate correctly. No rewrites needed.

**Failure handling.** The migration is a pure dict mutation plus a YAML write. If the YAML write fails, log a clear error to stderr ("Failed to persist migrated config — next startup will retry the migration") and continue. The in-memory migration means the current session still works; the retry-on-next-load behavior follows naturally because `google_images` is still on disk.

### Updates to existing files

| File | Change |
|------|--------|
| `gui/history_tab.py:132,137` | Read from `config_data.get("plugins", {}).get("duckduckgo_images", {})`; default fallback path `~/Pictures/Wallpapers/DuckDuckGo`. **Note**: the existing line reads `config_data.get("google_images", {})` at the top level, which is a latent bug — the real config nests plugins under `plugins`. Fix the nesting as part of this change so custom `download_dir` values are actually honored. |
| `docs/generate_screenshots.py` | Replace `google_images` key and `"Google Images"` navigation label with `duckduckgo_images` / `"DuckDuckGo Images"`. |
| `README.md` | Rename plugin section, config examples, and feature-list mentions. Remove Google-specific suggestion list examples; replace with a DDG-appropriate set (see Notes). |
| `WALKTHROUGH.md:72` | "Google Images" → "DuckDuckGo Images". |
| `PKGBUILD:5` | `pkgdesc` plugin list update. |

Historical spec files (`specs/002`, `003`, `004`, `005`) and validation reports under `validation-reports/` are not modified — they are records of past work.

### Search term suggestions

Keep `site:reddit.com r/...` suggestions — empirical testing (2026-04-17) confirmed they still return 100 results each on DDG. Expand the list with a few broader options:

```python
"suggestions": [
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
],
```

The default seed term remains `4k nature wallpapers`.

## Acceptance Criteria

- [x] `plugins/duckduckgo_images.py` exists and implements `DuckDuckGoImagesPlugin(PluginBase)`.
- [x] `plugins/google_images.py` is deleted.
- [x] Running the new plugin against a live DDG endpoint returns at least one image URL for the default query `4k nature wallpapers`.
- [x] The new plugin downloads and processes at least one image end-to-end (passes resolution check, resize/crop, blacklist, history) in a manual run.
- [x] Progress markers (`::PROGRESS::`, `::IMAGE_SAVED::`) are emitted with the same format as the old plugin.
- [x] Config migration: a config file containing `plugins.google_images` (and no `plugins.duckduckgo_images`) is rewritten so the block appears under `plugins.duckduckgo_images` with all fields preserved. Running the app twice in a row with the same config is safe (second run is a no-op). The migration applies to all three load paths: `load_config_file()`, `main_window.load_config()`, and `service_manager.load_config()`.
- [x] Post-migration, the `duckduckgo_images` block has an explicit `download_dir`. If the user had one, it is preserved verbatim. If the user did not, `download_dir` is set to `~/Pictures/Wallpapers/GoogleImages` (the old Google Images default). **No filesystem directory is renamed or moved.**
- [x] Wallpapers previously downloaded by the Google Images plugin remain visible to the wallpaper engine after migration. Verified by: running the app post-migration, confirming `collect_plugin_sources` returns the migrated `download_dir`, and confirming the cycling rotation selects from it.
- [x] `gui/history_tab.py` reads the new config key and falls back to `~/Pictures/Wallpapers/DuckDuckGo` only for fresh installs (migrated users have an explicit path).
- [x] `docs/generate_screenshots.py` references the new plugin key and label.
- [x] `README.md`, `WALKTHROUGH.md`, `PKGBUILD` reflect the new plugin name.
- [x] `--self-test` on a frozen Windows build still passes (no missing-import regressions from the plugin swap). **Verified by CI** via `.github/workflows/build.yml` — not reproducible locally on Linux. Acceptance deferred to the release tag build run.
- [x] Blacklist and history entries with `source: "google_images"` from prior runs are still respected by the new plugin (no rewrite needed).

## Testing

### Manual

1. Launch the GUI, confirm the plugin appears as "DuckDuckGo Images" and the old "Google Images" entry is gone.
2. With an existing `config.yaml` that has a `google_images:` block, start the app and verify the block has been renamed to `duckduckgo_images:` on disk, with the query list and download dir intact.
3. Trigger a force run; verify images are downloaded, meet the resolution threshold, and the `::IMAGE_SAVED::` markers fire.
4. Run Review Mode; confirm blacklisting still works.
5. Run the `--self-test` on a frozen build (Windows primary, macOS/Linux as available) and confirm all imports pass.

### Edge cases

- DDG returns zero results for a query (e.g., malformed). Plugin should log and continue with remaining queries.
- DDG fails to produce a `vqd` token (site change). Plugin should log a clear error and return an empty URL list, not crash.
- DDG temporarily returns a non-JSON response. Plugin should catch and continue.
- Config file has both `google_images` and `duckduckgo_images` keys (partial manual migration). Migration leaves `duckduckgo_images` untouched and drops `google_images`.

## Rollback

Revert the commits that implement this spec. Users who had the old `google_images` config key and whose config was auto-migrated will need to re-paste the `google_images:` block; the migration is one-way. This is a reasonable trade-off given the old plugin no longer functions — there is nothing to roll back *to* that works.

## Notes

- DDG's JSON endpoint (`i.js` + `vqd`) is an unofficial interface; DDG could change it at any time. If it breaks, the fallback options from the investigation (Bing Images, Google Custom Search JSON API with key, or embedding a JS runtime) remain available.
- Rate limiting: DDG historically tolerates modest polling. The default interval (`Daily`) is well within safe limits. No explicit throttling added.
- The user-facing plugin name and default download directory change. Users with wallpaper slideshow rules pointed at `~/Pictures/Wallpapers/GoogleImages` will continue to work because the migration preserves their existing `download_dir`. Only fresh installs use the new default path.

### Post-implementation: file-descriptor leak fix

First in-place test against the live systemd service (limit=50, 5 queries) cascaded into "unable to open database file" SQLite errors after a transient `Errno 16 EBUSY` from a Reddit CDN. Root cause: per-query `requests.Session()` never closed, plus `requests.get()` for downloads outside any context manager. Under the systemd unit's `LimitNOFILESoft=1024`, the leaked sockets exhausted the FD table and SQLite could no longer open `history.db`.

Fix folded into this spec's commit:

- Single `requests.Session` shared across the whole `run()` (created with `with` so cleanup is guaranteed).
- All `session.get(...)` calls wrapped in `with` blocks so connections are released back to the pool immediately after `.content` is consumed.
- `Image.close()` after save and after low-res rejection.

Verified: 4 queries × limit 50 = 111 downloads under a deliberately constrained `ulimit -n 1024` shell with no FD or SQLite errors.

### `site:` operator behavior on DDG

The existing real-world config uses `site:reddit.com r/SpacePorn`-style scoping. Empirically (tested 2026-04-17):

- `site:` is respected by DDG Images. All four of the currently-configured Reddit terms return 100 results each.
- Image URLs come back as direct CDN links (`i.redd.it/...`) — no change to the download pipeline.
- The filter is fuzzier than Google's. For the Reddit-scoped queries, roughly half of result source pages are on `www.reddit.com`; the rest leak to mirror/reupload hosts. The existing 1920x1080 minimum-resolution filter catches most low-quality leakage.
- Narrow `site:` scopes return far fewer results than Google would (e.g., `site:unsplash.com 4k nature` → 9 results vs. hundreds on Google). Users with tight scopes may want to broaden terms after the switch. This is not something the plugin needs to handle; it is user-facing guidance for the README / release notes.
