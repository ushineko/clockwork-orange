#!/usr/bin/env python3
"""
Shared blacklist manager for plugins using SQLite.
Stores image hashes and thumbnails to prevent re-downloading unwanted images.
"""
import hashlib
import os
import sqlite3
import sys
from datetime import datetime
from io import BytesIO
from pathlib import Path

from PIL import Image


class BlacklistManager:
    """Manages a blacklist of image hashes using SQLite with thumbnail support."""

    def __init__(self, storage_dir: str = None):
        if storage_dir:
            self.storage_dir = Path(storage_dir)
        else:
            self.storage_dir = Path.home() / ".config" / "clockwork-orange"

        self.db_path = self.storage_dir / "blacklist.db"
        self.init_db()

    def init_db(self):
        """Initialize the database schema."""
        self.storage_dir.mkdir(parents=True, exist_ok=True)
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                CREATE TABLE IF NOT EXISTS blacklist (
                    img_hash TEXT PRIMARY KEY,
                    source TEXT,
                    timestamp REAL,
                    thumbnail BLOB
                )
            """
            )
            conn.commit()

    def get_image_hash(self, image_path: str) -> str:
        """Calculate SHA256 hash of an image file."""
        sha256_hash = hashlib.sha256()
        try:
            with open(image_path, "rb") as f:
                for byte_block in iter(lambda: f.read(4096), b""):
                    sha256_hash.update(byte_block)
            return sha256_hash.hexdigest()
        except Exception as e:
            print(f"Error hashing image {image_path}: {e}", file=sys.stderr)
            return None

    def is_blacklisted(self, image_hash: str) -> bool:
        """Check if an image hash is in the blacklist."""
        try:
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute(
                    "SELECT 1 FROM blacklist WHERE img_hash = ?", (image_hash,)
                )
                return cursor.fetchone() is not None
        except Exception:
            return False

    def generate_thumbnail(self, image_path: str) -> bytes:
        """Generate a small thumbnail for the image."""
        try:
            img = Image.open(image_path)
            # Resize
            img.thumbnail((128, 128))
            # Convert to RGB if needed
            if img.mode in ("RGBA", "P"):
                img = img.convert("RGB")

            buffer = BytesIO()
            img.save(buffer, format="JPEG", quality=70)
            return buffer.getvalue()
        except Exception as e:
            print(f"Error generating thumbnail for {image_path}: {e}", file=sys.stderr)
            return None

    def add_to_blacklist(
        self,
        image_hash: str = None,
        plugin_name: str = "unknown",
        file_path: str = None,
    ):
        """
        Add an image to the blacklist.
        If file_path is provided, image_hash (optional) and thumbnail will be generated from it.
        If only image_hash is provided, thumbnail will be null.
        """
        thumbnail_blob = None

        if file_path:
            path = Path(file_path)
            if path.exists():
                if not image_hash:
                    image_hash = self.get_image_hash(str(path))
                thumbnail_blob = self.generate_thumbnail(str(path))

        if not image_hash:
            print("Cannot blacklist: no hash or file provided.", file=sys.stderr)
            return

        timestamp = datetime.now().timestamp()

        try:
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute(
                    """
                    INSERT OR REPLACE INTO blacklist (img_hash, source, timestamp, thumbnail)
                    VALUES (?, ?, ?, ?)
                """,
                    (image_hash, plugin_name, timestamp, thumbnail_blob),
                )
                conn.commit()
        except Exception as e:
            print(f"Error adding to blacklist DB: {e}", file=sys.stderr)

    def remove_from_blacklist(self, image_hash: str):
        """Remove a hash from the blacklist."""
        try:
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute(
                    "DELETE FROM blacklist WHERE img_hash = ?", (image_hash,)
                )
                conn.commit()
        except Exception as e:
            print(f"Error removing from blacklist DB: {e}", file=sys.stderr)

    def get_blacklist_items(self):
        """Return list of dicts: {hash, source, timestamp, thumbnail, date_str}"""
        items = []
        try:
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute(
                    "SELECT img_hash, source, timestamp, thumbnail FROM blacklist ORDER BY timestamp DESC"
                )
                rows = cursor.fetchall()

                for row in rows:
                    ts = row[2]
                    date_str = (
                        datetime.fromtimestamp(ts).strftime("%Y-%m-%d %H:%M")
                        if ts
                        else "Unknown"
                    )
                    items.append(
                        {
                            "hash": row[0],
                            "source": row[1],
                            "timestamp": ts,
                            "thumbnail": row[3],
                            "date": date_str,
                        }
                    )
        except Exception as e:
            print(f"Error fetching blacklist: {e}", file=sys.stderr)
        return items

    def process_files(self, file_paths: list, plugin_name: str = "manual"):
        """Process a list of files: hash them, add to blacklist (with thumbnail), and delete them."""
        for file_path in file_paths:
            path = Path(file_path)
            if path.exists():
                # Pass file_path so thumbnail is generated
                self.add_to_blacklist(plugin_name=plugin_name, file_path=str(path))
                try:
                    os.remove(path)
                    print(f"Blacklisted and removed: {path.name}")
                except Exception as e:
                    print(f"Error removing file {path}: {e}", file=sys.stderr)
            else:
                print(f"File not found: {file_path}", file=sys.stderr)
