#!/usr/bin/env python3
"""
Shared History Manager for tracking downloaded images and preventing duplicates.
Uses SQLite for persistent storage.
"""
import hashlib
import sqlite3
from datetime import datetime
from pathlib import Path


class HistoryManager:
    def __init__(self, db_path=None):
        if db_path is None:
            config_dir = Path.home() / ".config" / "clockwork-orange"
            config_dir.mkdir(parents=True, exist_ok=True)
            self.db_path = config_dir / "history.db"
        else:
            self.db_path = Path(db_path)

        self.init_db()

    def init_db(self):
        """Initialize the database schema."""
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.cursor()
            cursor.execute(
                """
                CREATE TABLE IF NOT EXISTS downloads (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    url_hash TEXT,
                    image_hash TEXT,
                    source TEXT,
                    timestamp REAL,
                    UNIQUE(url_hash)
                )
            """
            )
            # Index for fast lookups
            cursor.execute(
                "CREATE INDEX IF NOT EXISTS idx_image_hash ON downloads(image_hash)"
            )
            conn.commit()

    def seen_url(self, url):
        """Check if a URL has already been processed."""
        url_hash = hashlib.md5(url.encode()).hexdigest()
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.cursor()
            cursor.execute("SELECT 1 FROM downloads WHERE url_hash = ?", (url_hash,))
            return cursor.fetchone() is not None

    def seen_image(self, image_path):
        """Check if an image content hash has already been processed."""
        if not Path(image_path).exists():
            return False

        image_hash = self.get_file_hash(image_path)
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.cursor()
            cursor.execute(
                "SELECT 1 FROM downloads WHERE image_hash = ?", (image_hash,)
            )
            return cursor.fetchone() is not None

    def add_entry(self, url, image_path, source="unknown"):
        """Record a successful download."""
        url_hash = hashlib.md5(url.encode()).hexdigest()
        image_hash = self.get_file_hash(image_path)
        timestamp = datetime.now().timestamp()

        try:
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute(
                    """
                    INSERT INTO downloads (url_hash, image_hash, source, timestamp)
                    VALUES (?, ?, ?, ?)
                """,
                    (url_hash, image_hash, source, timestamp),
                )
                conn.commit()
                return True
        except sqlite3.IntegrityError:
            # Already exists (url_hash constraint)
            return False

    def get_file_hash(self, filepath):
        """Calculate MD5 hash of a file."""
        hasher = hashlib.md5()
        with open(filepath, "rb") as f:
            buf = f.read()
            hasher.update(buf)
        return hasher.hexdigest()

    def get_stats(self):
        """Return statistics about the history database."""
        stats = {"total_records": 0, "unique_images": 0, "db_size_bytes": 0}

        try:
            if self.db_path.exists():
                stats["db_size_bytes"] = self.db_path.stat().st_size

            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute("SELECT COUNT(*) FROM downloads")
                result = cursor.fetchone()
                if result:
                    stats["total_records"] = result[0]

                cursor.execute("SELECT COUNT(DISTINCT image_hash) FROM downloads")
                result = cursor.fetchone()
                if result:
                    stats["unique_images"] = result[0]
        except Exception:
            pass

        return stats

    def clear_history(self):
        """Clear all history records."""
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.cursor()
            cursor.execute("DELETE FROM downloads")
            conn.commit()

            # VACUUM must run outside a transaction
            try:
                original_isolation = conn.isolation_level
                conn.isolation_level = None
                cursor.execute("VACUUM")
            finally:
                conn.isolation_level = original_isolation
