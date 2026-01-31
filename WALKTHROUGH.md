# Clockwork Orange - Windows Support Walkthrough

This document outlines how to build, run, and manage **Clockwork Orange** on Windows.

> **Status**: Beta — tested on multiple Windows 10/11 installations. Feedback welcome!

## Quick Start (Recommended)

1. **Download** `clockwork-orange.exe` from [GitHub Releases](https://github.com/ushineko/clockwork-orange/releases)
2. **Run** the executable — double-click to launch the GUI
3. **Configure** your plugins and settings, then minimize to tray

### Windows SmartScreen Warning

The executable is **not code-signed**, so Windows SmartScreen will show a warning on first run:

> "Windows protected your PC — Microsoft Defender SmartScreen prevented an unrecognized app from starting."

This is expected. To proceed:
1. Click **"More info"**
2. Click **"Run anyway"**

This only happens once per download.

---

## 1. Building the Executable (For Developers)

We use **PyInstaller** to package the application into a single executable `clockwork-orange.exe`.

### Prerequisites
- Python 3.10+ (Anaconda/Miniforge recommended)
- `pip install -r requirements.txt` (ensure `pywin32` is installed)

### Build Command
Run the provided PowerShell script:
```powershell
.\scripts\build_windows.ps1
```
This will create `dist\clockwork-orange.exe`.

### GitHub Actions (Cloud Build)
We also support automated cloud builds via GitHub Actions.
1.  **Releases**: Tagged versions (e.g., `v2.7.0`) automatically build and publish to [GitHub Releases](https://github.com/ushineko/clockwork-orange/releases) — this is the easiest way to get the latest stable build.
2.  **Manual/Dev Builds**: Check the **Actions** tab for build artifacts from specific commits.

## 2. CLI Usage

You can use the executable just like the Python script.

**Set wallpaper from file:**
```powershell
.\dist\clockwork-orange.exe --desktop -f "C:\Path\To\Image.jpg"
```

**Set random wallpaper from directory:**
```powershell
.\dist\clockwork-orange.exe --desktop -d "C:\Wallpapers"
```

**Run GUI:**
```powershell
.\dist\clockwork-orange.exe --gui
```

## 3. Recommended Usage on Windows

### GUI with Autostart (Recommended)
The best way to use Clockwork Orange on Windows:

1. **Launch GUI**: Double-click `clockwork-orange.exe`
2. **Configure Plugins**: Enable desired plugins (Google Images, Wallhaven, Local, etc.)
3. **Set Interval**: Configure `default_wait` in Settings (seconds between wallpaper changes)
4. **Enable Autostart**: Check "Autostart" in Settings → Basic
5. **Minimize to Tray**: The GUI runs in background and changes wallpapers automatically

### Desktop Mode (Alternative)
For command-line usage without GUI:

```powershell
# Single wallpaper change
.\dist\clockwork-orange.exe --desktop

# Continuous mode (change every 600 seconds)
.\dist\clockwork-orange.exe --desktop --wait 600
```

Add to Startup folder (`shell:startup`) for automatic launch.



## 4. GUI Launch & Performance
**Issues Resolved**:
1.  **Direct Launch**: Double-clicking `clockwork-orange.exe` now opens the GUI by default (programmatically appends `--gui` if no args).
2.  **Invisible Window**: Fixed by bundling missing icons (`gui/icons`) and forcing window activation (`raise_()`).
3.  **Slow Startup**: Reduced startup time from >30s to <1s by removing slow `psutil` iteration and using a named mutex lock (Windows) / file lock (Linux) for single-instance checks.

**Verification**:
```powershell
# 1. Double-click dist/clockwork-orange.exe
# Result: GUI appears instantly.

# 2. Check logs (if debugging)
# [DEBUG] Instance check took 0.000s
```


To verify the installation and dependencies (including SSL and Plugins), run the self-test:
```powershell
.\dist\clockwork-orange.exe --self-test
```
A successful run will output `[OK]` for all checks and exit with code 0.

## 5. Troubleshooting

### Plugins Not Appearing/Working

- Ensure `plugins/` directory is correctly bundled.
- If running from source, ensure you have dependencies installed (`pip install -r requirements.txt`).
- In the frozen build, plugins are loaded internally. Check the GUI logs or console output for "Plugin execution failed" errors.

## 6. Multi-Monitor Support
Clockwork Orange supports setting different wallpapers on each connected monitor on Windows.

**The Solution:**
To avoid Windows security restrictions (RPC errors) that occur when running as Administrator, we use a custom **Spanned Wallpaper** engine:
1.  **Monitor Detection**: Switched from custom PowerShell/ctypes code to the `screeninfo` library for robust, cross-platform monitor geometry detection.
2.  **Image Stitching**: Uses `Pillow` to create a single composite image covering the entire virtual desktop.
3.  **Pixel-Perfect Placement**: Wallpapers are "stitched" onto this canvas to match your monitor layout perfectly.
4.  **System Application**: Sets the composite image as the system wallpaper in "Span" mode.

**Verification:**
Look for these messages in the Activity Log:
- `✓ Wallpapers set for 2 monitor(s)`
- `[DEBUG] Stitching wallpapers for 2 monitor(s)`
- `[DEBUG] Applying spanned wallpaper: ...\clockwork_spanned.jpg`

> [!TIP]
> This approach handles complex monitor layouts (mixed portrait/landscape, different resolutions, and offsets) automatically!
