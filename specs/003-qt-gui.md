# Spec 003: Qt-based Graphical User Interface

**Status**: COMPLETE
**Implementation Date**: 2024-08
**Commit**: (baseline feature)

## Overview
Build a comprehensive Qt6-based GUI for managing wallpaper configuration, plugins, and background service without requiring command-line knowledge.

## Requirements

### Functional Requirements
1. Main window with tabbed interface
2. Plugin management tab with enable/disable toggles
3. Settings tab for global configuration (interval, dual-wallpaper mode)
4. Blacklist manager tab for reviewing banned images
5. Service manager tab for systemd service control
6. Activity log tab for monitoring wallpaper changes
7. About dialog with version information

### Technical Requirements
1. Use PyQt6 for cross-platform GUI
2. Support both Linux (KDE Plasma 6) and Windows 10/11
3. Live configuration updates (write to YAML on change)
4. Service control integration (systemd on Linux, Task Scheduler on Windows)

## Implementation Details

### Main Window Structure
- Tab 1: Plugins (enable/disable, configure, review images)
- Tab 2: Settings (global options, intervals, dual-wallpaper)
- Tab 3: Blacklist Manager (view/remove banned images)
- Tab 4: Service (start/stop/status, autostart)
- Tab 5: Activity Log (recent wallpaper changes)

### Plugin Configuration Widgets
Each plugin has custom configuration UI:
- **Local**: Directory picker, recursive toggle
- **Wallhaven**: Search terms, API key, filters (SFW/NSFW)
- **Google Images**: Multi-term editor, max files, interval
- **Stable Diffusion**: Prompt list, model selection, resolution, safety filter

### Review Mode
- Scan plugin download directory for images
- Display images in preview panel
- Navigate with left/right arrow keys
- Delete button adds to blacklist and removes file

### Service Manager
- Display service status (running/stopped)
- Start/Stop/Restart buttons
- Enable/Disable autostart
- View service logs (journalctl on Linux)

## Acceptance Criteria

- [x] Main window launches with `--gui` flag
- [x] All tabs render correctly on both Linux and Windows
- [x] Plugin enable/disable toggles work
- [x] Configuration changes save to `~/.config/clockwork-orange.yml`
- [x] Review mode loads and displays images
- [x] Delete/Blacklist functionality works in review mode
- [x] Service manager controls systemd service correctly (Linux)
- [x] Activity log displays recent wallpaper changes
- [x] About dialog shows version and license information
- [x] Desktop entry installation script works (`install-desktop-entry.sh`)

## Testing

### Test Cases
1. Launch GUI on Linux (KDE Plasma 6)
2. Launch GUI on Windows 10/11
3. Enable/disable plugins and verify config file
4. Change wait interval and verify persistence
5. Start/stop service via GUI
6. Review images and add to blacklist
7. Install desktop entry and launch from application menu

### Expected Behavior
- GUI is responsive and doesn't freeze during operations
- Configuration changes are immediately saved
- Service controls work without requiring terminal
- Review mode handles large image directories efficiently
- Blacklist updates are reflected in plugin behavior

## Notes
- Requires PyQt6 and Pillow (optional but recommended)
- GUI is optional (CLI mode still fully functional)
- Windows version uses different service management approach
- Future: Add dark mode theme support
- Future: Add wallpaper preview before applying
