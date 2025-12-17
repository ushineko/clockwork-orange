#!/usr/bin/env python3
"""
Local plugin for clockwork-orange.
Handles both local files and directories as wallpaper sources.
"""
import sys
import json
from pathlib import Path
from typing import Dict, Any

# Ensure we can import the base class
sys.path.append(str(Path(__file__).parent.parent))
from plugins.base import PluginBase

class LocalPlugin(PluginBase):
    def get_description(self) -> str:
        return "Returns a configured local file or directory path."
    
    def get_config_schema(self) -> Dict[str, Any]:
        return {
            "path": {
                "type": "string",
                "description": "Path to file or directory",
                "required": True,
                "widget": "directory_path"  # Using directory_path as it's more flexible or maybe we need a generic path picker
            },
            "recursive": {
                "type": "boolean",
                "description": "Search recursively (if path is directory)",
                "default": False
            }
        }
    
    def run(self, config: Dict[str, Any]) -> Dict[str, Any]:
        path_str = config.get("path")
        if not path_str:
            return {"status": "error", "message": "Missing 'path' in configuration"}
            
        path = Path(path_str).expanduser().resolve()
        
        if not path.exists():
            return {"status": "error", "message": f"Path not found: {path}"}
            
        # We just return the path, main program handles file vs dir logic now
        return {
            "status": "success",
            "path": str(path)
        }

if __name__ == "__main__":
    plugin = LocalPlugin()
    plugin.main()
