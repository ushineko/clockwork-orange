# Spec 004: Windows 10/11 Platform Support

**Status**: COMPLETE
**Implementation Date**: 2025-01
**Commit**: (multi-commit feature)

## Overview
Extend Clockwork Orange to support Windows 10/11 platforms with native wallpaper APIs and PyInstaller-based frozen executable distribution.

## Requirements

### Functional Requirements
1. Detect Windows platform at runtime
2. Use Windows-specific APIs for wallpaper setting
3. Support multi-monitor setups on Windows
4. Package as standalone `.exe` with PyInstaller
5. Provide Windows-native service/autostart options

### Technical Requirements
1. Implement platform abstraction layer (`platform_utils.py`)
2. Use `ctypes` and `winreg` for Windows registry access
3. Call `SystemParametersInfoW` for wallpaper setting
4. Include all dependencies in frozen executable (Pillow, PyQt6, watchdog, etc.)
5. Handle Windows-specific paths and file permissions

## Implementation Details

### Platform Utilities Module
Create `platform_utils.py` with platform-specific implementations:
- `is_windows()` / `is_linux()` detection
- `get_monitor_count()` - Windows vs Linux implementations
- `set_desktop_wallpaper()` - Windows registry vs KDE D-Bus
- `set_lockscreen_wallpaper()` - Windows personalization vs kwriteconfig6

### Windows Wallpaper Implementation
```python
# Use ctypes to call SystemParametersInfoW
SPI_SETDESKWALLPAPER = 20
ctypes.windll.user32.SystemParametersInfoW(
    SPI_SETDESKWALLPAPER, 0, wallpaper_path, 3
)
```

### Registry Configuration
- Set wallpaper style via `winreg` module
- Configure multi-monitor spanning settings
- Update personalization settings for lock screen

### PyInstaller Build
- Build scripts: `scripts/build_windows.ps1` (debug), `scripts/build_windows_production.ps1` (release)
- Include hidden imports for dynamic modules (watchdog, PIL)
- Collect all plugin files and resources
- Use `--noconsole` for production builds
- Include README.md for About dialog

### Known Limitations (Beta)
- Lock screen support varies by Windows version
- Unsigned executable triggers SmartScreen warnings
- Multi-monitor support differs from KDE implementation

## Acceptance Criteria

- [x] Platform detection works correctly
- [x] Desktop wallpaper sets on Windows 10/11
- [x] Multi-monitor wallpaper works on Windows
- [x] GUI launches on Windows without console window
- [x] PyInstaller builds complete without errors
- [x] Frozen executable includes all plugins and dependencies
- [x] `watchdog` module included in frozen builds (fix: d571111)
- [x] Self-test passes in frozen executable
- [x] README.md included for About dialog (fix: 6602c2f)
- [x] Exit codes propagate correctly in `--noconsole` mode (fix: b9763a8)

## Testing

### Test Cases
1. Install on fresh Windows 10 system
2. Install on Windows 11 system
3. Test multi-monitor setup (2+ displays)
4. Launch frozen executable (.exe)
5. Run self-test in frozen executable
6. Verify all plugins load correctly
7. Test GUI functionality on Windows

### Expected Behavior
- Executable runs without requiring Python installation
- Wallpaper changes apply immediately
- No console window appears in production build
- All plugins (local, Google Images, Wallhaven, Stable Diffusion) work
- Service manager uses Windows Task Scheduler

## Notes
- Windows support is beta - feedback encouraged
- Unsigned executable requires SmartScreen bypass
- Code signing would eliminate SmartScreen warnings but requires certificate
- PyInstaller builds require careful dependency management
- Watchdog package requires explicit `--collect-all` directive
- Future: Investigate Windows Store distribution
- Future: Add Windows Installer (MSI/WiX) for easier deployment
