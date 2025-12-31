#!/usr/bin/env python3
"""
Plugin manager for clockwork-orange.
Handles discovery, loading, and execution of plugins.
"""
import contextlib
import importlib.util
import io
import json
import sys
import traceback
from pathlib import Path
from typing import Any, Dict, List, Optional

# Constants for frozen applications
if getattr(sys, "frozen", False):
    # PyInstaller creates a temporary bundle directory at sys._MEIPASS
    BASE_DIR = Path(sys._MEIPASS)
else:
    BASE_DIR = Path(__file__).parent


class PluginManager:
    def __init__(self, plugins_dir: Optional[Path] = None):
        if plugins_dir is None:
            self.plugins_dir = BASE_DIR / "plugins"
        else:
            self.plugins_dir = plugins_dir

        self.plugins = {}
        self.discover_plugins()

    def discover_plugins(self):
        """Find available plugins in the plugins directory."""
        if not self.plugins_dir.exists():
            print(f"[ERROR] Plugins directory not found: {self.plugins_dir}")
            return

        # Look for python files in the plugins directory
        # In frozen state, we might rely on known imports, but iterating dir works
        # if PyInstaller collected them as data or separate files.
        # Ideally, we include them as a package.
        
        # print(f"[DEBUG] Discovering plugins in {self.plugins_dir}")


        for item in self.plugins_dir.iterdir():
            if (
                item.is_file()
                and item.suffix == ".py"
                and item.name not in ["__init__.py", "base.py", "blacklist.py"]
                and not item.name.startswith("_")
            ):
                plugin_name = item.stem
                self.plugins[plugin_name] = item
                # print(f"[DEBUG] Found plugin: {plugin_name}")

    def get_available_plugins(self) -> List[str]:
        """Return a list of available plugin names."""
        return list(self.plugins.keys())

    def get_plugin_path(self, plugin_name: str) -> Optional[Path]:
        """Get the path to the plugin source file."""
        return self.plugins.get(plugin_name)

    def _load_plugin_module(self, plugin_name: str):
        """Dynamically load the plugin module."""
        plugin_path = self.get_plugin_path(plugin_name)
        if not plugin_path:
            raise ValueError(f"Plugin not found: {plugin_name}")

        try:
            # Create module spec
            spec = importlib.util.spec_from_file_location(
                f"plugins.{plugin_name}", plugin_path
            )
            if spec and spec.loader:
                module = importlib.util.module_from_spec(spec)
                sys.modules[spec.name] = module
                spec.loader.exec_module(module)
                return module
            else:
                raise ImportError(f"Could not load spec for {plugin_name}")
        except Exception as e:
            raise ImportError(f"Failed to load plugin {plugin_name}: {e}")

    def _get_plugin_instance(self, plugin_name: str):
        """Load module and return the plugin class instance."""
        module = self._load_plugin_module(plugin_name)
        
        # Find the class that inherits from PluginBase
        # We need to import PluginBase to check inheritance
        from plugins.base import PluginBase

        for attr_name in dir(module):
            attr = getattr(module, attr_name)
            if (
                isinstance(attr, type)
                and issubclass(attr, PluginBase)
                and attr is not PluginBase
            ):
                return attr()
        
        raise ValueError(f"No valid PluginBase subclass found in {plugin_name}")

    def get_plugin_schema(self, plugin_name: str) -> Dict[str, Any]:
        """Get the configuration schema for a plugin."""
        try:
            instance = self._get_plugin_instance(plugin_name)
            return instance.get_config_schema()
        except Exception as e:
            print(f"[ERROR] Failed to get schema for plugin {plugin_name}: {e}")
            return {}

    def get_plugin_description(self, plugin_name: str) -> str:
        """Get the description of a plugin."""
        try:
            instance = self._get_plugin_instance(plugin_name)
            return instance.get_description()
        except Exception as e:
            # print(f"[ERROR] Failed to get description for plugin {plugin_name}: {e}")
            return ""

    def run_plugin_in_process(
        self, plugin_name: str, config: Dict[str, Any]
    ) -> Dict[str, Any]:
        """
        Execute a plugin with the given configuration in-process.
        Captures stderr for logs.
        INTERNAL USE ONLY (or for --run-plugin handler).
        """
        try:
            instance = self._get_plugin_instance(plugin_name)
            
            # Capture stdout and stderr
            log_capture = io.StringIO()
            
            with contextlib.redirect_stderr(log_capture), contextlib.redirect_stdout(log_capture):
                try:
                    result = instance.run(config)
                except Exception as e:
                    traceback.print_exc()
                    result = {"status": "error", "message": str(e)}

            logs = log_capture.getvalue()
            
            # Attach logs to result
            if isinstance(result, dict):
                result["logs"] = logs
            
            return result

        except Exception as e:
            return {
                "status": "error", 
                "message": f"Plugin execution failed: {e}",
                "logs": str(e)
            }

    def execute_plugin(
        self, plugin_name: str, config: Dict[str, Any]
    ) -> Dict[str, Any]:
        """
        Execute a plugin with the given configuration.
        Spawns a subprocess to avoid blocking the main thread.
        """
        import subprocess
        config_json = json.dumps(config)
        
        # Determine command
        if getattr(sys, "frozen", False):
            # Frozen: Call app exe with --run-plugin
            cmd = [sys.executable, "--run-plugin", plugin_name, "--plugin-config", config_json]
        else:
            # Dev: Call python with plugin file
            plugin_path = self.get_plugin_path(plugin_name)
            if not plugin_path:
                 return {"status": "error", "message": f"Plugin not found: {plugin_name}"}
            cmd = [sys.executable, str(plugin_path), "--config", config_json]

        try:
            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            try:
                output = json.loads(result.stdout)
                if isinstance(output, dict) and result.stderr:
                     output["logs"] = result.stderr
                return output
            except json.JSONDecodeError:
                return {
                    "status": "error", 
                    "message": f"Invalid output: {result.stdout}", 
                    "logs": result.stderr,
                    "raw_output": result.stdout
                }
        except subprocess.CalledProcessError as e:
            return {
                "status": "error",
                "message": f"Execution failed: {e.stderr}",
                "logs": e.stderr
            }
        except Exception as e:
             return {"status": "error", "message": str(e)}


    def execute_plugin_stream(self, plugin_name: str, config: Dict[str, Any]):
        """
        Execute a plugin and yield output line-by-line for real-time logging.
        Yields strings (log lines) or the final result dict.
        """
        import subprocess
        config_json = json.dumps(config)
        
        # Determine command
        if getattr(sys, "frozen", False):
            cmd = [sys.executable, "--run-plugin", plugin_name, "--plugin-config", config_json]
        else:
            plugin_path = self.get_plugin_path(plugin_name)
            if not plugin_path:
                 yield {"status": "error", "message": f"Plugin not found: {plugin_name}"}
                 return
            cmd = [sys.executable, str(plugin_path), "--config", config_json]

        # Use Popen to stream output
        try:
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
                universal_newlines=True,
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
                    "logs": "See above logs",
                }
            else:
                try:
                    result = json.loads(stdout)
                    yield result
                except json.JSONDecodeError:
                    yield {
                        "status": "error",
                        "message": f"Invalid JSON output: {stdout}",
                        "logs": "See above logs",
                    }
        except Exception as e:
             yield {"status": "error", "message": str(e)}


if __name__ == "__main__":
    # Simple test
    manager = PluginManager()
    print(f"Available plugins: {manager.get_available_plugins()}")
    if "local" in manager.get_available_plugins():
        print("Schema for local:", manager.get_plugin_schema("local"))



