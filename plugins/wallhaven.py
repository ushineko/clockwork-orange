#!/usr/bin/env python3
"""
Wallhaven Plugin for Clockwork Orange.
Downloads wallpapers from Wallhaven.cc API v1.
"""

import sys
import time
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, List

import requests

# Add parent directory to path to allow importing base
sys.path.append(str(Path(__file__).parent.parent))
from plugins.base import PluginBase
from plugins.blacklist import BlacklistManager
from plugins.history import HistoryManager


class WallhavenPlugin(PluginBase):
    def get_description(self) -> str:
        return "Download wallpapers from Wallhaven.cc (API v1)"

    def get_config_schema(self) -> Dict[str, Any]:
        return {
            "api_key": {
                "type": "string",
                "description": "API Key (Optional, required for NSFW)",
                "default": "",
            },
            "query": {
                "type": "string_list",
                "description": "Search Query",
                "default": [{"term": "landscape", "enabled": True}],
                "suggestions": [
                    "landscape",
                    "cyberpunk",
                    "pixel art",
                    "4k",
                    "toplist",
                ],
            },
            "sorting": {
                "type": "string",
                "description": "Sort Order",
                "default": "relevance",
                "enum": [
                    "relevance",
                    "random",
                    "date_added",
                    "views",
                    "favorites",
                    "toplist",
                ],
            },
            "top_range": {
                "type": "string",
                "description": "Top Range (for toplist sorting)",
                "default": "1M",
                "enum": ["1d", "3d", "1w", "1M", "3M", "6M", "1y"],
            },
            "category_general": {
                "type": "boolean",
                "description": "General",
                "group": "Categories",
                "default": True,
            },
            "category_anime": {
                "type": "boolean",
                "description": "Anime",
                "group": "Categories",
                "default": True,
            },
            "category_people": {
                "type": "boolean",
                "description": "People",
                "group": "Categories",
                "default": True,
            },
            "purity_sfw": {
                "type": "boolean",
                "description": "SFW",
                "group": "Purity",
                "default": True,
            },
            "purity_sketchy": {
                "type": "boolean",
                "description": "Sketchy",
                "group": "Purity",
                "default": False,
            },
            "purity_nsfw": {
                "type": "boolean",
                "description": "NSFW",
                "group": "Purity",
                "default": False,
            },
            "resolutions": {
                "type": "string",
                "description": "Exact Resolutions (comma-separated)",
                "default": "",
                "suggestions": ["1920x1080", "2560x1440", "3840x2160"],
            },
            "atleast": {
                "type": "string",
                "description": "Minimum Resolution (WxH)",
                "default": "2560x1440",
                "suggestions": ["1920x1080", "2560x1440", "3840x2160"],
            },
            "ratios": {
                "type": "string",
                "description": "Aspect Ratios (comma-separated)",
                "default": "16x9",
                "suggestions": ["16x9", "21x9", "16x10", "portrait"],
            },
            "download_dir": {
                "type": "string",
                "description": "Download Directory",
                "default": str(Path.home() / "Pictures" / "Wallpapers" / "Wallhaven"),
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
                "description": "Max Downloads per run",
                "default": 10,
            },
            "max_files": {
                "type": "integer",
                "description": "Retention Limit (Max Files)",
                "default": 100,
            },
        }

    def run(self, config: Dict[str, Any]) -> Dict[str, Any]:
        # Setup directories
        download_dir = Path(
            config.get(
                "download_dir", Path.home() / "Pictures" / "Wallpapers" / "Wallhaven"
            )
        )
        download_dir.mkdir(parents=True, exist_ok=True)

        limit = int(config.get("limit", 10))
        max_files = int(config.get("max_files", 100))

        # Initialize Managers
        self.blacklist_manager = BlacklistManager()
        self.history_manager = HistoryManager()

        # Handle GUI/Review actions
        if config.get("action") == "process_blacklist":
            return self._handle_blacklist_action(config)

        if config.get("reset", False):
            self._perform_reset(download_dir)

        # Handle force run (bypass interval)
        interval = config.get("interval", "Daily").lower()
        force = config.get("force", False)

        if not force and not self._should_run(download_dir, interval):
            print(f"[Wallhaven] Skipping run (interval: {interval})", file=sys.stderr)
            return {"status": "success", "path": str(download_dir)}

        # Parse queries
        queries = self._parse_queries(config.get("query", "landscape"))
        if not queries:
            queries = ["landscape"]

        self._process_queries(queries, config, download_dir, limit)

        # Update last run timestamp
        self._update_last_run(download_dir)

        # Cleanup
        print(f"::PROGRESS:: 95 :: Cleaning up old files...", file=sys.stderr)
        self._cleanup_old_files(download_dir, max_files)

        print(f"::PROGRESS:: 100 :: Done!", file=sys.stderr)
        return {"status": "success", "path": str(download_dir)}

    def _process_queries(
        self, queries: List[str], config: Dict[str, Any], download_dir: Path, limit: int
    ):
        total_downloaded = 0
        for i, query in enumerate(queries):
            term_progress_base = int((i / len(queries)) * 90)
            print(
                f"::PROGRESS:: {term_progress_base} :: Searching API for '{query}'...",
                file=sys.stderr,
            )

            # Build API Parameters with specific query
            params = self._build_api_params(config, query)

            print(
                f"[Wallhaven] Starting search for '{query}' with params: {params}",
                file=sys.stderr,
            )

            # Search API
            try:
                results = self._search_api(params)
                print(
                    f"[Wallhaven] Found {len(results)} wallpapers for '{query}'",
                    file=sys.stderr,
                )
            except Exception as e:
                print(f"[Wallhaven] API Error for '{query}': {e}", file=sys.stderr)
                continue

            # Download Images
            count = 0
            limit_per_query = limit
            total_results = len(results)

            for j, item in enumerate(results):
                if count >= limit_per_query:
                    break

                # Calculate granular progress
                term_slice = 90 / len(queries)
                current_percent = int(
                    term_progress_base + (j / total_results) * term_slice
                )

                print(
                    f"::PROGRESS:: {current_percent} :: Processing image {j+1}/{total_results}...",
                    file=sys.stderr,
                )

                if self._process_item(item, download_dir):
                    count += 1
                    total_downloaded += count

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

    def _build_api_params(self, config: Dict[str, Any], query: str) -> Dict[str, Any]:
        params = {
            "q": query,
            "apikey": config.get("api_key", ""),
            "sorting": config.get("sorting", "relevance"),
            "order": "desc",
            "page": 1,
        }

        if params["sorting"] == "toplist":
            params["topRange"] = config.get("top_range", "1M")

        # Categories: General/Anime/People (111)
        c_gen = "1" if config.get("category_general", True) else "0"
        c_ani = "1" if config.get("category_anime", True) else "0"
        c_ppl = "1" if config.get("category_people", True) else "0"
        params["categories"] = f"{c_gen}{c_ani}{c_ppl}"

        # Purity: SFW/Sketchy/NSFW (100)
        p_sfw = "1" if config.get("purity_sfw", True) else "0"
        p_sky = "1" if config.get("purity_sketchy", False) else "0"
        p_nsfw = "1" if config.get("purity_nsfw", False) else "0"
        params["purity"] = f"{p_sfw}{p_sky}{p_nsfw}"

        # Resolutions
        resolutions = config.get("resolutions", "").strip()
        if resolutions:
            params["resolutions"] = resolutions

        atleast = config.get("atleast", "").strip()
        if atleast:
            params["atleast"] = atleast

        ratios = config.get("ratios", "").strip()
        if ratios:
            params["ratios"] = ratios

        return params

    def _search_api(self, params: Dict[str, Any]) -> List[Dict[str, Any]]:
        url = "https://wallhaven.cc/api/v1/search"
        # Only include seed if random to ensure randomness or consistency?
        # Actually random needs seed to paging, but we just fetch page 1 for now.
        if params["sorting"] == "random":
            import random

            params["seed"] = "".join(
                [str(random.choice("0123456789abcdef")) for _ in range(6)]
            )

        response = requests.get(url, params=params, timeout=10)
        if response.status_code != 200:
            raise Exception(f"HTTP {response.status_code}: {response.text}")

        data = response.json()
        return data.get("data", [])

    def _process_item(self, item: Dict[str, Any], download_dir: Path) -> bool:
        url = item.get("path")
        img_id = item.get("id")

        if not url:
            return False

        # Filename: wallhaven-{id}.ext
        ext = Path(url).suffix
        filename = f"wallhaven-{img_id}{ext}"
        filepath = download_dir / filename

        # Check History (URL)
        if self.history_manager.seen_url(url):
            return False

        if filepath.exists():
            return False

        # Download
        try:
            print(f"[Wallhaven] Downloading {url}...", file=sys.stderr)
            res = requests.get(url, timeout=20)
            if res.status_code == 200:
                filepath.write_bytes(res.content)

                # Check Blacklist
                img_hash = self.blacklist_manager.get_image_hash(filepath)
                if self.blacklist_manager.is_blacklisted(img_hash):
                    print(
                        f"[Wallhaven] Blacklisted image detected. Removing.",
                        file=sys.stderr,
                    )
                    filepath.unlink()
                    return False

                # Check History (Content)
                if self.history_manager.seen_image(filepath):
                    print(
                        f"[Wallhaven] Duplicate content detected. Removing.",
                        file=sys.stderr,
                    )
                    filepath.unlink()
                    self.history_manager.add_entry(url, filepath, source="wallhaven")
                    return False

                # Success
                self.history_manager.add_entry(url, filepath, source="wallhaven")
                print(f"[Wallhaven] Saved {filepath.name}", file=sys.stderr)
                print(f"::IMAGE_SAVED:: {filepath}", file=sys.stderr)
                return True

        except Exception as e:
            print(f"[Wallhaven] Error downloading {url}: {e}", file=sys.stderr)
            return False

        return False

    def _handle_blacklist_action(self, config):
        targets = config.get("targets", [])
        print(
            f"[Wallhaven] Processing blacklist for {len(targets)} files...",
            file=sys.stderr,
        )
        self.blacklist_manager.process_files(targets, plugin_name="wallhaven")
        return {"status": "success", "message": "Blacklist processed"}

    def _perform_reset(self, download_dir):
        print(f"[Wallhaven] Resetting directory {download_dir}...", file=sys.stderr)
        try:
            import shutil

            for item in download_dir.iterdir():
                if item.is_file():
                    item.unlink()
                elif item.is_dir():
                    shutil.rmtree(item)
        except Exception as e:
            print(f"[Wallhaven] Reset failed: {e}", file=sys.stderr)

    def _cleanup_old_files(self, download_dir: Path, max_files: int):
        try:
            files = sorted(
                [f for f in download_dir.iterdir() if f.is_file()],
                key=lambda f: f.stat().st_mtime,
            )
            if len(files) > max_files:
                for f in files[: len(files) - max_files]:
                    f.unlink()
        except Exception:
            pass

        except Exception:
            pass

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


if __name__ == "__main__":
    plugin = WallhavenPlugin()
    plugin.main()
