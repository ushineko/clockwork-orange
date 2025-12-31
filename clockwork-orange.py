#!/usr/bin/env python
import argparse
import configparser
import json
import mimetypes
import random
import signal
import subprocess
import sys
import tempfile
import time
import platform_utils
from pathlib import Path
from threading import Event

import requests
import yaml
from watchdog.events import FileSystemEventHandler
from watchdog.observers import Observer

from plugin_manager import PluginManager

# GUI imports (optional)
try:
    from gui.main_window import main as gui_main

    GUI_AVAILABLE = True
except ImportError as e:
    print(f"GUI Import Error: {e}")
    GUI_AVAILABLE = False

# Supported image file extensions
IMAGE_EXTENSIONS = {".jpg", ".jpeg", ".png", ".bmp", ".gif", ".tiff", ".webp", ".svg"}

# Global flag for graceful shutdown
shutdown_requested = False


def signal_handler(signum, frame):
    """Handle interrupt signals gracefully."""
    global shutdown_requested
    print(f"\n[DEBUG] Received signal {signum}, shutting down gracefully...")
    shutdown_requested = True


def is_image_file(file_path: Path) -> bool:
    """Check if a file is a supported image format."""
    if not file_path.is_file():
        return False

    # Check by extension first (faster)
    if file_path.suffix.lower() in IMAGE_EXTENSIONS:
        return True

    # Fallback to MIME type detection
    mime_type, _ = mimetypes.guess_type(str(file_path))
    if mime_type and mime_type.startswith("image/"):
        return True

    return False


def get_random_image_from_directory(directory: Path) -> Path:
    """Get a random image file from the specified directory."""
    print(f"[DEBUG] Searching for image files in directory: {directory}")

    if not directory.is_dir():
        raise ValueError(f"Path is not a directory: {directory}")

    # List all files in directory for debugging
    all_files = list(directory.iterdir())
    print(f"[DEBUG] All files in directory ({len(all_files)}):")
    for f in all_files:
        print(f"[DEBUG]   - {f.name} (is_file: {f.is_file()}, suffix: {f.suffix})")

    # Find all image files in the directory
    image_files = [f for f in directory.iterdir() if is_image_file(f)]

    print(f"[DEBUG] Image files found ({len(image_files)}):")
    for f in image_files:
        print(f"[DEBUG]   - {f.name}")

    if not image_files:
        raise ValueError(f"No image files found in directory: {directory}")

    selected_file = random.choice(image_files)
    print(f"[DEBUG] Randomly selected: {selected_file}")
    print(f"[DEBUG] Selected file exists: {selected_file.exists()}")
    print(f"[DEBUG] Selected file is file: {selected_file.is_file()}")

    return selected_file


def set_wallpaper(p: Path):
    print(f"[DEBUG] Setting wallpaper from path: {p}")
    p = p.resolve()
    print(f"[DEBUG] Resolved absolute path: {p}")

    if not p.exists():
        print(f"[ERROR] File does not exist: {p}")
        return False

    file_size = p.stat().st_size
    print(f"[DEBUG] File size: {file_size} bytes")

    return platform_utils.set_wallpaper(p)


def download_and_set_wallpaper(url: str):
    """Download image from URL and set as wallpaper."""
    print(f"[DEBUG] Downloading image from: {url}")

    try:
        response = requests.get(url, timeout=30)
        print(f"[DEBUG] HTTP response status: {response.status_code}")
        print(f"[DEBUG] Content length: {len(response.content)} bytes")
        print(
            f"[DEBUG] Content type: {response.headers.get('content-type', 'unknown')}"
        )

        if response.status_code != 200:
            print(
                f"[ERROR] Failed to download image. HTTP status: {response.status_code}"
            )
            return False

    except requests.exceptions.RequestException as e:
        print(f"[ERROR] Network error while downloading image: {e}")
        return False

    with tempfile.NamedTemporaryFile(delete=False, suffix=".jpg") as f:
        p = Path(f.name).resolve()
        print(f"[DEBUG] Created temporary file: {p}")

        try:
            p.write_bytes(response.content)
            print(
                f"[DEBUG] Successfully wrote {len(response.content)} bytes to temporary file"
            )
        except Exception as e:
            print(f"[ERROR] Failed to write image to temporary file: {e}")
            return False

        return set_wallpaper(p)


def set_local_wallpaper(file_path: Path):
    """Set wallpaper from a local file."""
    file_path = file_path.resolve()
    print(f"[DEBUG] Setting wallpaper from local file: {file_path}")

    if not file_path.exists():
        print(f"[ERROR] File does not exist: {file_path}")
        return False

    if not is_image_file(file_path):
        print(f"[ERROR] File is not a supported image format: {file_path}")
        return False

    return set_wallpaper(file_path)


def get_random_image_from_sources(sources: list) -> Path:
    """Get a random image, fairly selecting between sources first."""
    print(f"[DEBUG] Selecting from {len(sources)} sources...")

    valid_sources = _gather_valid_sources(sources)

    if not valid_sources:
        raise ValueError("No valid sources found")

    # Shuffle to pick a random source order
    random.shuffle(valid_sources)

    for source in valid_sources:
        candidate = _select_candidate_from_source(source)
        if candidate:
            print(f"[DEBUG] Selected image from source {source}: {candidate}")
            return candidate

    raise ValueError(
        f"No image files found in any of the {len(valid_sources)} provided sources"
    )


def _gather_valid_sources(sources: list) -> list:
    """Resolve and filter valid source paths."""
    valid_sources = []
    for s in sources:
        path = Path(s).resolve()
        if path.exists():
            valid_sources.append(path)
        else:
            print(f"[WARN] Source does not exist: {path}")
    return valid_sources


def _select_candidate_from_source(source: Path):
    """Attempt to find an image candidate from a single source."""
    if source.is_file():
        if is_image_file(source):
            return source
    elif source.is_dir():
        try:
            candidates = [f for f in source.iterdir() if is_image_file(f)]
            if candidates:
                return random.choice(candidates)
        except Exception as e:
            print(f"[ERROR] Failed to scan {source}: {e}")
    return None


def set_random_wallpaper_from_sources(sources: list):
    """Set wallpaper from a random image in the specified sources."""
    try:
        image_file = get_random_image_from_sources(sources)
        return set_wallpaper(image_file)
    except ValueError as e:
        print(f"[ERROR] {e}")
        return False


def set_random_wallpaper_from_directory(directory_path: Path):
    """Set wallpaper from a random image in the specified directory."""
    return set_random_wallpaper_from_sources([directory_path])


def cycle_wallpapers_from_sources(sources: list, wait_seconds: int):
    """Continuously cycle through random wallpapers from sources with wait intervals."""
    print(f"[DEBUG] Starting continuous wallpaper cycling from {len(sources)} sources")
    print(f"[DEBUG] Wait interval: {wait_seconds} seconds")
    print(f"[DEBUG] Press Ctrl+C to stop")

    # Set up signal handlers for graceful shutdown
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    cycle_count = 0

    while not shutdown_requested:
        cycle_count += 1
        print(f"[DEBUG] Cycle #{cycle_count}")

        # Re-resolve dynamic sources here?
        # User said: "when the checkbox... is enabled/disabled... the directory... will be added/removed".
        # This implies we need to RELOAD config every cycle if we want true dynamic behavior without restart.
        # But for now, let's assume 'sources' is passed in.
        # If we want dynamic, the Caller (main) should be inside the loop or we reload config here.
        # Doing config reload every 30s is fine.
        # But 'sources' argument makes it static.
        # I will implement static sources here first. Dynamic reloading requires more refactoring of 'main'.

        try:
            success = set_random_wallpaper_from_sources(sources)
            if success:
                print(f"[DEBUG] Wallpaper set successfully (cycle #{cycle_count})")
            else:
                print(f"[ERROR] Failed to set wallpaper (cycle #{cycle_count})")
        except Exception as e:
            print(f"[ERROR] Unexpected error during cycle #{cycle_count}: {e}")

        if not shutdown_requested:
            print(f"[DEBUG] Waiting {wait_seconds} seconds before next cycle...")
            # Sleep in small increments to be responsive to interrupts
            for _ in range(wait_seconds):
                if shutdown_requested:
                    break
                time.sleep(1)

    print(f"[DEBUG] Wallpaper cycling stopped after {cycle_count} cycles")


def cycle_wallpapers_from_directory(directory_path: Path, wait_seconds: int):
    return cycle_wallpapers_from_sources([directory_path], wait_seconds)


def set_lockscreen_wallpaper(image_path: Path):
    """Set lock screen wallpaper."""
    print(f"[DEBUG] set_lockscreen_wallpaper called with: {image_path}")

    image_path = image_path.resolve()
    print(f"[DEBUG] Resolved path: {image_path}")

    if not image_path.exists():
        print(f"[ERROR] Image file does not exist: {image_path}")
        return False

    if not is_image_file(image_path):
        print(f"[ERROR] File is not a supported image format: {image_path}")
        return False

    return platform_utils.set_lockscreen_wallpaper(image_path)


def set_lockscreen_random_from_directory(directory_path: Path):
    """Set random lock screen wallpaper from directory."""
    try:
        image_file = get_random_image_from_directory(directory_path)
        return set_lockscreen_wallpaper(image_file)
    except ValueError as e:
        print(f"[ERROR] {e}")
        return False


def cycle_lockscreen_wallpapers_from_directory(directory_path: Path, wait_seconds: int):
    """Continuously cycle through random lock screen wallpapers from directory with wait intervals."""
    print(
        f"[DEBUG] Starting continuous lock screen wallpaper cycling from directory: {directory_path}"
    )
    print(f"[DEBUG] Wait interval: {wait_seconds} seconds")
    print(f"[DEBUG] Press Ctrl+C to stop")

    # Set up signal handlers for graceful shutdown
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    cycle_count = 0

    while not shutdown_requested:
        cycle_count += 1
        print(f"[DEBUG] Lock screen cycle #{cycle_count}")

        try:
            success = set_lockscreen_random_from_directory(directory_path)
            if success:
                print(
                    f"[DEBUG] Lock screen wallpaper set successfully (cycle #{cycle_count})"
                )
            else:
                print(
                    f"[ERROR] Failed to set lock screen wallpaper (cycle #{cycle_count})"
                )
        except Exception as e:
            print(
                f"[ERROR] Unexpected error during lock screen cycle #{cycle_count}: {e}"
            )

        if not shutdown_requested:
            print(
                f"[DEBUG] Waiting {wait_seconds} seconds before next lock screen cycle..."
            )
            # Sleep in small increments to be responsive to interrupts
            for _ in range(wait_seconds):
                if shutdown_requested:
                    break
                time.sleep(1)

    print(f"[DEBUG] Lock screen wallpaper cycling stopped after {cycle_count} cycles")


def get_two_different_images_from_directory(directory: Path):
    """Get two different random image files from the specified directory."""
    print(f"[DEBUG] Searching for image files in directory: {directory}")

    if not directory.is_dir():
        raise ValueError(f"Path is not a directory: {directory}")

    # Find all image files in the directory
    image_files = [f for f in directory.iterdir() if is_image_file(f)]

    print(f"[DEBUG] Found {len(image_files)} image files")

    if len(image_files) < 2:
        raise ValueError(
            f"Need at least 2 image files in directory, found {len(image_files)}: {directory}"
        )

    # Select two different images
    selected_files = random.sample(image_files, 2)
    print(f"[DEBUG] Selected two different images:")
    print(f"[DEBUG]   - Image 1: {selected_files[0]}")
    print(f"[DEBUG]   - Image 2: {selected_files[1]}")

    return selected_files[0], selected_files[1]


def set_dual_wallpapers_from_directory(directory_path: Path):
    """Set both desktop and lock screen wallpapers from different random images in directory."""
    try:
        desktop_image, lockscreen_image = get_two_different_images_from_directory(
            directory_path
        )

        print(f"[DEBUG] Setting desktop wallpaper: {desktop_image}")
        desktop_success = set_wallpaper(desktop_image)

        print(f"[DEBUG] Setting lock screen wallpaper: {lockscreen_image}")
        lockscreen_success = set_lockscreen_wallpaper(lockscreen_image)

        return desktop_success and lockscreen_success
    except ValueError as e:
        print(f"[ERROR] {e}")
        return False


def set_dual_wallpapers_from_files(desktop_file: Path, lockscreen_file: Path):
    """Set both desktop and lock screen wallpapers from specific files."""
    print(f"[DEBUG] Setting desktop wallpaper: {desktop_file}")
    desktop_success = set_wallpaper(desktop_file)

    print(f"[DEBUG] Setting lock screen wallpaper: {lockscreen_file}")
    lockscreen_success = set_lockscreen_wallpaper(lockscreen_file)

    return desktop_success and lockscreen_success


def cycle_dual_wallpapers_from_directory(directory_path: Path, wait_seconds: int):
    """Continuously cycle through different random wallpapers for both desktop and lock screen."""
    print(
        f"[DEBUG] Starting continuous dual wallpaper cycling from directory: {directory_path}"
    )
    print(f"[DEBUG] Wait interval: {wait_seconds} seconds")
    print(f"[DEBUG] Press Ctrl+C to stop")

    # Set up signal handlers for graceful shutdown
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    cycle_count = 0

    while not shutdown_requested:
        cycle_count += 1
        print(f"[DEBUG] Dual wallpaper cycle #{cycle_count}")

        try:
            success = set_dual_wallpapers_from_directory(directory_path)
            if success:
                print(
                    f"[DEBUG] Both wallpapers set successfully (cycle #{cycle_count})"
                )
            else:
                print(
                    f"[ERROR] Failed to set one or both wallpapers (cycle #{cycle_count})"
                )
        except Exception as e:
            print(
                f"[ERROR] Unexpected error during dual wallpaper cycle #{cycle_count}: {e}"
            )

        if not shutdown_requested:
            print(
                f"[DEBUG] Waiting {wait_seconds} seconds before next dual wallpaper cycle..."
            )
            # Sleep in small increments to be responsive to interrupts
            for _ in range(wait_seconds):
                if shutdown_requested:
                    break
                time.sleep(1)

    print(f"[DEBUG] Dual wallpaper cycling stopped after {cycle_count} cycles")


def load_config_file():
    """Load configuration from file."""
    config_paths = [
        Path.home() / ".config" / "clockwork-orange.yml",
        Path("C:/Users/Public/clockwork_config.yml"),  # Shared config for service
    ]

    for config_path in config_paths:
        if config_path.exists():
            try:
                print(f"[DEBUG] Loading configuration from {config_path}")
                with open(config_path, "r") as f:
                    return yaml.safe_load(f) or {}
            except Exception as e:
                print(f"[ERROR] Failed to load config file {config_path}: {e}")
    
    # Defaults handled by caller
    return {}



def merge_config_with_args(config, args):
    """Merge configuration file options with command line arguments."""
    print(f"[DEBUG] Merging configuration with command line arguments")

    # Create a copy of args to avoid modifying the original
    merged_args = argparse.Namespace(**vars(args))

    # Apply configuration defaults if not specified on command line
    if hasattr(config, "get"):
        # Desktop wallpaper settings
        if (
            not merged_args.desktop
            and not merged_args.lockscreen
            and config.get("desktop", False)
        ):
            merged_args.desktop = True
            print(f"[DEBUG] Enabled desktop mode from config")

        # Lock screen settings
        if (
            not merged_args.lockscreen
            and not merged_args.desktop
            and config.get("lockscreen", False)
        ):
            merged_args.lockscreen = True
            print(f"[DEBUG] Enabled lock screen mode from config")

        # Dual wallpaper settings
        if config.get("dual_wallpapers", False):
            merged_args.desktop = True
            merged_args.lockscreen = True
            print(f"[DEBUG] Enabled dual wallpaper mode from config")

        # Default wait interval (only if wait not specified)
        if not merged_args.wait:
            if config.get("default_wait"):
                merged_args.wait = config["default_wait"]
                print(
                    f"[DEBUG] Set default wait interval from config: {merged_args.wait}"
                )
            elif merged_args.service:
                # Service mode fallback default
                merged_args.wait = 900
                print(f"[DEBUG] Service mode: defaulting to 900s wait interval")

        # Default plugin
        # Default plugin
        # We do NOT want to force a single plugin here.
        # If no plugin is specified, we want 'main' to detect this and enter Multi-Plugin Mode.
        # So we remove the fallback logic entirely.
        pass

    return merged_args

    return merged_args


def write_config_file(args):
    """Write configuration file based on current command line arguments."""
    config_path = Path.home() / ".config" / "clockwork-orange.yml"
    print(f"[DEBUG] Writing configuration file to: {config_path}")

    config_path.parent.mkdir(parents=True, exist_ok=True)

    config = {}
    config.update(_get_wallpaper_config(args))
    config.update(_get_default_source_config(args))

    if args.wait:
        config["default_wait"] = args.wait

    try:
        with open(config_path, "w") as f:
            yaml.dump(config, f, default_flow_style=False, sort_keys=True)
        return True
    except Exception as e:
        print(f"[ERROR] Failed to write configuration file: {e}")
        return False


def _get_wallpaper_config(args):
    config = {}
    if args.desktop and args.lockscreen:
        config["dual_wallpapers"] = True
    elif args.desktop:
        config["desktop"] = True
    elif args.lockscreen:
        config["lockscreen"] = True
    return config


def _get_default_source_config(args):
    config = {}
    if args.url:
        config["default_url"] = args.url
    elif args.file:
        config["default_file"] = str(args.file)
    elif args.directory:
        config["default_directory"] = str(args.directory)
    return config


def debug_lockscreen_config():
    """Debug function to show current lock screen configuration."""
    config_path = Path.home() / ".config" / "kscreenlockerrc"

    if not config_path.exists():
        print(f"[DEBUG] Configuration file does not exist: {config_path}")
        return

    try:
        config = configparser.ConfigParser()
        config.read(config_path)
        _print_config_sections(config)
        _check_lockscreen_wallpaper_setting(config)
        _check_greeter_wallpaper_setting(config)
    except Exception as e:
        print(f"[ERROR] Failed to read configuration file: {e}")


def _print_config_sections(config):
    print(f"[DEBUG] Configuration sections found:")
    for section in config.sections():
        print(f"[DEBUG]   - {section}")
        for key, value in config.items(section):
            print(f"[DEBUG]     {key} = {value}")


def _check_lockscreen_wallpaper_setting(config):
    wallpaper_section = "Greeter][Wallpaper][org.kde.image][General"
    if config.has_section(wallpaper_section):
        if config.has_option(wallpaper_section, "Image"):
            print(
                f"[DEBUG] Current lock screen wallpaper: {config.get(wallpaper_section, 'Image')}"
            )
        elif config.has_option(wallpaper_section, "image"):
            print(
                f"[DEBUG] Current lock screen wallpaper: {config.get(wallpaper_section, 'image')}"
            )
        else:
            print(f"[DEBUG] No Image key found in {wallpaper_section}")
    else:
        print(f"[DEBUG] Wallpaper section {wallpaper_section} not found")


def _check_greeter_wallpaper_setting(config):
    if config.has_section("Greeter") and config.has_option("Greeter", "wallpaper"):
        print(f"[DEBUG] Main Greeter wallpaper: {config.get('Greeter', 'wallpaper')}")
    else:
        print(f"[DEBUG] No wallpaper key found in Greeter section")


def collect_plugin_sources(config, plugin_manager):
    """Collect source paths from all enabled plugins."""
    sources = []
    plugins_config = config.get("plugins", {})

    for name, plugin_cfg in plugins_config.items():
        if plugin_cfg.get("enabled", False):
            # print(f"[DEBUG] Processing enabled plugin: {name}")
            try:
                result = plugin_manager.execute_plugin(name, plugin_cfg)
                if result.get("status") == "success":
                    path = result.get("path")
                    if path:
                        sources.append(Path(path))
            except Exception as e:
                print(f"[ERROR] Failed to execute plugin {name}: {e}")

    return sources


def set_dual_wallpaper_from_sources(sources: list):
    """Set both desktop and lock screen wallpapers from different random images in sources."""
    try:
        image1 = get_random_image_from_sources(sources)
        image2 = get_random_image_from_sources(sources)

        # Simple retry if same image picked
        if image1 == image2:
            image2 = get_random_image_from_sources(sources)

        print(f"[DEBUG] Setting desktop wallpaper: {image1}")
        desktop_success = set_wallpaper(image1)

        print(f"[DEBUG] Setting lock screen wallpaper: {image2}")
        lockscreen_success = set_lockscreen_wallpaper(image2)

        return desktop_success and lockscreen_success
    except ValueError as e:
        print(f"[ERROR] {e}")
        return False


def cycle_dynamic_plugins(
    plugin_manager: PluginManager,
    wait_seconds: int,
    desktop: bool = True,
    lockscreen: bool = False,
):
    """Continuously cycle wallpapers using enabled plugins, reloading config each time."""
    print(f"[DEBUG] Starting dynamic plugin cycling")
    print(f"[DEBUG] Wait interval: {wait_seconds} seconds")
    print(f"[DEBUG] Desktop: {desktop}, Lockscreen: {lockscreen}")
    print(f"[DEBUG] Press Ctrl+C to stop")

    # Initialize Watchdog
    config_path = Path.home() / ".config" / "clockwork-orange.yml"
    config_change_event = Event()
    observer = Observer()
    watcher = ConfigWatcher(config_path, config_change_event)

    # Ensure config dir exists
    config_path.parent.mkdir(parents=True, exist_ok=True)

    observer.schedule(watcher, str(config_path.parent), recursive=False)
    observer.start()
    print(f"[DEBUG] Started configuration watcher on {config_path}")

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    cycle_count = 0

    try:
        while not shutdown_requested:
            cycle_count += 1
            print(f"[DEBUG] Cycle #{cycle_count}")

            # Execute one cycle
            config = _execute_dynamic_cycle(plugin_manager, desktop, lockscreen)

            if not shutdown_requested:
                _wait_for_next_cycle(config, wait_seconds, config_change_event)
    finally:
        observer.stop()
        observer.join()
        print(f"[DEBUG] Watcher stopped")

    print(f"[DEBUG] Dynamic cycling stopped")


def clean_config(config):
    """Clean up invalid plugins from configuration."""
    if not config.get("plugins"):
        return

    plugin_manager = PluginManager()
    available = plugin_manager.get_available_plugins()

    plugins = config["plugins"]
    to_remove = []

    for name in plugins:
        if name not in available:
            to_remove.append(name)

    if to_remove:
        print(
            f"[DEBUG] Cleaning up invalid plugins from config: {', '.join(to_remove)}"
        )
        for name in to_remove:
            del config["plugins"][name]

        # Write back changes
        config_path = Path.home() / ".config" / "clockwork-orange.yml"
        try:
            # Create a simplified args object for write_config_file equivalent
            # But simpler here since we just dump the dict
            with open(config_path, "w") as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=True)
            print(f"[DEBUG] Configuration cleaned and saved")
        except Exception as e:
            print(f"[ERROR] Failed to save cleaned configuration: {e}")


def main():
    # Helper: specific check to default to GUI on double-click (Windows mostly)
    if len(sys.argv) == 1:
        sys.argv.append("--gui")

    start_time = time.time()
    
    parser = _create_argument_parser()
    args = parser.parse_args()
    
    # Only print debug messages if not running a plugin (to keep JSON output clean)
    if not hasattr(args, 'run_plugin') or not args.run_plugin:
        print(f"[DEBUG] Startup initiated at {start_time}")
        print(f"[DEBUG] Arguments parsed in {time.time() - start_time:.4f}s")


    # Handle Plugin Execution (App-as-Interpreter)
    if args.run_plugin:
        try:
            from plugin_manager import PluginManager
            
            plugin_name = args.run_plugin
            config = {}
            if args.plugin_config:
                try:
                    config = json.loads(args.plugin_config)
                except json.JSONDecodeError:
                    print(json.dumps({"status": "error", "message": "Invalid config JSON"}))
                    sys.exit(1)

            # We use the internal method that runs in-process
            # Redirect stdout to stderr so logs are streamed
            # Final result is printed to original stdout
            sys.stdout = sys.stderr
            try:
                pm = PluginManager()
                # Access private method directly to avoid capture overhead of other methods
                instance = pm._get_plugin_instance(plugin_name)
                result = instance.run(config)
            finally:
                sys.stdout = sys.__stdout__

            print(json.dumps(result))
            sys.exit(0)
        except Exception as e:
            # Restore stdout just in case
            sys.stdout = sys.__stdout__
            error = {"status": "error", "message": str(e), "logs": str(e)}
            print(json.dumps(error))
            sys.exit(1)


    # Handle Self-Test
    if args.self_test:
        print("Running Cloudwork Orange Self-Test...")
        results = {}
        
        # Test 1: Python Info
        print(f"Python: {sys.version}")
        print(f"Platform: {sys.platform}")
        print(f"Frozen: {getattr(sys, 'frozen', False)}")
        
        # Test 2: Critical Imports
        modules = ["ctypes", "sqlite3", "ssl", "PIL", "requests", "yaml", "watchdog"]
        for mod in modules:
            try:
                __import__(mod)
                print(f"[OK] Import {mod}")
                results[mod] = True
            except ImportError as e:
                print(f"[FAIL] Import {mod}: {e}")
                results[mod] = False
        
        # Test 3: SSL/Network
        try:
            import requests
            print("Testing Network/SSL...")
            requests.get("https://www.google.com", timeout=5)
            print("[OK] Network/SSL Request")
            results["network"] = True
        except Exception as e:
            print(f"[FAIL] Network/SSL Request: {e}")
            results["network"] = False

        # Test 4: Plugins
        try:
            from plugin_manager import PluginManager
            pm = PluginManager()
            plugins = pm.get_available_plugins()
            print(f"Plugins Found: {plugins}")
            if not plugins:
                 print("[WARN] No plugins found!")
            results["plugins_count"] = len(plugins)
        except Exception as e:
            print(f"[FAIL] Plugin System: {e}")
        
        sys.exit(0 if all(results.values()) else 1)


    # Load configuration file and merge with command line arguments
    config = load_config_file()

    # Auto-clean configuration
    clean_config(config)

    args = merge_config_with_args(config, args)

    # Handle GUI option early
    _handle_gui_mode(args)

    # Check for implicit multi-plugin mode capability
    has_enabled_plugins = False
    if config.get("plugins"):
        for p_cfg in config["plugins"].values():
            if p_cfg.get("enabled"):
                has_enabled_plugins = True
                break

    # Validate arguments
    _validate_args(parser, args, has_enabled_plugins)

    # Handle debug option
    if args.debug_lockscreen:
        debug_lockscreen_config()
        return

    # Handle write-config option
    if args.write_config:
        success = write_config_file(args)
        if success:
            print("[DEBUG] Configuration file written successfully")
            sys.exit(0)
        else:
            print("[ERROR] Failed to write configuration file")
            sys.exit(1)

    plugin_manager = PluginManager()

    # Resolve plugin if specified
    if args.plugin:
        _handle_specific_plugin_execution(args, config, plugin_manager)

    target_desc = _get_target_description(args)
    print(f"[DEBUG] Starting {target_desc} set process")

    success = _perform_wallpaper_operation(args, config, plugin_manager)

    if success:
        print(f"[DEBUG] {target_desc.capitalize()} set successfully")
        print("[DEBUG] Process completed")
    else:
        print(f"[ERROR] Failed to set {target_desc}")
        sys.exit(1)


def _create_argument_parser():
    """Create and return the argument parser."""
    plugin_manager = PluginManager()
    available_plugins = plugin_manager.get_available_plugins()

    parser = argparse.ArgumentParser(
        description="Set wallpaper or lock screen from URL, local file, or random image from directory (KDE Plasma 6 only)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
IMPORTANT: This script is designed specifically for KDE Plasma 6 and requires
qdbus6 and kwriteconfig6 commands. It will not work with older KDE versions.

Examples:
  # Regular wallpaper operations:
  %(prog)s                           # Download from default URL (https://pic.re/image)
  %(prog)s -u https://example.com/img.jpg  # Download from custom URL
  %(prog)s -f /path/to/image.jpg     # Set from local file
  %(prog)s -d /path/to/wallpapers    # Set random image from directory
  %(prog)s -d /path/to/wallpapers -w 30  # Cycle random wallpapers every 30 seconds
  
  # Lock screen operations:
  %(prog)s --lockscreen -f /path/to/image.jpg     # Set lock screen from local file
  %(prog)s --lockscreen -d /path/to/wallpapers    # Set random lock screen from directory
  %(prog)s --lockscreen -d /path/to/wallpapers -w 60  # Cycle lock screen every 60 seconds
  
  # Dual wallpaper operations (different images for desktop and lock screen):
  %(prog)s --desktop --lockscreen -d /path/to/wallpapers    # Set different random wallpapers
  %(prog)s --desktop --lockscreen -d /path/to/wallpapers -w 120  # Cycle both every 2 minutes

Configuration File:
  The script also supports a configuration file at ~/.config/clockwork-orange.yml
  Example configuration:
    dual_wallpapers: true
    default_directory: /path/to/wallpapers
    default_wait: 300

  Create initial config file:
  %(prog)s --desktop --lockscreen -d /path/to/wallpapers -w 300 --write-config
  
  # Start graphical user interface:
  %(prog)s --gui
        """,
    )

    # Target selection
    parser.add_argument(
        "--lockscreen",
        action="store_true",
        help="Set lock screen wallpaper instead of desktop wallpaper",
    )
    parser.add_argument(
        "--desktop",
        action="store_true",
        help="Set desktop wallpaper (can be combined with operations)",
    )

    group = parser.add_mutually_exclusive_group()
    group.add_argument(
        "-u", "--url", help="Download image from URL and set as wallpaper (Legacy)"
    )
    group.add_argument(
        "-f", "--file", type=Path, help="Set wallpaper from local file (Legacy)"
    )
    group.add_argument(
        "-d",
        "--directory",
        type=Path,
        help="Set random wallpaper from directory (Legacy)",
    )
    group.add_argument(
        "--plugin",
        choices=available_plugins,
        help=f'Use a specific plugin source. Available: {", ".join(available_plugins)}',
    )

    parser.add_argument(
        "--plugin-config",
        type=str,
        help="JSON configuration string for the plugin (overrides config file)",
    )

    parser.add_argument(
        "-w",
        "--wait",
        type=int,
        metavar="SECONDS",
        help="Wait specified seconds between wallpaper changes (only works with -d/--directory)",
    )

    parser.add_argument(
        "--debug-lockscreen",
        action="store_true",
        help="Show current lock screen configuration for debugging",
    )

    parser.add_argument(
        "--write-config",
        action="store_true",
        help="Write configuration file based on current command line options and exit",
    )

    parser.add_argument(
        "--gui", action="store_true", help="Start the graphical user interface"
    )

    parser.add_argument(
        "--service",
        action="store_true",
        help="Run in background service mode (sets default wait to 900s if unspecified)",
    )

    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-diagnostic to verify environment and dependencies",
    )

    parser.add_argument(
        "--run-plugin",
        help="Run a specific plugin (internal use for frozen builds)",
    )

    return parser



def _validate_args(parser, args, has_enabled_plugins):
    """Validate command line arguments."""
    # We allow running without specific args IF we have enabled plugins (Multi-Plugin Mode)
    if (
        not args.file
        and not args.directory
        and not args.plugin
        and not has_enabled_plugins
    ):
        parser.error(
            "Operation requires either --file, --directory, --plugin, or enabled plugins in config"
        )

    if args.wait is not None and args.wait <= 0:
        parser.error("--wait must be a positive integer")

    # Validate URL with lockscreen
    if args.url and args.lockscreen:
        parser.error(
            "--url cannot be used with --lockscreen (lock screen requires local files)"
        )

    # Validate dual wallpaper mode
    if args.desktop and args.lockscreen:
        if args.url:
            parser.error(
                "--url cannot be used with dual wallpaper mode (lock screen requires local files)"
            )
        if (
            not args.file
            and not args.directory
            and not args.plugin
            and not has_enabled_plugins
        ):
            parser.error(
                "Dual wallpaper mode requires either --file, --directory, --plugin, or enabled plugins"
            )


def _handle_specific_plugin_execution(args, config, plugin_manager):
    """Handle execution of a specifically requested plugin via CLI."""
    print(f"[DEBUG] Plugin mode: {args.plugin}")
    # Determine config for plugin
    plugin_config = {}

    # Check for config in file first
    if config.get("plugins", {}).get(args.plugin):
        plugin_config = config["plugins"][args.plugin]

    # CLI config overrides file config
    if args.plugin_config:
        try:
            cli_config = json.loads(args.plugin_config)
            plugin_config.update(cli_config)
        except json.JSONDecodeError:
            print(f"[ERROR] Invalid JSON in --plugin-config")
            sys.exit(1)

    # Execute plugin
    print(f"[DEBUG] Executing plugin {args.plugin} with config: {plugin_config}")
    result = plugin_manager.execute_plugin(args.plugin, plugin_config)

    if result.get("status") == "success":
        ret_path = Path(result["path"])
        print(f"[DEBUG] Plugin returned path: {ret_path}")

        if ret_path.is_dir():
            print(f"[DEBUG] Plugin resolved to directory")
            args.directory = ret_path
        elif ret_path.is_file():
            print(f"[DEBUG] Plugin resolved to file")
            args.file = ret_path
        else:
            print(f"[ERROR] Plugin returned invalid path: {ret_path}")
            sys.exit(1)
    else:
        print(f"[ERROR] Plugin execution failed: {result.get('message')}")
        sys.exit(1)


def _get_target_description(args):
    """Get human-readable description of the target."""
    if args.desktop and args.lockscreen:
        return "both desktop and lock screen"
    elif args.lockscreen:
        return "lock screen"
    elif args.desktop:
        return "desktop"
    else:
        return "wallpaper"


def _handle_dual_mode(args, config, plugin_manager):
    """Handle dual wallpaper operations (both desktop and lock screen)."""
    if args.file:
        print(
            f"[ERROR] Dual wallpaper mode with single file not supported (need different images)"
        )
        print(f"[ERROR] Use --desktop --lockscreen -d /path/to/directory instead")
        sys.exit(1)
    elif args.directory:
        print(f"[DEBUG] Dual wallpaper directory mode: {args.directory}")
        if args.wait is not None:
            # Continuous cycling mode for dual wallpapers
            print(
                f"[DEBUG] Dual wallpaper continuous mode with {args.wait} second intervals"
            )
            cycle_dual_wallpapers_from_directory(args.directory, args.wait)
            return True  # If we reach here, cycling completed successfully
        else:
            # Single dual wallpaper mode
            return set_dual_wallpapers_from_directory(args.directory)
    elif not args.plugin:
        # Implicit Dynamic Mode (Configured Plugins)
        if args.wait:
            cycle_dynamic_plugins(
                plugin_manager, args.wait, desktop=True, lockscreen=True
            )
            return True
        else:
            sources = collect_plugin_sources(config, plugin_manager)
            if sources:
                return set_dual_wallpaper_from_sources(sources)
            else:
                print("[ERROR] No enabled plugins found for dual wallpaper mode")
                return False
    return False


def _handle_lockscreen_mode(args, config, plugin_manager):
    """Handle lock screen wallpaper operations."""
    if args.file:
        print(f"[DEBUG] Lock screen file mode: {args.file}")
        return set_lockscreen_wallpaper(args.file)
    elif args.directory:
        print(f"[DEBUG] Lock screen directory mode: {args.directory}")
        if args.wait is not None:
            print(
                f"[DEBUG] Lock screen continuous mode with {args.wait} second intervals"
            )
            cycle_lockscreen_wallpapers_from_directory(args.directory, args.wait)
            return True
        else:
            return set_lockscreen_random_from_directory(args.directory)
    elif not args.plugin:
        # Implicit Dynamic Mode
        if args.wait:
            cycle_dynamic_plugins(
                plugin_manager, args.wait, desktop=False, lockscreen=True
            )
            return True
        else:
            sources = collect_plugin_sources(config, plugin_manager)
            if sources:
                img = get_random_image_from_sources(sources)
                return set_lockscreen_wallpaper(img)
            else:
                print("[ERROR] No enabled plugins found for lock screen mode")
                return False
    else:
        print("[ERROR] Lock screen mode requires either --file or --directory")
        sys.exit(1)


def _handle_desktop_mode(args, config, plugin_manager):
    """Handle desktop wallpaper operations."""
    if args.url:
        print(f"[DEBUG] Desktop URL mode: {args.url}")
        return download_and_set_wallpaper(args.url)
    elif args.file:
        print(f"[DEBUG] Desktop file mode: {args.file}")
        return set_local_wallpaper(args.file)
    elif args.directory:
        print(f"[DEBUG] Desktop directory mode: {args.directory}")
        if args.wait is not None:
            print(f"[DEBUG] Desktop continuous mode with {args.wait} second intervals")
            cycle_wallpapers_from_directory(args.directory, args.wait)
            return True
        else:
            return set_random_wallpaper_from_directory(args.directory)
    elif not args.plugin:
        # Implicit Dynamic Mode
        if args.wait:
            cycle_dynamic_plugins(
                plugin_manager, args.wait, desktop=True, lockscreen=False
            )
            return True
        else:
            sources = collect_plugin_sources(config, plugin_manager)
            if sources:
                return set_random_wallpaper_from_sources(sources)
            else:
                print("[ERROR] No enabled plugins found for desktop mode")
                return False
    else:
        print(
            "[ERROR] Desktop mode requires either --url, --file, --directory or enabled plugins"
        )
        sys.exit(1)


def _handle_default_mode(args, config, plugin_manager):
    """Handle default wallpaper operations (no explicit target)."""
    if args.url:
        print(f"[DEBUG] URL mode: {args.url}")
        return download_and_set_wallpaper(args.url)
    elif args.file:
        print(f"[DEBUG] File mode: {args.file}")
        return set_local_wallpaper(args.file)
    elif args.directory:
        print(f"[DEBUG] Directory mode: {args.directory}")
        if args.wait is not None:
            print(f"[DEBUG] Continuous mode with {args.wait} second intervals")
            cycle_wallpapers_from_directory(args.directory, args.wait)
            return True
        else:
            return set_random_wallpaper_from_directory(args.directory)
    else:
        # Check for enabled plugins (Dynamic Multi-Plugin Mode)
        initial_sources = collect_plugin_sources(config, plugin_manager)
        if initial_sources:
            print(
                f"[DEBUG] Multi-plugin mode active with {len(initial_sources)} sources"
            )
            if args.wait:
                cycle_dynamic_plugins(plugin_manager, args.wait)
                return True
            else:
                return set_random_wallpaper_from_sources(initial_sources)
        else:
            # Fallback if no source specified
            print(
                "[ERROR] No source specified and no plugins enabled. Use --plugin, --file, --directory, or --url."
            )
            sys.exit(1)


def _perform_wallpaper_operation(args, config, plugin_manager):
    """Dispatch wallpaper operation based on mode."""
    if args.desktop and args.lockscreen:
        return _handle_dual_mode(args, config, plugin_manager)
    elif args.lockscreen:
        return _handle_lockscreen_mode(args, config, plugin_manager)
    elif args.desktop:
        return _handle_desktop_mode(args, config, plugin_manager)
    else:
        return _handle_default_mode(args, config, plugin_manager)


def _handle_gui_mode(args):
    """Start GUI if requested."""
    if args.gui:
        if not GUI_AVAILABLE:
            print("[ERROR] GUI not available. Please install PyQt6: pip install PyQt6")
            input("Press Enter to exit...")
            sys.exit(1)
        print(f"[DEBUG] Starting GUI... SysArgv: {sys.argv}")
        try:
            sys.exit(gui_main())
        except Exception as e:
            print(f"[FATAL] GUI Crashed: {e}")
            import traceback
            traceback.print_exc()
            input("Press Enter to exit...")
            sys.exit(1)



def _clean_lockscreen_config():
    """Clean up redundant entries in kscreenlockerrc."""
    try:
        print(f"[DEBUG] Cleaning up redundant configuration entries...")
        subprocess.run(
            [
                "kwriteconfig6",
                "--file",
                "kscreenlockerrc",
                "--group",
                "Greeter",
                "--group",
                "Wallpaper",
                "--group",
                "org.kde.image",
                "--group",
                "General",
                "--key",
                "image",  # Remove lowercase image key
                "--delete",
            ],
            capture_output=True,
            text=True,
            check=False,
        )

        # Also remove the main Greeter wallpaper entry
        subprocess.run(
            [
                "kwriteconfig6",
                "--file",
                "kscreenlockerrc",
                "--group",
                "Greeter",
                "--key",
                "wallpaper",
                "--delete",
            ],
            capture_output=True,
            text=True,
            check=False,
        )

        # Cleanup [Daemon] section duplicates (lockonresume vs LockOnResume, etc)
        # These cause python configparser to fail and might confuse KDE
        for key in ["lockonresume", "timeout", "autolock"]:
            subprocess.run(
                [
                    "kwriteconfig6",
                    "--file",
                    "kscreenlockerrc",
                    "--group",
                    "Daemon",
                    "--key",
                    key,
                    "--delete",
                ],
                capture_output=True,
                text=True,
                check=False,
            )

        print(f"[DEBUG] Redundant entries cleaned up")
    except Exception as e:
        print(f"[WARNING] Could not clean up redundant entries: {e}")


def _reload_screensaver_config():
    """Reload the screen saver configuration via DBus."""
    print(f"[DEBUG] Attempting to reload screen saver configuration...")

    # Try multiple services and methods to ensure reliability
    services = ["org.freedesktop.ScreenSaver", "org.kde.screensaver"]
    methods = ["configure", "org.kde.screensaver.configure"]

    success = False

    for service in services:
        for method in methods:
            try:
                cmd = ["qdbus6", service, "/ScreenSaver", method]
                # print(f"[DEBUG] Calling: {' '.join(cmd)}")
                result = subprocess.run(
                    cmd, check=False, capture_output=True, text=True
                )
                if result.returncode == 0:
                    print(f"[DEBUG] Successfully called {method} on {service}")
                    success = True
                # else:
                #    print(f"[DEBUG] Failed call {method} on {service}: {result.stderr.strip()}")
            except Exception as e:
                print(f"[WARNING] Error calling {method} on {service}: {e}")

    if not success:
        print(f"[WARNING] All attempts to reload screen saver configuration failed")
        print(f"[DEBUG] You may need to log out and back in for changes to take effect")
    else:
        print(f"[DEBUG] Screen saver configuration reload signal sent")


def _execute_dynamic_cycle(plugin_manager, desktop, lockscreen):
    """Execute a single cycle of dynamic plugin wallpaper setting."""
    try:
        # 1. Reload Config
        config = load_config_file()

        # 2. Collect Sources
        sources = collect_plugin_sources(config, plugin_manager)

        # 3. Pick and Set
        if sources:
            success = False
            if desktop and lockscreen:
                success = set_dual_wallpaper_from_sources(sources)
            elif lockscreen:
                img = get_random_image_from_sources(sources)
                success = set_lockscreen_wallpaper(img)
            else:
                success = set_random_wallpaper_from_sources(sources)

            if success:
                print(f"[DEBUG] Wallpaper set successfully")
            else:
                print(f"[ERROR] Failed to set wallpaper")
        else:
            print(f"[DEBUG] No plugins enabled or sources found.")

        # Return latest config for wait interval
        return config

    except Exception as e:
        print(f"[ERROR] Error in cycle: {e}")
        import traceback

        traceback.print_exc()
        return {}


class ConfigWatcher(FileSystemEventHandler):
    """Watch for configuration file changes."""

    def __init__(self, config_path, change_event):
        self.config_path = Path(config_path).resolve()
        self.change_event = change_event

    def _process_event(self, event):
        # We try to match by path.
        # Note: event.src_path is absolute if we watch a dir with observer,
        # but safely check resolve.
        try:
            event_path = Path(event.src_path).resolve()

            # Check for direct match
            if event_path == self.config_path:
                print(
                    f"[DEBUG] Config change detected (Event: {event.event_type}), triggering update..."
                )
                self.change_event.set()
                return

            # Handle atomic moves (moved_to)
            if event.event_type == "moved":
                dest_path = Path(event.dest_path).resolve()
                if dest_path == self.config_path:
                    print(
                        f"[DEBUG] Config move detected (Event: {event.event_type}), triggering update..."
                    )
                    self.change_event.set()
        except Exception as e:
            print(f"[DEBUG] Error processing event: {e}")

    def on_modified(self, event):
        self._process_event(event)

    def on_created(self, event):
        self._process_event(event)

    def on_moved(self, event):
        self._process_event(event)


def _wait_for_next_cycle(config, default_wait, change_event=None):
    """Wait for the next cycle, respecting interrupt signals and config changes."""
    sleep_duration = default_wait
    if config and config.get("default_wait"):
        try:
            sleep_duration = int(config["default_wait"])
        except (ValueError, TypeError):
            pass

    # If we have a change event, wait on it
    if change_event:
        # We still loop to check shutdown_requested, but use event.wait with timeout
        # However, event.wait returns true if event set, false if timeout.
        # If event set, we return early (triggering next cycle immediately).

        start_time = time.time()
        while time.time() - start_time < sleep_duration:
            if shutdown_requested:
                break

            # Wait for 1 second or until event is set
            # We chunk it to 1s to check shutdown_requested efficiently
            # OR we could just wait(remaining) if shutdown_requested was handled via another signal event,
            # but shutdown_requested is a simple bool flag here.

            # Better: wait for the event with a 1s timeout
            if change_event.wait(timeout=1.0):
                print(f"[DEBUG] Config change detected, interrupting wait cycle")
                change_event.clear()  # Reset for next time
                return  # Exit wait immediately

    else:
        # Legacy fallback
        for _ in range(sleep_duration):
            if shutdown_requested:
                break
            time.sleep(1)






if __name__ == "__main__":
    # Windows: No service support (services can't change wallpapers due to Session 0 isolation)
    # Linux: Service support handled by systemd
    main()
