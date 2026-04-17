#!/usr/bin/env python3
"""
DuckDuckGo Images Downloader Plugin for Clockwork Orange.
Fetches, downloads, and processes images from DuckDuckGo's image search.
"""

import hashlib
import re
import sys
import time
from datetime import datetime, timedelta
from pathlib import Path

import requests
from PIL import Image

try:
    from ddgs import DDGS
except ImportError:
    # Linux distro packages (Arch/Debian) don't ship ddgs; fall back to the
    # direct scrape path, which works from system Python where TLS
    # fingerprints aren't an issue. Frozen Windows/macOS builds bundle ddgs
    # via requirements.txt to survive DDG's anti-bot.
    DDGS = None

sys.path.append(str(Path(__file__).parent.parent))
from plugins.base import PluginBase
from plugins.blacklist import BlacklistManager
from plugins.history import HistoryManager

_USER_AGENT = (
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36"
)


class DuckDuckGoImagesPlugin(PluginBase):
    def get_description(self) -> str:
        return "Download wallpapers from DuckDuckGo Images"

    def get_config_schema(self) -> dict:
        return {
            "query": {
                "type": "string_list",
                "default": [{"term": "4k nature wallpapers", "enabled": True}],
                "description": "Search Terms",
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
            },
            "download_dir": {
                "type": "string",
                "default": str(
                    Path.home() / "Pictures" / "Wallpapers" / "DuckDuckGo"
                ),
                "description": "Download Path",
                "widget": "directory_path",
            },
            "interval": {
                "type": "string",
                "default": "Daily",
                "description": "Check Interval",
                "enum": ["Hourly", "Daily", "Weekly"],
            },
            "limit": {
                "type": "integer",
                "default": 10,
                "description": "Max Downloads (HQ)",
            },
            "max_files": {
                "type": "integer",
                "default": 50,
                "description": "Retention Limit",
            },
        }

    def run(self, config: dict) -> dict:
        queries = self._parse_queries(config.get("query", "4k nature wallpapers"))

        if not queries:
            queries = ["4k nature wallpapers"]

        download_dir = Path(
            config.get(
                "download_dir",
                Path.home() / "Pictures" / "Wallpapers" / "DuckDuckGo",
            )
        )
        interval = config.get("interval", "Daily").lower()
        limit = int(config.get("limit", 10))
        max_files = int(config.get("max_files", 50))

        download_dir.mkdir(parents=True, exist_ok=True)

        self.blacklist_manager = BlacklistManager()
        self.history_manager = HistoryManager()

        if config.get("action") == "process_blacklist":
            return self._handle_blacklist_action(config)

        force = config.get("force", False)
        if not force and not self._should_run(download_dir, interval):
            print(
                f"[DuckDuckGo] Skipping run (interval: {interval})", file=sys.stderr
            )
            return {"status": "success", "path": str(download_dir)}

        print(f"[DuckDuckGo] Starting download...", file=sys.stderr)

        if config.get("reset", False):
            self._perform_reset(download_dir)

        # One shared HTTP session for image downloads: bounds connection-pool
        # growth and ensures sockets are released when the run finishes.
        # URL discovery uses the ddgs library, which handles TLS fingerprinting
        # and backend fallback (DuckDuckGo -> Bing) on its own.
        with requests.Session() as session:
            session.headers.update({
                "User-Agent": _USER_AGENT,
                "Accept-Language": "en-US,en;q=0.9",
            })
            self._session = session
            try:
                total_images_downloaded = self._process_batch(
                    queries, download_dir, limit
                )
            finally:
                self._session = None

        self._update_last_run(download_dir)

        print(f"::PROGRESS:: 95 :: Cleaning up old files...", file=sys.stderr)
        self._cleanup_old_files(download_dir, max_files)

        print(f"::PROGRESS:: 100 :: Done!", file=sys.stderr)

        if total_images_downloaded > 0 or list(download_dir.glob("*")):
            return {"status": "success", "path": str(download_dir)}

        return {"status": "error", "message": "No images found or downloaded"}

    def _parse_queries(self, raw_query):
        queries = []
        if isinstance(raw_query, str):
            queries = [q.strip() for q in raw_query.split(",") if q.strip()]
        elif isinstance(raw_query, list):
            for item in raw_query:
                if isinstance(item, dict):
                    if item.get("enabled", True) and item.get("term"):
                        queries.append(item.get("term"))
                elif isinstance(item, str):
                    queries.append(item)
        return queries

    def _handle_blacklist_action(self, config):
        targets = config.get("targets", [])
        print(
            f"[DuckDuckGo] Processing blacklist for {len(targets)} files...",
            file=sys.stderr,
        )
        self.blacklist_manager.process_files(targets, plugin_name="duckduckgo_images")
        return {"status": "success", "message": "Blacklist processed"}

    def _perform_reset(self, download_dir):
        print(f"::PROGRESS:: 0 :: Resetting directory...", file=sys.stderr)
        print(
            f"[DuckDuckGo] Reset requested. Clearing {download_dir}...",
            file=sys.stderr,
        )
        try:
            for item in download_dir.iterdir():
                if item.is_file():
                    item.unlink()
                elif item.is_dir():
                    import shutil

                    shutil.rmtree(item)
            print(f"[DuckDuckGo] Directory cleared.", file=sys.stderr)
        except Exception as e:
            print(f"[DuckDuckGo] Failed to clear directory: {e}", file=sys.stderr)

    def _process_batch(self, queries, download_dir, limit):
        total_images_downloaded = 0
        for i, query in enumerate(queries):
            term_progress_base = int((i / len(queries)) * 90)
            print(
                f"::PROGRESS:: {term_progress_base} :: Scraping '{query}'...",
                file=sys.stderr,
            )
            print(f"[DuckDuckGo] Processing query: '{query}'", file=sys.stderr)

            count = self._download_images_for_term(
                query, download_dir, limit, term_progress_base, len(queries)
            )
            total_images_downloaded += count
        return total_images_downloaded

    def _cleanup_old_files(self, download_dir: Path, max_files: int):
        try:
            files = sorted(
                [f for f in download_dir.glob("*.jpg")], key=lambda f: f.stat().st_mtime
            )

            if len(files) > max_files:
                to_remove = len(files) - max_files
                print(
                    f"[DuckDuckGo] Cleaning up {to_remove} old images...",
                    file=sys.stderr,
                )
                for f in files[:to_remove]:
                    f.unlink()
                    print(f"[DuckDuckGo] Removed {f.name}", file=sys.stderr)

        except Exception as e:
            print(f"[DuckDuckGo] Cleanup failed: {e}", file=sys.stderr)

    def _should_run(self, download_dir: Path, interval: str) -> bool:
        if interval == "always":
            return True

        timestamp_file = download_dir / ".last_run"
        if not timestamp_file.exists():
            return True

        try:
            last_run = float(timestamp_file.read_text().strip())
            last_run_dt = datetime.fromtimestamp(last_run)
            now = datetime.now()

            if interval == "hourly":
                return now - last_run_dt > timedelta(hours=1)
            elif interval == "daily":
                return now - last_run_dt > timedelta(days=1)
            elif interval == "weekly":
                return now - last_run_dt > timedelta(weeks=1)
        except Exception:
            return True

        return False

    def _update_last_run(self, download_dir: Path):
        timestamp_file = download_dir / ".last_run"
        timestamp_file.write_text(str(time.time()))

    def _download_images_for_term(
        self,
        query: str,
        download_dir: Path,
        limit: int,
        progress_base: int,
        total_terms: int,
    ) -> int:

        image_urls = self._scrape_image_urls(query)

        print(
            f"[DuckDuckGo] Found {len(image_urls)} potential images for '{query}'",
            file=sys.stderr,
        )

        count = 0
        for j, url in enumerate(image_urls):
            if count >= limit:
                break

            term_slice = 90 / total_terms
            current_percent = int(progress_base + (j / max(len(image_urls), 1)) * term_slice)

            print(
                f"::PROGRESS:: {current_percent} :: Checking candidate {j+1} for image {count+1}/{limit}...",
                file=sys.stderr,
            )

            if self._process_image(url, download_dir):
                count += 1
            else:
                print(
                    f"::PROGRESS:: {current_percent} :: Skipped low-quality/duplicate image...",
                    file=sys.stderr,
                )

        return count

    def _scrape_image_urls(self, query: str) -> list:
        if DDGS is not None:
            return self._scrape_via_ddgs(query)
        return self._scrape_via_direct(query)

    def _scrape_via_ddgs(self, query: str) -> list:
        try:
            with DDGS() as ddgs:
                results = ddgs.images(
                    query,
                    size="Large",
                    max_results=200,
                )
            return self._filter_results(results)
        except Exception as e:
            print(f"[DuckDuckGo] Scraping failed for '{query}': {e}", file=sys.stderr)
            return []

    def _scrape_via_direct(self, query: str) -> list:
        try:
            with self._session.get(
                "https://duckduckgo.com/",
                params={"q": query, "iax": "images", "ia": "images"},
                timeout=15,
            ) as landing:
                vqd_match = re.search(r'vqd=["\'](\d-[\d-]+)["\']', landing.text)
                if not vqd_match:
                    vqd_match = re.search(r'"vqd":"(\d-[\d-]+)"', landing.text)
            if not vqd_match:
                print(
                    f"[DuckDuckGo] Failed to extract vqd token for '{query}'",
                    file=sys.stderr,
                )
                return []
            vqd = vqd_match.group(1)

            with self._session.get(
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
            ) as resp:
                try:
                    data = resp.json()
                except ValueError as e:
                    print(
                        f"[DuckDuckGo] Non-JSON response for '{query}': {e}",
                        file=sys.stderr,
                    )
                    return []
            return self._filter_results(data.get("results", []))

        except Exception as e:
            print(f"[DuckDuckGo] Scraping failed for '{query}': {e}", file=sys.stderr)
            return []

    def _filter_results(self, results) -> list:
        urls = []
        seen = set()
        for r in results:
            url = r.get("image")
            if not url or url in seen:
                continue
            try:
                w = int(r.get("width") or 0)
                h = int(r.get("height") or 0)
            except (TypeError, ValueError):
                w, h = 0, 0
            if w and h and (w < 1920 or h < 1080):
                continue
            urls.append(url)
            seen.add(url)
        return urls

    def _process_image(self, url: str, download_dir: Path) -> bool:
        try:
            if self.history_manager.seen_url(url):
                return False

            filename = hashlib.md5(url.encode()).hexdigest() + ".jpg"
            filepath = download_dir / filename

            if filepath.exists():
                return False

            print(f"[DuckDuckGo] Downloading {url}...", file=sys.stderr)

            with self._session.get(url, timeout=10) as img_response:
                if img_response.status_code != 200:
                    return False
                content = img_response.content

            from io import BytesIO

            img = Image.open(BytesIO(content))

            if img.mode in ("RGBA", "P"):
                img = img.convert("RGB")

            min_w, min_h = 1920, 1080
            if img.width < min_w or img.height < min_h:
                print(
                    f"[DuckDuckGo] Rejected low-res image: {img.width}x{img.height} (needs {min_w}x{min_h})",
                    file=sys.stderr,
                )
                img.close()
                return False

            print(
                f"[DuckDuckGo] Processing image: {img.width}x{img.height}",
                file=sys.stderr,
            )

            cropped = self._resize_and_crop(img, 3840, 2160)
            img.close()
            img = cropped

            img.save(filepath, "JPEG", quality=90)
            img.close()

            img_hash = self.blacklist_manager.get_image_hash(filepath)
            if self.blacklist_manager.is_blacklisted(img_hash):
                print(
                    f"[DuckDuckGo] Image is blacklisted. Removing {filepath.name}",
                    file=sys.stderr,
                )
                filepath.unlink()
                return False

            if self.history_manager.seen_image(filepath):
                print(
                    f"[DuckDuckGo] Image content already in history. Skipping.",
                    file=sys.stderr,
                )
                filepath.unlink()
                self.history_manager.add_entry(url, filepath, source="duckduckgo_images")
                return False

            self.history_manager.add_entry(url, filepath, source="duckduckgo_images")

            print(
                f"[DuckDuckGo] Saved processed image to {filepath}", file=sys.stderr
            )
            print(f"::IMAGE_SAVED:: {filepath}", file=sys.stderr)
            return True

        except Exception as e:
            print(f"[DuckDuckGo] Failed to process image: {e}", file=sys.stderr)
            return False

    def _resize_and_crop(self, img, target_w, target_h):
        target_ratio = target_w / target_h
        img_ratio = img.width / img.height

        if img_ratio > target_ratio:
            new_h = target_h
            new_w = int(new_h * img_ratio)
        else:
            new_w = target_w
            new_h = int(new_w / img_ratio)

        img = img.resize((new_w, new_h), Image.Resampling.LANCZOS)

        left = (new_w - target_w) / 2
        top = (new_h - target_h) / 2
        right = (new_w + target_w) / 2
        bottom = (new_h + target_h) / 2

        img = img.crop((left, top, right, bottom))
        return img


if __name__ == "__main__":
    plugin = DuckDuckGoImagesPlugin()
    plugin.main()
