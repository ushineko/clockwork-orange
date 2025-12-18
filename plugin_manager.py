#!/usr/bin/env python3
"""
Plugin manager for clockwork-orange.
Handles discovery, loading, and execution of plugins.
"""
import os
import sys
import json
import subprocess
import importlib.util
from pathlib import Path
from typing import Dict, Any, List, Optional

class PluginManager:
    def __init__(self, plugins_dir: Optional[Path] = None):
        if plugins_dir is None:
            # Default to 'plugins' directory relative to this file
            self.plugins_dir = Path(__file__).parent / "plugins"
        else:
            self.plugins_dir = plugins_dir
            
        self.plugins = {}
        self.discover_plugins()
        
    def discover_plugins(self):
        """Find available plugins in the plugins directory."""
        if not self.plugins_dir.exists():
            return
            
        # Look for python files in the plugins directory
        for item in self.plugins_dir.iterdir():
            if item.is_file() and item.suffix == '.py' and item.name not in ['base.py', 'blacklist.py'] and not item.name.startswith('_'):
                plugin_name = item.stem
                self.plugins[plugin_name] = item
                
    def get_available_plugins(self) -> List[str]:
        """Return a list of available plugin names."""
        return list(self.plugins.keys())
        
    def get_plugin_path(self, plugin_name: str) -> Optional[Path]:
        """Get the path to the plugin executable."""
        return self.plugins.get(plugin_name)

    def get_plugin_schema(self, plugin_name: str) -> Dict[str, Any]:
        """Get the configuration schema for a plugin."""
        plugin_path = self.get_plugin_path(plugin_name)
        if not plugin_path:
            raise ValueError(f"Plugin not found: {plugin_name}")
            
        try:
            cmd = [sys.executable, str(plugin_path), "--get-config-schema"]
            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            return json.loads(result.stdout)
        except subprocess.CalledProcessError as e:
            print(f"[ERROR] Failed to get schema for plugin {plugin_name}: {e.stderr}")
            return {}
        except json.JSONDecodeError:
            print(f"[ERROR] Failed to parse schema for plugin {plugin_name}")
            return {}

    def get_plugin_description(self, plugin_name: str) -> str:
        """Get the description of a plugin."""
        plugin_path = self.get_plugin_path(plugin_name)
        if not plugin_path:
            return ""
            
        try:
            cmd = [sys.executable, str(plugin_path), "--get-description"]
            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            return result.stdout.strip()
        except subprocess.CalledProcessError as e:
            print(f"[ERROR] Failed to get description for plugin {plugin_name}: {e.stderr}")
            return ""
        except Exception as e:
            print(f"[ERROR] Failed to get description for plugin {plugin_name}: {e}")
            return ""

    def execute_plugin(self, plugin_name: str, config: Dict[str, Any]) -> Dict[str, Any]:
        """
        Execute a plugin with the given configuration.
        
        Args:
            plugin_name: Name of the plugin to execute.
            config: Configuration dictionary to pass to the plugin.
            
        Returns:
            The result dictionary from the plugin.
        """
        plugin_path = self.get_plugin_path(plugin_name)
        if not plugin_path:
            raise ValueError(f"Plugin not found: {plugin_name}")
            
        try:
            config_json = json.dumps(config)
            cmd = [sys.executable, str(plugin_path), "--config", config_json]
            
            # Print command for debugging
            # print(f"[DEBUG] Executing plugin: {' '.join(cmd)}")
            
            # Run with capture_output=True to get both stdout and stderr
            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            
            # Parse output
            try:
                output = json.loads(result.stdout)
                
                # Attach logs (stderr) to the result if it's a dict
                if isinstance(output, dict):
                    output['logs'] = result.stderr
                    
                return output
            except json.JSONDecodeError:
                # If output is not JSON, wrap it in an error
                return {
                    "status": "error", 
                    "message": f"Invalid output from plugin: {result.stdout.strip()}",
                    "raw_output": result.stdout,
                    "logs": result.stderr
                }
                
        except subprocess.CalledProcessError as e:
            return {
                "status": "error", 
                "message": f"Plugin execution failed: {e.stderr.strip()}",
                "return_code": e.returncode,
                "logs": e.stderr
            }
        except Exception as e:
            return {
                "status": "error", 
                "message": str(e),
                "logs": str(e)
            }

    def execute_plugin_stream(self, plugin_name: str, config: Dict[str, Any]):
        """
        Execute a plugin and yield output line-by-line for real-time logging.
        Yields strings (log lines) or the final result dict.
        """
        plugin_path = self.get_plugin_path(plugin_name)
        if not plugin_path:
            raise ValueError(f"Plugin not found: {plugin_name}")

        config_json = json.dumps(config)
        cmd = [sys.executable, str(plugin_path), "--config", config_json]

        # Use Popen to stream output
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
            universal_newlines=True
        )
        
        # Stream stderr (logs)
        while True:
            line = process.stderr.readline()
            if not line and process.poll() is not None:
                break
            if line:
                yield line.strip()
                
        # Get stdout (result)
        stdout = process.stdout.read()
        
        if process.returncode != 0:
             yield {
                "status": "error", 
                "message": f"Plugin execution failed with code {process.returncode}",
                "logs": "See above logs"
            }
        else:
            try:
                result = json.loads(stdout)
                yield result
            except json.JSONDecodeError:
                 yield {
                    "status": "error", 
                    "message": f"Invalid JSON output: {stdout}",
                    "logs": "See above logs"
                }

if __name__ == "__main__":
    # Simple test
    manager = PluginManager()
    print(f"Available plugins: {manager.get_available_plugins()}")
