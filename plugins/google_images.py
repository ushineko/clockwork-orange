#!/usr/bin/env python3
"""
Google Images Downloader Plugin for Clockwork Orange.
Scrapes, downloads, and processes images from Google Images.
"""

import os
import sys
import hashlib
import json
import time
import requests
import random
import re
from pathlib import Path
from datetime import datetime, timedelta
from bs4 import BeautifulSoup
from PIL import Image, ImageOps

# Add parent directory to path to allow importing base
sys.path.append(str(Path(__file__).parent.parent))
from plugins.base import PluginBase
from plugins.blacklist import BlacklistManager

class GoogleImagesPlugin(PluginBase):
    def get_description(self) -> str:
        return "Download wallpapers from Google Images"

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
                    "4k abstract wallpapers"
                ]
            },
            "download_dir": {
                "type": "string",
                "default": str(Path.home() / "Pictures" / "Wallpapers" / "GoogleImages"),
                "description": "Download Path",
                "widget": "directory_path"
            },
            "interval": {
                "type": "string",
                "default": "Daily",
                "description": "Check Interval",
                "enum": ["Hourly", "Daily", "Weekly"]
            },
            "limit": {
                "type": "integer",
                "default": 10,
                "description": "Max Downloads (HQ)"
            },
            "max_files": {
                "type": "integer",
                "default": 50,
                "description": "Retention Limit"
            }
        }

    def run(self, config: dict) -> dict:
        # Parse queries
        raw_query = config.get("query", "4k nature wallpapers")
        queries = []
        if isinstance(raw_query, str):
            queries = [q.strip() for q in raw_query.split(",") if q.strip()]
        elif isinstance(raw_query, list):
            for item in raw_query:
                if isinstance(item, dict):
                    if item.get('enabled', True) and item.get('term'):
                        queries.append(item.get('term'))
                elif isinstance(item, str):
                    queries.append(item)
                    
        if not queries:
             queries = ["4k nature wallpapers"]
             
        download_dir = Path(config.get("download_dir", Path.home() / "Pictures" / "Wallpapers" / "GoogleImages"))
        interval = config.get("interval", "Daily").lower()
        limit = int(config.get("limit", 10))
        max_files = int(config.get("max_files", 50))
        
        # Ensure download directory exists
        download_dir.mkdir(parents=True, exist_ok=True)
        
        # Initialize BlacklistManager (global)
        self.blacklist_manager = BlacklistManager()

        # Handle blacklist action
        if config.get("action") == "process_blacklist":
            targets = config.get("targets", [])
            print(f"[GoogleImages] Processing blacklist for {len(targets)} files...", file=sys.stderr)
            self.blacklist_manager.process_files(targets, plugin_name="google_images")
            return {"status": "success", "message": "Blacklist processed"}
        
        # Handle force run (bypass interval)
        force = config.get("force", False)
        if not force and not self._should_run(download_dir, interval):
            print(f"[GoogleImages] Skipping run (interval: {interval})", file=sys.stderr)
            return {"status": "success", "path": str(download_dir)}
            
        print(f"[GoogleImages] Starting download...", file=sys.stderr)
        
        # Reset (Clear directory) if requested
        if config.get("reset", False):
            print(f"::PROGRESS:: 0 :: Resetting directory...", file=sys.stderr)
            print(f"[GoogleImages] Reset requested. Clearing {download_dir}...", file=sys.stderr)
            try:
                for item in download_dir.iterdir():
                    if item.is_file():
                        item.unlink()
                    elif item.is_dir():
                        import shutil
                        shutil.rmtree(item)
                print(f"[GoogleImages] Directory cleared.", file=sys.stderr)
            except Exception as e:
                print(f"[GoogleImages] Failed to clear directory: {e}", file=sys.stderr)
        
        total_images_downloaded = 0
        
        for i, query in enumerate(queries):
            term_progress_base = int((i / len(queries)) * 90)
            print(f"::PROGRESS:: {term_progress_base} :: Scraping '{query}'...", file=sys.stderr)
            print(f"[GoogleImages] Processing query: '{query}'", file=sys.stderr)
            
            # Scrape and download
            count = self._download_images_for_term(query, download_dir, limit, term_progress_base, len(queries))
            total_images_downloaded += count
            
        # Update last run timestamp
        self._update_last_run(download_dir)
        
        # Cleanup
        print(f"::PROGRESS:: 95 :: Cleaning up old files...", file=sys.stderr)
        self._cleanup_old_files(download_dir, max_files)
        
        print(f"::PROGRESS:: 100 :: Done!", file=sys.stderr)
        
        if total_images_downloaded > 0:
            return {"status": "success", "path": str(download_dir)}
        else:
            # If no new images but directory has files, still return success
            if list(download_dir.glob("*")):
                return {"status": "success", "path": str(download_dir)}
            return {"status": "error", "message": "No images found or downloaded"}
            
    def _cleanup_old_files(self, download_dir: Path, max_files: int):
        try:
            files = sorted(
                [f for f in download_dir.glob("*.jpg")],
                key=lambda f: f.stat().st_mtime
            )
            
            if len(files) > max_files:
                to_remove = len(files) - max_files
                print(f"[GoogleImages] Cleaning up {to_remove} old images...", file=sys.stderr)
                for f in files[:to_remove]:
                    f.unlink()
                    print(f"[GoogleImages] Removed {f.name}", file=sys.stderr)
                    
        except Exception as e:
             print(f"[GoogleImages] Cleanup failed: {e}", file=sys.stderr)

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
            return True # Error reading file, run anyway
            
        return False

    def _update_last_run(self, download_dir: Path):
        timestamp_file = download_dir / ".last_run"
        timestamp_file.write_text(str(time.time()))
            
    # ... (other methods) ...

    def _download_images_for_term(self, query: str, download_dir: Path, limit: int, progress_base: int, total_terms: int) -> int:
        headers = {
            "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9"
        }
        
        # Prepare search URL (tbm=isch for images, tbs=isz:l for large images)
        search_url = f"https://www.google.com/search?q={query}&tbm=isch&tbs=isz:l"
        
        try:
            response = requests.get(search_url, headers=headers)
            
            regex = r'\["(http[^"]+\.(?:jpg|jpeg|png))",\d+,\d+\]'
            matches = re.finditer(regex, response.text)
            
            image_urls = []
            seen_urls = set()
            
            for match in matches:
                url = match.group(1)
                try:
                    url = bytes(url, "utf-8").decode("unicode_escape").replace(r'\/', '/')
                except Exception:
                    pass
                if "encrypted-tbn0" in url:
                    continue
                if url not in seen_urls:
                    image_urls.append(url)
                    seen_urls.add(url)
            
            print(f"[GoogleImages] Found {len(image_urls)} potential images for '{query}'", file=sys.stderr)
            
            count = 0
            for j, url in enumerate(image_urls):
                if count >= limit:
                    break
                    
                # Calculate granular progress
                # Each term gets (100 / total_terms) percent
                # Each image gets a fraction of that
                term_slice = 90 / total_terms
                current_percent = int(progress_base + (j / len(image_urls)) * term_slice)
                
                # Show candidate progress
                print(f"::PROGRESS:: {current_percent} :: Checking candidate {j+1} for image {count+1}/{limit}...", file=sys.stderr)
                    
                if self._process_image(url, download_dir):
                    count += 1
                else:
                    print(f"::PROGRESS:: {current_percent} :: Skipped low-quality/duplicate image...", file=sys.stderr)
                    
            return count
            
        except Exception as e:
            print(f"[GoogleImages] Scraping failed for '{query}': {e}", file=sys.stderr)
            return 0

    def _process_image(self, url: str, download_dir: Path) -> bool:
        try:
            # Create a filename from URL hash to avoid duplicates
            import hashlib
            filename = hashlib.md5(url.encode()).hexdigest() + ".jpg"
            filepath = download_dir / filename
            
            if filepath.exists():
                return False # Already downloaded
                
            print(f"[GoogleImages] Downloading {url}...", file=sys.stderr)
            
            # Download with timeout
            img_response = requests.get(url, timeout=10)
            if img_response.status_code != 200:
                return False
                
            # Process with Pillow
            from io import BytesIO
            img = Image.open(BytesIO(img_response.content))
            
            # Convert to RGB (remove alpha)
            if img.mode in ('RGBA', 'P'):
                img = img.convert('RGB')
                
            # Quality Check
            min_w, min_h = 1920, 1080
            if img.width < min_w or img.height < min_h:
                print(f"[GoogleImages] Rejected low-res image: {img.width}x{img.height} (needs {min_w}x{min_h})", file=sys.stderr)
                return False
            
            print(f"[GoogleImages] Processing image: {img.width}x{img.height}", file=sys.stderr)
                
            # Target resolution (4K)
            target_w, target_h = 3840, 2160
            
            # Resize logic: Aspect Fill
            # Calculate aspect ratios
            target_ratio = target_w / target_h
            img_ratio = img.width / img.height
            
            if img_ratio > target_ratio:
                # Image is wider than target
                # Resize based on height
                new_h = target_h
                new_w = int(new_h * img_ratio)
            else:
                # Image is taller than target
                # Resize based on width
                new_w = target_w
                new_h = int(new_w / img_ratio)
                
            # Resize
            img = img.resize((new_w, new_h), Image.Resampling.LANCZOS)
            
            # Center Crop
            left = (new_w - target_w) / 2
            top = (new_h - target_h) / 2
            right = (new_w + target_w) / 2
            bottom = (new_h + target_h) / 2
            
            img = img.crop((left, top, right, bottom))
            
            # Save
            img.save(filepath, "JPEG", quality=90)
            
            # Check blacklist
            img_hash = self.blacklist_manager.get_image_hash(filepath)
            if self.blacklist_manager.is_blacklisted(img_hash):
                print(f"[GoogleImages] Image is blacklisted. Removing {filepath.name}", file=sys.stderr)
                filepath.unlink()
                return False
                
            print(f"[GoogleImages] Saved processed image to {filepath}", file=sys.stderr)
            print(f"::IMAGE_SAVED:: {filepath}", file=sys.stderr)
            return True
            
        except Exception as e:
            print(f"[GoogleImages] Failed to process image: {e}", file=sys.stderr)
            return False

if __name__ == "__main__":
    plugin = GoogleImagesPlugin()
    plugin.main()
