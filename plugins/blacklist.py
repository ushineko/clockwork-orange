#!/usr/bin/env python3
"""
Shared blacklist manager for plugins.
"""
import json
import hashlib
import sys
import os
from pathlib import Path
from datetime import datetime

class BlacklistManager:
    """Manages a blacklist of image hashes to prevent re-downloading unwanted images."""
    
    def __init__(self, storage_dir: str = None):
        """
        Initialize BlacklistManager.
        If storage_dir is None, uses ~/.config/clockwork-orange/
        """
        if storage_dir:
            self.storage_dir = Path(storage_dir)
        else:
            self.storage_dir = Path.home() / ".config" / "clockwork-orange"
            
        self.blacklist_file = self.storage_dir / "blacklist.json"
        self.data = {} # Format: {hash: {date: iso_str, plugin: str}}
        self.load_blacklist()
        
    def load_blacklist(self):
        """Load blacklist from JSON file, supporting migration from legacy list format."""
        if self.blacklist_file.exists():
            try:
                with open(self.blacklist_file, 'r') as f:
                    content = json.load(f)
                
                if isinstance(content, list):
                    # Migration from legacy list format
                    print(f"Migrating legacy blacklist format in {self.blacklist_file}", file=sys.stderr)
                    current_time = datetime.now().isoformat()
                    self.data = {h: {"date": current_time, "plugin": "legacy"} for h in content}
                    self.save_blacklist()
                elif isinstance(content, dict):
                    self.data = content
                else:
                    print(f"Unknown blacklist format in {self.blacklist_file}", file=sys.stderr)
                    self.data = {}
            except Exception as e:
                print(f"Error loading blacklist: {e}", file=sys.stderr)
                self.data = {}
        else:
            self.data = {}
            
    def save_blacklist(self):
        """Save blacklist to JSON file."""
        try:
            self.storage_dir.mkdir(parents=True, exist_ok=True)
            with open(self.blacklist_file, 'w') as f:
                json.dump(self.data, f, indent=2)
        except Exception as e:
            print(f"Error saving blacklist: {e}", file=sys.stderr)

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
        return image_hash in self.data

    def add_to_blacklist(self, image_hash: str, plugin_name: str = "unknown"):
        """Add an image hash to the blacklist with metadata."""
        if image_hash:
            self.data[image_hash] = {
                "date": datetime.now().isoformat(),
                "plugin": plugin_name
            }
            self.save_blacklist()
            
    def remove_from_blacklist(self, image_hash: str):
        """Remove a hash from the blacklist."""
        if image_hash in self.data:
            del self.data[image_hash]
            self.save_blacklist()
    
    def process_files(self, file_paths: list, plugin_name: str = "manual"):
        """Process a list of files: hash them, add to blacklist, and delete them."""
        for file_path in file_paths:
            path = Path(file_path)
            if path.exists():
                img_hash = self.get_image_hash(str(path))
                if img_hash:
                    self.add_to_blacklist(img_hash, plugin_name)
                    try:
                        os.remove(path)
                        print(f"Blacklisted and removed: {path.name}")
                    except Exception as e:
                        print(f"Error removing file {path}: {e}", file=sys.stderr)
            else:
                 print(f"File not found: {file_path}", file=sys.stderr)
