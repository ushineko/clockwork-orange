#!/usr/bin/env python3
"""
Base plugin class for clockwork-orange.
Plugins should inherit from this class or implement a compatible CLI interface.
"""
import abc
import argparse
import json
import sys
from pathlib import Path
from typing import Dict, Any, Optional

class PluginBase(abc.ABC):
    """Base class for clockwork-orange plugins."""
    
    def __init__(self):
        self.parser = argparse.ArgumentParser(description=self.get_description())
        self.parser.add_argument('--config', type=str, help='JSON configuration string')
        self.parser.add_argument('--get-config-schema', action='store_true', help='Print configuration schema')
        
    @abc.abstractmethod
    def get_description(self) -> str:
        """Return a brief description of the plugin."""
        pass
    
    @abc.abstractmethod
    def get_config_schema(self) -> Dict[str, Any]:
        """
        Return the configuration schema for this plugin.
        Should return a dictionary describing expected configuration fields.
        Example:
        {
            "path": {"type": "string", "description": "Path to image or directory", "required": True},
            "recursive": {"type": "boolean", "description": "Search recursively", "default": False}
        }
        """
        pass
    
    @abc.abstractmethod
    def run(self, config: Dict[str, Any]) -> Dict[str, Any]:
        """
        Execute the plugin logic.
        
        Args:
            config: Configuration dictionary.
            
        Returns:
            A dictionary with the result.
            Success format: {"status": "success", "path": "/path/to/image.jpg"}
            Error format: {"status": "error", "message": "Error description"}
        """
        pass
    
    def main(self):
        """Main entry point for CLI execution."""
        args = self.parser.parse_args()
        
        if args.get_config_schema:
            print(json.dumps(self.get_config_schema(), indent=2))
            return
            
        try:
            config = {}
            if args.config:
                config = json.loads(args.config)
                
            result = self.run(config)
            print(json.dumps(result))
            
        except Exception as e:
            error_result = {"status": "error", "message": str(e)}
            print(json.dumps(error_result))
            sys.exit(1)

if __name__ == "__main__":
    print("This module provides the base class for plugins and cannot be run directly.")
    sys.exit(1)
