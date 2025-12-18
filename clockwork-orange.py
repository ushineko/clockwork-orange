#!/usr/bin/env python
import requests
import subprocess
from pathlib import Path
import tempfile
import sys
import argparse
import random
import mimetypes
import time
import signal
import configparser
import os
import yaml
from plugin_manager import PluginManager

# GUI imports (optional)
try:
    from gui.main_window import main as gui_main
    GUI_AVAILABLE = True
except ImportError:
    GUI_AVAILABLE = False

# Supported image file extensions
IMAGE_EXTENSIONS = {'.jpg', '.jpeg', '.png', '.bmp', '.gif', '.tiff', '.webp', '.svg'}

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
    if mime_type and mime_type.startswith('image/'):
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

    script = """
    desktops().forEach(d => {
        d.currentConfigGroup = Array("Wallpaper",
                                     "org.kde.image",
                                     "General");
        d.writeConfig("Image", "file://FILEPATH");
        d.reloadConfig();
    });
    """.replace("FILEPATH", str(p))

    print(f"[DEBUG] Generated KDE script with file path: {p}")

    cmd = [
        "qdbus6",
        "org.kde.plasmashell",
        "/PlasmaShell",
        "org.kde.PlasmaShell.evaluateScript",
        script,
    ]

    print(f"[DEBUG] Executing qdbus6 command: {' '.join(cmd[:4])} [script content]")
    
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        print(f"[DEBUG] qdbus6 command executed successfully")
        if result.stdout:
            print(f"[DEBUG] qdbus6 stdout: {result.stdout}")
        if result.stderr:
            print(f"[DEBUG] qdbus6 stderr: {result.stderr}")
        return True
    except subprocess.CalledProcessError as e:
        print(f"[ERROR] qdbus6 command failed with return code {e.returncode}")
        print(f"[ERROR] stdout: {e.stdout}")
        print(f"[ERROR] stderr: {e.stderr}")
        return False
    except FileNotFoundError:
        print(f"[ERROR] qdbus6 command not found. Is qdbus6 installed?")
        return False

def download_and_set_wallpaper(url: str):
    """Download image from URL and set as wallpaper."""
    print(f"[DEBUG] Downloading image from: {url}")
    
    try:
        response = requests.get(url, timeout=30)
        print(f"[DEBUG] HTTP response status: {response.status_code}")
        print(f"[DEBUG] Content length: {len(response.content)} bytes")
        print(f"[DEBUG] Content type: {response.headers.get('content-type', 'unknown')}")
        
        if response.status_code != 200:
            print(f"[ERROR] Failed to download image. HTTP status: {response.status_code}")
            return False
            
    except requests.exceptions.RequestException as e:
        print(f"[ERROR] Network error while downloading image: {e}")
        return False
    
    with tempfile.NamedTemporaryFile(delete=False, suffix='.jpg') as f:
        p = Path(f.name).resolve()
        print(f"[DEBUG] Created temporary file: {p}")
        
        try:
            p.write_bytes(response.content)
            print(f"[DEBUG] Successfully wrote {len(response.content)} bytes to temporary file")
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
    
    # Resolve all sources
    valid_sources = []
    for s in sources:
        path = Path(s).resolve()
        if path.exists():
            valid_sources.append(path)
        else:
            print(f"[WARN] Source does not exist: {path}")

    if not valid_sources:
        raise ValueError("No valid sources found")

    # Shuffle to pick a random source order
    # (We iterate until we find one that has images)
    random.shuffle(valid_sources)
    
    for source in valid_sources:
        # print(f"[DEBUG] Checking source: {source}")
        candidates = []
        
        if source.is_file():
             if is_image_file(source):
                 candidates.append(source)
        elif source.is_dir():
            try:
                # Scan directory
                candidates = [f for f in source.iterdir() if is_image_file(f)]
            except Exception as e:
                print(f"[ERROR] Failed to scan {source}: {e}")
                
        if candidates:
            selected = random.choice(candidates)
            print(f"[DEBUG] Selected image from source {source}: {selected}")
            return selected
            
    raise ValueError(f"No image files found in any of the {len(valid_sources)} provided sources")

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
    """Set lock screen wallpaper using kwriteconfig6 (KDE 6)."""
    print(f"[DEBUG] set_lockscreen_wallpaper called with: {image_path}")
    
    image_path = image_path.resolve()
    print(f"[DEBUG] Resolved path: {image_path}")
    print(f"[DEBUG] Path exists: {image_path.exists()}")
    print(f"[DEBUG] Is file: {image_path.is_file()}")
    
    if not image_path.exists():
        print(f"[ERROR] Image file does not exist: {image_path}")
        return False
    
    if not is_image_file(image_path):
        print(f"[ERROR] File is not a supported image format: {image_path}")
        return False
    
    # Use kwriteconfig6 to set the lock screen wallpaper (recommended approach for KDE 6)
    wallpaper_path = f"file://{image_path}"
    print(f"[DEBUG] Setting lock screen wallpaper using kwriteconfig6: {wallpaper_path}")
    
    try:
        # Use kwriteconfig6 with the correct syntax for KDE 6
        result = subprocess.run([
            'kwriteconfig6', '--file', 'kscreenlockerrc',
            '--group', 'Greeter',
            '--group', 'Wallpaper', 
            '--group', 'org.kde.image',
            '--group', 'General',
            '--key', 'Image',  # Use uppercase Image as per documentation
            wallpaper_path
        ], capture_output=True, text=True, check=True)
        
        print(f"[DEBUG] kwriteconfig6 executed successfully")
        if result.stdout:
            print(f"[DEBUG] kwriteconfig6 stdout: {result.stdout}")
        if result.stderr:
            print(f"[DEBUG] kwriteconfig6 stderr: {result.stderr}")
        
        # Clean up redundant entries by removing the lowercase 'image' key
        try:
            print(f"[DEBUG] Cleaning up redundant configuration entries...")
            subprocess.run([
                'kwriteconfig6', '--file', 'kscreenlockerrc',
                '--group', 'Greeter',
                '--group', 'Wallpaper', 
                '--group', 'org.kde.image',
                '--group', 'General',
                '--key', 'image',  # Remove lowercase image key
                '--delete'
            ], capture_output=True, text=True, check=False)
            
            # Also remove the main Greeter wallpaper entry to avoid conflicts
            subprocess.run([
                'kwriteconfig6', '--file', 'kscreenlockerrc',
                '--group', 'Greeter',
                '--key', 'wallpaper',  # Remove main wallpaper entry
                '--delete'
            ], capture_output=True, text=True, check=False)
            
            print(f"[DEBUG] Redundant entries cleaned up")
        except Exception as e:
            print(f"[WARNING] Could not clean up redundant entries: {e}")
        
        # Try to reload the screen saver configuration
        try:
            print(f"[DEBUG] Attempting to reload screen saver configuration...")
            subprocess.run(['qdbus6', 'org.freedesktop.ScreenSaver', '/ScreenSaver', 'configure'], 
                         check=False, capture_output=True)
            print(f"[DEBUG] Screen saver configuration reloaded")
        except Exception as e:
            print(f"[WARNING] Could not reload screen saver configuration: {e}")
            print(f"[DEBUG] You may need to log out and back in for changes to take effect")
        
        # Show debug information about the current configuration
        print(f"[DEBUG] Current lock screen configuration after change:")
        debug_lockscreen_config()
        
        return True
        
    except subprocess.CalledProcessError as e:
        print(f"[ERROR] kwriteconfig6 failed with return code {e.returncode}")
        print(f"[ERROR] stdout: {e.stdout}")
        print(f"[ERROR] stderr: {e.stderr}")
        return False
    except FileNotFoundError:
        print(f"[ERROR] kwriteconfig6 command not found. Is KDE 6 installed?")
        return False
    except Exception as e:
        print(f"[ERROR] Unexpected error: {e}")
        return False

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
    print(f"[DEBUG] Starting continuous lock screen wallpaper cycling from directory: {directory_path}")
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
                print(f"[DEBUG] Lock screen wallpaper set successfully (cycle #{cycle_count})")
            else:
                print(f"[ERROR] Failed to set lock screen wallpaper (cycle #{cycle_count})")
        except Exception as e:
            print(f"[ERROR] Unexpected error during lock screen cycle #{cycle_count}: {e}")
        
        if not shutdown_requested:
            print(f"[DEBUG] Waiting {wait_seconds} seconds before next lock screen cycle...")
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
        raise ValueError(f"Need at least 2 image files in directory, found {len(image_files)}: {directory}")
    
    # Select two different images
    selected_files = random.sample(image_files, 2)
    print(f"[DEBUG] Selected two different images:")
    print(f"[DEBUG]   - Image 1: {selected_files[0]}")
    print(f"[DEBUG]   - Image 2: {selected_files[1]}")
    
    return selected_files[0], selected_files[1]

def set_dual_wallpapers_from_directory(directory_path: Path):
    """Set both desktop and lock screen wallpapers from different random images in directory."""
    try:
        desktop_image, lockscreen_image = get_two_different_images_from_directory(directory_path)
        
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
    print(f"[DEBUG] Starting continuous dual wallpaper cycling from directory: {directory_path}")
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
                print(f"[DEBUG] Both wallpapers set successfully (cycle #{cycle_count})")
            else:
                print(f"[ERROR] Failed to set one or both wallpapers (cycle #{cycle_count})")
        except Exception as e:
            print(f"[ERROR] Unexpected error during dual wallpaper cycle #{cycle_count}: {e}")
        
        if not shutdown_requested:
            print(f"[DEBUG] Waiting {wait_seconds} seconds before next dual wallpaper cycle...")
            # Sleep in small increments to be responsive to interrupts
            for _ in range(wait_seconds):
                if shutdown_requested:
                    break
                time.sleep(1)
    
    print(f"[DEBUG] Dual wallpaper cycling stopped after {cycle_count} cycles")

def load_config_file():
    """Load configuration from YAML file."""
    config_path = Path.home() / ".config" / "clockwork-orange.yml"
    print(f"[DEBUG] Looking for configuration file: {config_path}")
    
    if not config_path.exists():
        print(f"[DEBUG] Configuration file not found, using defaults")
        return {}
    
    try:
        with open(config_path, 'r') as f:
            config = yaml.safe_load(f)
        print(f"[DEBUG] Loaded configuration from {config_path}")
        print(f"[DEBUG] Configuration: {config}")
        return config or {}
    except yaml.YAMLError as e:
        print(f"[ERROR] Failed to parse YAML configuration file: {e}")
        return {}
    except Exception as e:
        print(f"[ERROR] Failed to read configuration file: {e}")
        return {}

def merge_config_with_args(config, args):
    """Merge configuration file options with command line arguments."""
    print(f"[DEBUG] Merging configuration with command line arguments")
    
    # Create a copy of args to avoid modifying the original
    merged_args = argparse.Namespace(**vars(args))
    
    # Apply configuration defaults if not specified on command line
    if hasattr(config, 'get'):
        # Desktop wallpaper settings
        if not merged_args.desktop and not merged_args.lockscreen and config.get('desktop', False):
            merged_args.desktop = True
            print(f"[DEBUG] Enabled desktop mode from config")
        
        # Lock screen settings
        if not merged_args.lockscreen and not merged_args.desktop and config.get('lockscreen', False):
            merged_args.lockscreen = True
            print(f"[DEBUG] Enabled lock screen mode from config")
        
        # Dual wallpaper settings
        if config.get('dual_wallpapers', False):
            merged_args.desktop = True
            merged_args.lockscreen = True
            print(f"[DEBUG] Enabled dual wallpaper mode from config")
        
        # Default wait interval (only if wait not specified)
        if not merged_args.wait:
            if config.get('default_wait'):
                merged_args.wait = config['default_wait']
                print(f"[DEBUG] Set default wait interval from config: {merged_args.wait}")
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
    
    # Create config directory if it doesn't exist
    config_path.parent.mkdir(parents=True, exist_ok=True)
    
    # Build configuration dictionary based on current arguments
    config = {}
    
    # Determine wallpaper mode
    if args.desktop and args.lockscreen:
        config['dual_wallpapers'] = True
    elif args.desktop:
        config['desktop'] = True
    elif args.lockscreen:
        config['lockscreen'] = True
    
    # Set default source
    if args.url:
        config['default_url'] = args.url
    elif args.file:
        config['default_file'] = str(args.file)
    elif args.directory:
        config['default_directory'] = str(args.directory)
    
    # Set wait interval if specified
    if args.wait:
        config['default_wait'] = args.wait
    
    # Write the configuration file
    try:
        with open(config_path, 'w') as f:
            yaml.dump(config, f, default_flow_style=False, sort_keys=True)
        
        print(f"[DEBUG] Successfully wrote configuration file to {config_path}")
        print(f"[DEBUG] Configuration written:")
        for key, value in config.items():
            print(f"[DEBUG]   {key}: {value}")
        
        return True
        
    except Exception as e:
        print(f"[ERROR] Failed to write configuration file: {e}")
        return False

def debug_lockscreen_config():
    """Debug function to show current lock screen configuration."""
    config_path = Path.home() / ".config" / "kscreenlockerrc"
    print(f"[DEBUG] Current lock screen configuration file: {config_path}")
    
    if not config_path.exists():
        print(f"[DEBUG] Configuration file does not exist")
        return
    
    try:
        config = configparser.ConfigParser()
        config.read(config_path)
        
        print(f"[DEBUG] Configuration sections found:")
        for section in config.sections():
            print(f"[DEBUG]   - {section}")
            for key, value in config.items(section):
                print(f"[DEBUG]     {key} = {value}")
        
        # Check for the specific wallpaper setting
        wallpaper_section = 'Greeter][Wallpaper][org.kde.image][General'
        if config.has_section(wallpaper_section):
            if config.has_option(wallpaper_section, 'Image'):
                current_wallpaper = config.get(wallpaper_section, 'Image')
                print(f"[DEBUG] Current lock screen wallpaper (Image): {current_wallpaper}")
            elif config.has_option(wallpaper_section, 'image'):
                current_wallpaper = config.get(wallpaper_section, 'image')
                print(f"[DEBUG] Current lock screen wallpaper (image): {current_wallpaper}")
            else:
                print(f"[DEBUG] No Image/image key found in {wallpaper_section}")
        else:
            print(f"[DEBUG] Wallpaper section {wallpaper_section} not found")
        
        if config.has_section('Greeter'):
            if config.has_option('Greeter', 'wallpaper'):
                main_wallpaper = config.get('Greeter', 'wallpaper')
                print(f"[DEBUG] Main Greeter wallpaper: {main_wallpaper}")
            else:
                print(f"[DEBUG] No wallpaper key found in Greeter section")
            
    except Exception as e:
        print(f"[ERROR] Failed to read configuration file: {e}")

def collect_plugin_sources(config, plugin_manager):
    """Collect source paths from all enabled plugins."""
    sources = []
    plugins_config = config.get('plugins', {})
    
    for name, plugin_cfg in plugins_config.items():
        if plugin_cfg.get('enabled', False):
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

def cycle_dynamic_plugins(plugin_manager: PluginManager, wait_seconds: int, desktop: bool = True, lockscreen: bool = False):
    """Continuously cycle wallpapers using enabled plugins, reloading config each time."""
    print(f"[DEBUG] Starting dynamic plugin cycling")
    print(f"[DEBUG] Wait interval: {wait_seconds} seconds")
    print(f"[DEBUG] Desktop: {desktop}, Lockscreen: {lockscreen}")
    print(f"[DEBUG] Press Ctrl+C to stop")
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    cycle_count = 0
    
    while not shutdown_requested:
        cycle_count += 1
        print(f"[DEBUG] Cycle #{cycle_count}")
        
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
                
        except Exception as e:
             print(f"[ERROR] Error in cycle: {e}")
             import traceback
             traceback.print_exc()
             
        if not shutdown_requested:
            # Sleep
            # Prefer dynamic config wait if available, otherwise use initial argument
            sleep_duration = wait_seconds
            if 'config' in locals() and config.get('default_wait'):
                try:
                    sleep_duration = int(config['default_wait'])
                except (ValueError, TypeError):
                    pass
            
            # Print update if it changed significantly? No, simpler is better.
            
            for _ in range(sleep_duration):
                if shutdown_requested: break
                time.sleep(1)
    print(f"[DEBUG] Dynamic cycling stopped")
    
def clean_config(config):
    """Clean up invalid plugins from configuration."""
    if not config.get('plugins'):
        return
        
    plugin_manager = PluginManager()
    available = plugin_manager.get_available_plugins()
    
    plugins = config['plugins']
    to_remove = []
    
    for name in plugins:
        if name not in available:
            to_remove.append(name)
            
    if to_remove:
        print(f"[DEBUG] Cleaning up invalid plugins from config: {', '.join(to_remove)}")
        for name in to_remove:
            del config['plugins'][name]
            
        # Write back changes
        config_path = Path.home() / ".config" / "clockwork-orange.yml"
        try:
            # Create a simplified args object for write_config_file equivalent
            # But simpler here since we just dump the dict
            with open(config_path, 'w') as f:
                yaml.dump(config, f, default_flow_style=False, sort_keys=True)
            print(f"[DEBUG] Configuration cleaned and saved")
        except Exception as e:
            print(f"[ERROR] Failed to save cleaned configuration: {e}")

def main():
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
        """
    )
    
    # Target selection
    parser.add_argument('--lockscreen', action='store_true',
                       help='Set lock screen wallpaper instead of desktop wallpaper')
    parser.add_argument('--desktop', action='store_true',
                       help='Set desktop wallpaper (can be combined with operations)')
    
    # Plugin Support
    plugin_manager = PluginManager()
    available_plugins = plugin_manager.get_available_plugins()
    
    group = parser.add_mutually_exclusive_group()
    group.add_argument('-u', '--url', 
                      help='Download image from URL and set as wallpaper (Legacy)')
    group.add_argument('-f', '--file', type=Path,
                      help='Set wallpaper from local file (Legacy)')
    group.add_argument('-d', '--directory', type=Path,
                      help='Set random wallpaper from directory (Legacy)')
    group.add_argument('--plugin', choices=available_plugins,
                      help=f'Use a specific plugin source. Available: {", ".join(available_plugins)}')
    
    parser.add_argument('--plugin-config', type=str,
                       help='JSON configuration string for the plugin (overrides config file)')
    
    parser.add_argument('-w', '--wait', type=int, metavar='SECONDS',
                       help='Wait specified seconds between wallpaper changes (only works with -d/--directory)')
    
    parser.add_argument('--debug-lockscreen', action='store_true',
                       help='Show current lock screen configuration for debugging')
    
    parser.add_argument('--write-config', action='store_true',
                       help='Write configuration file based on current command line options and exit')
    
    parser.add_argument('--gui', action='store_true',
                       help='Start the graphical user interface')
                       
    parser.add_argument('--service', action='store_true',
                       help='Run in background service mode (sets default wait to 900s if unspecified)')
    
    args = parser.parse_args()
    
    # Load configuration file and merge with command line arguments
    config = load_config_file()
    
    # Auto-clean configuration
    clean_config(config)
    
    args = merge_config_with_args(config, args)
    
    args = merge_config_with_args(config, args)
    
    # Handle GUI option early to avoid validation errors from config defaults
    if args.gui:
        if not GUI_AVAILABLE:
            print("[ERROR] GUI not available. Please install PyQt6: pip install PyQt6")
            sys.exit(1)
        print("[DEBUG] Starting GUI...")
        sys.exit(gui_main())

    # Check for implicit multi-plugin mode capability
    has_enabled_plugins = False
    if config.get('plugins'):
         for p_cfg in config['plugins'].values():
             if p_cfg.get('enabled'):
                 has_enabled_plugins = True
                 break

    # Validate arguments
    # We allow running without specific args IF we have enabled plugins (Multi-Plugin Mode)
    if not args.file and not args.directory and not args.plugin and not has_enabled_plugins:
        parser.error("Operation requires either --file, --directory, --plugin, or enabled plugins in config")
    
    if args.wait is not None and args.wait <= 0:
        parser.error("--wait must be a positive integer")
    
    # Validate URL with lockscreen
    if args.url and args.lockscreen:
        parser.error("--url cannot be used with --lockscreen (lock screen requires local files)")
    
    # Validate dual wallpaper mode
    if args.desktop and args.lockscreen:
        if args.url:
            parser.error("--url cannot be used with dual wallpaper mode (lock screen requires local files)")
        if not args.file and not args.directory and not args.plugin and not has_enabled_plugins:
            parser.error("Dual wallpaper mode requires either --file, --directory, --plugin, or enabled plugins")
    
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
    
    # Determine target(s)
    if args.desktop and args.lockscreen:
        target = "both desktop and lock screen"
    elif args.lockscreen:
        target = "lock screen"
    elif args.desktop:
        target = "desktop"
    else:
        target = "wallpaper"  # Default behavior

    # Resolve plugin if specified
    if args.plugin:
        print(f"[DEBUG] Plugin mode: {args.plugin}")
        # Determine config for plugin
        plugin_config = {}
        
        # Check for config in file first
        if config.get('plugins', {}).get(args.plugin):
            plugin_config = config['plugins'][args.plugin]
        
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
    
    print(f"[DEBUG] Starting {target} set process")
    
    success = False
    
    if args.desktop and args.lockscreen:
        # Dual wallpaper operations (both desktop and lock screen)
        if args.file:
            print(f"[ERROR] Dual wallpaper mode with single file not supported (need different images)")
            print(f"[ERROR] Use --desktop --lockscreen -d /path/to/directory instead")
            sys.exit(1)
        elif args.directory:
            print(f"[DEBUG] Dual wallpaper directory mode: {args.directory}")
            if args.wait is not None:
                # Continuous cycling mode for dual wallpapers
                print(f"[DEBUG] Dual wallpaper continuous mode with {args.wait} second intervals")
                cycle_dual_wallpapers_from_directory(args.directory, args.wait)
                success = True  # If we reach here, cycling completed successfully
            else:
                # Single dual wallpaper mode
                success = set_dual_wallpapers_from_directory(args.directory)
        elif not args.plugin:
            # Implicit Dynamic Mode (Configured Plugins)
            if args.wait:
                 cycle_dynamic_plugins(plugin_manager, args.wait, desktop=True, lockscreen=True)
                 success = True
            else:
                 sources = collect_plugin_sources(config, plugin_manager)
                 if sources:
                     success = set_dual_wallpaper_from_sources(sources)
                 else:
                     print("[ERROR] No enabled plugins found for dual wallpaper mode")
                     success = False
    elif args.lockscreen:
        # Lock screen operations only
        if args.file:
            print(f"[DEBUG] Lock screen file mode: {args.file}")
            success = set_lockscreen_wallpaper(args.file)
        elif args.directory:
            print(f"[DEBUG] Lock screen directory mode: {args.directory}")
            if args.wait is not None:
                # Continuous cycling mode for lock screen
                print(f"[DEBUG] Lock screen continuous mode with {args.wait} second intervals")
                cycle_lockscreen_wallpapers_from_directory(args.directory, args.wait)
                success = True  # If we reach here, cycling completed successfully
            else:
                # Single random lock screen wallpaper mode
                success = set_lockscreen_random_from_directory(args.directory)
        elif not args.plugin:
             # Implicit Dynamic Mode
             if args.wait:
                 cycle_dynamic_plugins(plugin_manager, args.wait, desktop=False, lockscreen=True)
                 success = True
             else:
                 sources = collect_plugin_sources(config, plugin_manager)
                 if sources:
                     img = get_random_image_from_sources(sources)
                     success = set_lockscreen_wallpaper(img)
                 else:
                     print("[ERROR] No enabled plugins found for lock screen mode")
                     success = False
        else:
            print("[ERROR] Lock screen mode requires either --file or --directory")
            sys.exit(1)
    elif args.desktop:
        # Desktop wallpaper operations only
        if args.url:
            print(f"[DEBUG] Desktop URL mode: {args.url}")
            success = download_and_set_wallpaper(args.url)
        elif args.file:
            print(f"[DEBUG] Desktop file mode: {args.file}")
            success = set_local_wallpaper(args.file)
        elif args.directory:
            print(f"[DEBUG] Desktop directory mode: {args.directory}")
            if args.wait is not None:
                # Continuous cycling mode for desktop
                print(f"[DEBUG] Desktop continuous mode with {args.wait} second intervals")
                cycle_wallpapers_from_directory(args.directory, args.wait)
                success = True  # If we reach here, cycling completed successfully
            else:
                # Single random desktop wallpaper mode
                success = set_random_wallpaper_from_directory(args.directory)
        elif not args.plugin:
             # Implicit Dynamic Mode
             if args.wait:
                 cycle_dynamic_plugins(plugin_manager, args.wait, desktop=True, lockscreen=False)
                 success = True
             else:
                 sources = collect_plugin_sources(config, plugin_manager)
                 if sources:
                     success = set_random_wallpaper_from_sources(sources)
                 else:
                     print("[ERROR] No enabled plugins found for desktop mode")
                     success = False
        else:
            print("[ERROR] Desktop mode requires either --url, --file, --directory or enabled plugins")
            sys.exit(1)
    else:
        # Regular wallpaper operations (default behavior)
        if args.url:
            print(f"[DEBUG] URL mode: {args.url}")
            success = download_and_set_wallpaper(args.url)
        elif args.file:
            print(f"[DEBUG] File mode: {args.file}")
            success = set_local_wallpaper(args.file)
        elif args.directory:
            print(f"[DEBUG] Directory mode: {args.directory}")
            if args.wait is not None:
                # Continuous cycling mode
                print(f"[DEBUG] Continuous mode with {args.wait} second intervals")
                cycle_wallpapers_from_directory(args.directory, args.wait)
                success = True  # If we reach here, cycling completed successfully
            else:
                # Single random wallpaper mode
                success = set_random_wallpaper_from_directory(args.directory)
        else:
             # Check for enabled plugins (Dynamic Multi-Plugin Mode)
             initial_sources = collect_plugin_sources(config, plugin_manager)
             if initial_sources:
                 print(f"[DEBUG] Multi-plugin mode active with {len(initial_sources)} sources")
                 if args.wait:
                     cycle_dynamic_plugins(plugin_manager, args.wait)
                     success = True
                 else:
                     success = set_random_wallpaper_from_sources(initial_sources)
             else:
                 # Fallback if no source specified
                 print("[ERROR] No source specified and no plugins enabled. Use --plugin, --file, --directory, or --url.")
                 parser.print_help()
                 sys.exit(1)
    
    if success:
        print(f"[DEBUG] {target.capitalize()} set successfully")
    else:
        print(f"[ERROR] Failed to set {target}")
        sys.exit(1)
    
    print("[DEBUG] Process completed")


if __name__ == "__main__":
    main()