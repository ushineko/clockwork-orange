#!/usr/bin/env python3
"""
Local plugin for clockwork-orange.
Handles both local files and directories as wallpaper sources.
"""
import sys
from pathlib import Path
from typing import Any, Dict

# Ensure we can import the base class
sys.path.append(str(Path(__file__).parent.parent))
from plugins.base import PluginBase
from plugins.blacklist import BlacklistManager


class LocalPlugin(PluginBase):
    def get_description(self) -> str:
        return "Returns a configured local file or directory path."

    def get_config_schema(self) -> Dict[str, Any]:
        return {
            "path": {
                "type": "string",
                "description": "Path to file or directory",
                "required": True,
                "widget": "directory_path",
            },
            "recursive": {
                "type": "boolean",
                "description": "Search recursively (if path is directory)",
                "default": False,
            },
        }

    def run(self, config: Dict[str, Any]) -> Dict[str, Any]:
        path_str = config.get("path")

        # Initialize BlacklistManager (global)
        self.blacklist_manager = BlacklistManager()

        if not path_str:
            return {"status": "error", "message": "Missing 'path' in configuration"}

        path = Path(path_str).expanduser().resolve()

        # Handle blacklist action
        if config.get("action") == "process_blacklist":
            targets = config.get("targets", [])
            print(
                f"[Local] Processing blacklist for {len(targets)} files...",
                file=sys.stderr,
            )
            self.blacklist_manager.process_files(targets, plugin_name="local")
            return {"status": "success", "message": "Blacklist processed"}

        if not path.exists():
            return {"status": "error", "message": f"Path not found: {path}"}

        # We just return the path, main program handles file vs dir logic now
        return {"status": "success", "path": str(path)}


if __name__ == "__main__":
    plugin = LocalPlugin()
    plugin.main()
