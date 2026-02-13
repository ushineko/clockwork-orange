# clockwork-orange

A Python application for managing wallpapers and lock screen backgrounds, supporting **Linux (KDE Plasma 6)**, **Windows 10/11**, and **macOS 13+**. Supports setting wallpapers from URLs, local files, or random selection from directories, with per-monitor wallpaper support on all platforms.

> **Platform Notes:**
> *   **Linux**: Requires KDE Plasma 6 (`qdbus6`, `kwriteconfig6`).
> *   **Windows**: Requires Windows 10/11. Use the standalone `.exe` or Python 3.10+.
> *   **macOS**: Requires macOS 13 (Ventura) or later. Use the `.app` bundle or Python 3.10+.
>
> **Downloads** — Get platform builds from [GitHub Releases](https://github.com/ushineko/clockwork-orange/releases):
> *   **Windows**: `clockwork-orange.exe` — Not code-signed; Windows SmartScreen may warn. Click "More info" → "Run anyway".
> *   **macOS**: `Clockwork-Orange-macOS.zip` — Not code-signed or notarized. Right-click → "Open" on first launch to bypass Gatekeeper.
> *   **Linux**: `.pkg.tar.zst` (Arch) or `.deb` (Debian/Ubuntu).

*Clockwork Orange: Our Choice Is Your Imperative (tm)*

## Table of Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Arch Linux Installation](#arch-linux-installation)
- [Quick Start](#quick-start)
  - [Basic Usage](#basic-usage)
  - [Configuration File](#configuration-file)
- [Command Line Options](#command-line-options)
- [Configuration File](#configuration-file-1)
  - [Configuration Options](#configuration-options)
- [Dynamic Multi-Plugin & Dual Mode](#dynamic-multi-plugin--dual-mode)
- [How It Works](#how-it-works)
  - [Desktop Wallpapers](#desktop-wallpapers)
  - [Lock Screen Wallpapers](#lock-screen-wallpapers)
  - [Image Detection](#image-detection)
- [Plugin System](#plugin-system)
  - [Local Plugin](#local-plugin)
  - [Wallhaven Plugin](#wallhaven-plugin)
  - [Google Images Plugin](#google-images-plugin)
  - [Stable Diffusion Plugin](#ai-wallpapers-stable-diffusion)
  - [Review & Blacklist System](#review--blacklist-system)
- [AI Wallpapers (Stable Diffusion)](#ai-wallpapers-stable-diffusion)
  - [Requirements](#requirements-1)
  - [Setup](#setup)
  - [Configuration](#configuration)
- [Graphical User Interface](#graphical-user-interface)
  - [Starting the GUI](#starting-the-gui)
  - [Desktop Entry Installation](#desktop-entry-installation)
- [Running as a Background Service](#running-as-a-background-service)
  - [Option 1: Systemd User Service (Recommended)](#option-1-systemd-user-service-recommended)
  - [Option 2: Simple Background Process](#option-2-simple-background-process)
  - [Option 3: Desktop Autostart](#option-3-desktop-autostart)
- [Service Files](#service-files)
  - [Service Configuration Notes](#service-configuration-notes)
- [Troubleshooting](#troubleshooting)
- [Examples](#examples)
- [Notes](#notes)
- [License](#license)

## Features

- **Cross-Platform**: Linux (KDE Plasma 6), Windows 10/11, and macOS 13+
- **Multi-Monitor Support**: Automatically detects connected monitors and sets a unique random wallpaper for each one (all platforms)
- **Dynamic Multi-Plugin Mode**: Concurrently pull wallpapers from multiple enabled plugins (e.g., Google Images + Local Folder)
- **Fair Source Selection**: Intelligent randomization ensures equal representation from all enabled sources, preventing large local libraries from dominating
- **Shared Blacklist**: Centralized, hash-based blacklist system shared across all plugins
- **Dual Wallpaper Support**: Set different wallpapers for desktop and lock screen simultaneously (Linux only; dynamically pulled from different plugins)
- **Continuous Cycling**: Automatically cycle through wallpapers at specified intervals
- **Lock Screen Support**: Configure KDE Plasma 6 lock screen backgrounds (Linux only)
- **Configuration File**: YAML-based configuration for persistent settings
- **Service Mode**: Run as a background service with systemd (Linux) or GUI tray app (Windows/macOS)
- **Plugin System**: Extensible plugin architecture (includes Google Images downloader)
- **AI Wallpaper Generation**: Generate unique wallpapers locally using Stable Diffusion (optional)
- **Image Review**: Built-in GUI tool to review, mark, and ban unwanted wallpapers
- **Detailed Debugging**: Detailed logging for troubleshooting

## Requirements

### All Platforms
- **Python 3.10+**
- **PyYAML** (`pip install PyYAML`)
- **PyQt6** (for GUI: `pip install PyQt6`)
- **Pillow** (for image processing: `pip install Pillow`)

### Linux
- **KDE Plasma 6** (required — `qdbus6`, `kwriteconfig6`)

### Windows
- **Windows 10/11**
- **pywin32** (installed automatically via `requirements.txt`)

### macOS
- **macOS 13 (Ventura) or later**
- **pyobjc** (installed automatically via `requirements.txt`)

## Installation

### From Source (All Platforms)

1. **Clone the repository:**
   ```bash
   git clone https://github.com/ushineko/clockwork-orange
   cd clockwork-orange
   ```

2. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

3. **Run:**
   ```bash
   python clockwork-orange.py --gui
   ```

### macOS

**Option A: Download the `.app` bundle** from [GitHub Releases](https://github.com/ushineko/clockwork-orange/releases). Unzip and drag to Applications.

**Option B: Build from source** using Homebrew Python (required for `.app` bundles):
```bash
/opt/homebrew/bin/python3.13 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
pip install PyInstaller
bash scripts/build_macos.sh
# Result: dist/Clockwork Orange.app
```

### Windows

Download `clockwork-orange.exe` from [GitHub Releases](https://github.com/ushineko/clockwork-orange/releases), or build from source using the PowerShell build scripts in `scripts/`.

## Arch Linux Installation

### From AUR (Recommended)
This package is designed for the AUR. Once submitted, you can install it using your favorite AUR helper:
```bash
yay -S clockwork-orange-git
```

### Local Development / Manual Build
To build the package from your local repository (useful for testing changes before pushing):
```bash
./build_local.sh
sudo pacman -U clockwork-orange-git-*.pkg.tar.zst
```

### Optional: Stable Diffusion Setup
For AI-generated wallpapers, run the setup script after installation:
```bash
clockwork-orange-setup-sd
```
See [AI Wallpapers (Stable Diffusion)](#ai-wallpapers-stable-diffusion) for details.

## Quick Start

### Basic Usage

```bash
# Set desktop wallpaper from URL (default behavior)
./clockwork-orange.py

# Set desktop wallpaper from local file
./clockwork-orange.py -f /path/to/image.jpg

# Set random desktop wallpaper from directory
./clockwork-orange.py -d /path/to/wallpapers

# Set lock screen wallpaper
./clockwork-orange.py --lockscreen -f /path/to/image.jpg

# Set different wallpapers for desktop and lock screen
./clockwork-orange.py --desktop --lockscreen -d /path/to/wallpapers

# Cycle through random wallpapers every 5 minutes
./clockwork-orange.py -d /path/to/wallpapers -w 300

# Start the graphical user interface
./clockwork-orange.py --gui
```

### Configuration File

Create a configuration file for persistent settings:

```bash
# Create initial configuration
./clockwork-orange.py --desktop --lockscreen -d /path/to/your/wallpapers -w 300 --write-config

# Now you can just run without arguments
./clockwork-orange.py
```

## Command Line Options

### Target Selection
- `--desktop` - Set desktop wallpaper only
- `--lockscreen` - Set lock screen wallpaper only
- `--desktop --lockscreen` - Set both (different images)

### Source Options (mutually exclusive)
- `-u, --url URL` - Download from URL
- `-f, --file PATH` - Use local file
- `-d, --directory PATH` - Random selection from directory

### Other Options
- `-w, --wait SECONDS` - Wait interval for cycling (directory mode only)
- `--write-config` - Write configuration file and exit
- `--debug-lockscreen` - Show current lock screen configuration
- `--gui` - Start the graphical user interface

## Configuration File

The script supports a YAML configuration file at `~/.config/clockwork-orange.yml`:

```yaml
# Enable dual wallpaper mode (both desktop and lock screen with different images)
dual_wallpapers: true

# Default directory for wallpapers
default_directory: /path/to/your/wallpapers

# Default wait interval for cycling (in seconds)
default_wait: 300

# Alternative: Enable only desktop mode
# desktop: true

# Alternative: Enable only lock screen mode  
# lockscreen: true

# Alternative: Set default URL
# default_url: "https://pic.re/image"

# Alternative: Set default file
# default_file: "/path/to/specific/wallpaper.jpg"

# Plugin Configuration (Dynamic Mode)
plugins:
  # Local file source
  local:
    enabled: true
    path: "/home/user/Pictures/Wallpapers"
    recursive: true

  # Google Images downloader
  google_images:
    enabled: true
    query: 
      - term: "4k nature wallpapers"
        enabled: true
      - term: "james webb telescope"
        enabled: true
    max_files: 200
    interval: "Hourly"
```

### Configuration Options

- `dual_wallpapers: true` - Enable dual wallpaper mode
- `desktop: true` - Enable desktop wallpaper mode only
- `lockscreen: true` - Enable lock screen mode only
- `default_directory: "/path/to/wallpapers"` - Default wallpaper directory
- `default_file: "/path/to/wallpaper.jpg"` - Default wallpaper file
- `default_url: "https://example.com/image.jpg"` - Default URL
- `default_wait: 300` - Default wait interval for cycling

## Dynamic Multi-Plugin & Dual Mode

Clockwork Orange now supports a powerful **Dynamic Multi-Plugin Mode**.

If you enable multiple plugins (e.g., Google Images and Local File Source) and run the script without specifying a single source flag (like `-f` or `-d`), it will automatically enter **Dynamic Mode**:

1.  **Aggregation**: It gathers valid image sources from all currently enabled plugins.
2.  **Fair Selection**: It randomly selects a **source first**, then an image from that source. This ensures that a plugin with 10 images has the same chance of being picked as a plugin with 10,000 images.
3.  **Dual Wallpapers**: If "Dual Wallpaper" mode is active, it will pick two *distinct* images (potentially from different plugins) and set one for the Desktop and one for the Lock Screen.
4.  **Service Integration**: The background service uses this mode by default, respecting your "Wait Interval" setting from the GUI.

## How It Works

### Desktop Wallpapers (Multi-Monitor)

Wallpaper setting is handled natively on each platform:

- **Linux**: Uses `qdbus6` to communicate with KDE Plasma's D-Bus interface, setting per-desktop wallpapers.
- **Windows**: Uses `SystemParametersInfoW` with spanned wallpaper stitching via `screeninfo` and Pillow.
- **macOS**: Uses `NSWorkspace.setDesktopImageURL_forScreen_options_error_()` via pyobjc for per-screen wallpaper control, with `osascript` as a fallback.

### Lock Screen Wallpapers (Linux Only)
The script modifies the `~/.config/kscreenlockerrc` configuration file using `kwriteconfig6`. Lock screen wallpaper is not supported on Windows or macOS.

### Image Detection
The script automatically detects image files by:
- File extension (`.jpg`, `.jpeg`, `.png`, `.bmp`, `.gif`, `.tiff`, `.webp`, `.svg`)
- MIME type detection as fallback

## Plugin System

Clockwork Orange features a robust plugin system tailored for wallpaper acquisition.

### Local Plugin
The default source for local files and directories.
- **Configurable Path**: Point to any directory or specific file.
- **Recursive Search**: Optionally scan subdirectories for images.
- **Blacklist Integration**: Respects the global blacklist.

**Configuration:**
```yaml
plugins:
  local:
    enabled: true
    path: "/home/user/Wallpapers"
    recursive: true
```

### Wallhaven Plugin
Downloads wallpapers from Wallhaven.cc API.
- **Sorting**: Relevance, Random, Date Added, Views, Favorites, Toplist.
- **Filters**: Categories (General/Anime/People) and Purity (SFW/Sketchy/NSFW).
- **Resolutions**: Filter by exact resolution or minimum size.
- **API Key**: Optional (required for NSFW content).

**Configuration:**
```yaml
plugins:
  wallhaven:
    enabled: true
    query: 
      - term: "cyberpunk"
        enabled: true
      - term: "pixel art"
        enabled: true
    api_key: "YOUR_API_KEY"  # Optional
    purity_nsfw: false
    sorting: "relevance"
    resolutions: "2560x1440,3840x2160"
```

### Google Images Plugin
The built-in Google Images plugin allows you to scrape high-quality wallpapers based on search terms.
- **Smart Search Terms UI**: Easily add, remove, and toggle individual search terms via the GUI
- **Intelligent Processing**: Automatically downloads, resizes, and center-crops images to 4K (3840x2160)
- **Quality Control**: Skips low-resolution thumbnails or duplicates
- **Scheduling**: Built-in interval checks (Hourly/Daily/Weekly)
- **Retention**: Automatically cleans up old files to save space

**Usage:**
Configure via the GUI "Plugins" tab or manually in `config.yml`:
```yaml
plugins:
  google_images:
    enabled: true
    download_dir: "/home/user/Pictures/Wallpapers/Google"
    query:
      - term: "4k cyberpunk city"
        enabled: true
```

### Review & Blacklist System
The GUI includes a comprehensive **Review Mode** and **Shared Blacklist Manager**:

**Review Mode (in Plugins Tab):**
1. **Scan**: Load images from the plugin's download directory.
2. **Review**: Navigate through images using **Left/Right Arrows**.
3. **Mark/Delete**: Press **Button** or use context menu to permanently delete and blacklist an image.

**Blacklist Manager (Tab):**
- **Centralized Database**: All plugins share a single `blacklist.json` database.
- **Metadata**: Tracks file hash, deletion date, and source plugin.
- **Management**: View the list of all banned hashes and remove entries if you made a mistake.

## Graphical User Interface

Clockwork Orange includes a comprehensive Qt-based GUI for managing plugins, reviewing wallpapers, and configuring the background service.

👉 **[View the full GUI Documentation and Tour](GUI.md)** for detailed screenshots and feature explanations.

### Starting the GUI

```bash
./clockwork-orange.py --gui
```

### Desktop Entry Installation

To integrate with your system launcher (install `.desktop` file):

```bash
./install-desktop-entry.sh
```

After installation, you can launch "Clockwork Orange" from your application menu or pin it to your taskbar.

## AI Wallpapers (Stable Diffusion)
The built-in Stable Diffusion plugin allows you to generate unique, high-quality wallpapers locally on your machine.

### Requirements
- **GPU highly recommended**: NVIDIA with CUDA support provides the best performance
- **Disk space**: ~2-4GB for the virtual environment and model weights
- **RAM**: 8GB minimum, 16GB recommended

### Setup

The setup script creates an isolated virtual environment with PyTorch and the HuggingFace diffusers library. This avoids conflicts with your system Python packages.

**For Arch Linux package users (`clockwork-orange-git`):**
```bash
# Option 1: Use the convenience command
clockwork-orange-setup-sd

# Option 2: Run the script directly
/usr/lib/clockwork-orange/scripts/setup_stable_diffusion.sh
```

**For git checkout / manual installations:**
```bash
./scripts/setup_stable_diffusion.sh
```

The script will:
1. Create a virtual environment at `~/.local/share/clockwork-orange/venv-sd`
2. Install PyTorch (with CUDA support if an NVIDIA GPU is detected)
3. Install diffusers, transformers, and accelerate

After setup completes, the Stable Diffusion plugin will automatically use this environment.

### Configuration
The plugin offers extensive customization options via the GUI:

- **Prompts**: 
    - Supports **Multi-Prompt** lists. Add multiple prompts (e.g., "cyberpunk city", "serene lake", "deep space nebula").
    - The plugin will randomly select one prompt from your enabled list for each generation cycle.
- **Resolution & Upscaling**:
    - **Base Resolution**: Set generation size (e.g., 768x512). Dimensions are automatically snapped to multiples of 8.
    - **Upscale**: Automatically resizes the output to QHD (2560x1440) using high-quality resampling, ensuring crisp wallpapers without the artifacting of direct high-res generation.
- **Safety Checker**:
    - Toggle the built-in NSFW safety filter on/off. Images flagged by the filter are automatically discarded to prevent black placeholder files.
- **Model Selection**:
    - Choose from popular HuggingFace models (default: `runwayml/stable-diffusion-v1-5`).
    - Supports both public and authenticated models (requires token).

### Usage
- **Generate**: Click the "Generate" button in the Stable Diffusion tab to run a manual batch.
- **Delete Now**: Instantly delete unwanted generations from the review panel (bypasses the standard blacklist).
- **Interval**: Set the generation frequency (e.g., "Hourly") to keep your desktop fresh with new AI art automatically.
## Running as a Background Service

On **Windows** and **macOS**, the GUI must be running for wallpaper cycling. The app minimizes to the system tray (menu bar on macOS) and cycles wallpapers via an internal timer. No separate background service is needed.

On **Linux**, you can use the GUI tray mode or run a standalone background service:

### Option 1: Systemd User Service (Linux, Recommended)

1. **Setup the service:**
   ```bash
   mkdir -p ~/.config/systemd/user
   cp clockwork-orange.service ~/.config/systemd/user/
   systemctl --user daemon-reload
   systemctl --user enable clockwork-orange.service
   ```

2. **Start the service:**
   ```bash
   systemctl --user start clockwork-orange.service
   ```

3. **Management commands:**
   ```bash
   # Check status
   systemctl --user status clockwork-orange.service
   
   # View logs
   journalctl --user -u clockwork-orange.service -f
   
   # Stop service
   systemctl --user stop clockwork-orange.service
   
   # Restart service
   systemctl --user restart clockwork-orange.service
   ```

### Option 2: Simple Background Process

```bash
# Run in background with nohup
nohup ./run_clockwork_orange.sh --desktop --lockscreen -d /path/to/wallpapers -w 300 > clockwork-orange.log 2>&1 &

# Or run with screen/tmux for better process management
screen -S clockwork-orange ./run_clockwork_orange.sh --desktop --lockscreen -d /path/to/wallpapers -w 300
```

### Option 3: Desktop Autostart

Create `~/.config/autostart/clockwork-orange.desktop`:
```ini
[Desktop Entry]
Type=Application
Name=Clockwork Orange
Comment=Automatic wallpaper cycling service
Exec=/path/to/your/clockwork-orange/run_clockwork_orange.sh --desktop --lockscreen -d /path/to/wallpapers -w 300
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
```

## Service Files

The repository includes several service-related files:

- `clockwork-orange.service` - Systemd user service file
- `run_clockwork_orange.sh` - Wrapper script with proper environment setup
- `clockwork-orange.yml.example` - Example configuration file

### Service Configuration Notes

The service file uses `/usr/bin/python3` by default. If you're using a conda environment, virtual environment, or custom Python installation, you may need to update the `ExecStart` path in the service file to point to your specific Python interpreter.

**For conda/virtual environments:**
```ini
ExecStart=/path/to/your/python /path/to/clockwork-orange/clockwork-orange.py
```

**For system Python (default):**
```ini
ExecStart=/usr/bin/python3 /path/to/clockwork-orange/clockwork-orange.py
```

Make sure PyYAML is installed for your chosen Python interpreter:
```bash
# For system Python
sudo pacman -S python-yaml  # Arch Linux
sudo apt install python3-yaml  # Ubuntu/Debian

# For conda/virtual environments
pip install PyYAML
```

## Troubleshooting

### Check Service Status
```bash
systemctl --user status clockwork-orange.service
```

### View Service Logs
```bash
journalctl --user -u clockwork-orange.service -f
```

### Test Manually
```bash
# Test with your specific Python interpreter
/path/to/your/python clockwork-orange.py --desktop --lockscreen -d /path/to/wallpapers -w 30
```

### Check Environment Variables
```bash
echo $DISPLAY
echo $XDG_RUNTIME_DIR
echo $DBUS_SESSION_BUS_ADDRESS
```

### Debug Lock Screen Configuration
```bash
./clockwork-orange.py --debug-lockscreen
```

## Examples

### Daily Wallpaper Cycling
```bash
# Set up configuration for daily cycling
./clockwork-orange.py --desktop --lockscreen -d /home/user/Pictures/Wallpapers -w 86400 --write-config

# Run as service
systemctl --user start clockwork-orange.service
```

### Hourly Desktop Wallpaper Changes
```bash
# Desktop only, every hour
./clockwork-orange.py -d /home/user/DesktopWallpapers -w 3600
```

### Lock Screen Only
```bash
# Set lock screen from specific file
./clockwork-orange.py --lockscreen -f /home/user/lockscreen.jpg
```

## Notes

- The service automatically restarts if it crashes
- Make sure your wallpaper directory exists and contains image files
- The service runs with a 10-second restart delay if it fails
- Logs are available through journalctl for systemd services
- Command line arguments override configuration file settings
- The script ensures different images for desktop and lock screen in dual mode

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
