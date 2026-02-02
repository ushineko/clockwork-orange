# Spec 001: Multi-Monitor Wallpaper Support

**Status**: COMPLETE
**Implementation Date**: 2024-01
**Commit**: (baseline feature)

## Overview
Implement multi-monitor support for KDE Plasma 6 to set unique wallpapers on each connected display.

## Requirements

### Functional Requirements
1. Detect the number of connected monitors using KDE Plasma's D-Bus interface
2. Assign a unique wallpaper to each monitor from the available image pool
3. Support both desktop and lock screen wallpapers across all monitors
4. Handle monitor configuration changes gracefully

### Technical Requirements
1. Use `qdbus6` to communicate with KDE Plasma's wallpaper API
2. Use JavaScript-based wallpaper configuration via D-Bus
3. Ensure wallpapers are set atomically (all monitors update together)

## Implementation Details

### Monitor Detection
- Query desktop count via D-Bus interface
- Store monitor count for wallpaper allocation

### Wallpaper Assignment
- Generate array of wallpaper file paths matching monitor count
- Use modulo operator to cycle through available images if monitors > images
- Execute JavaScript wallpaper configuration for all desktops

### Lock Screen Support
- Separate implementation using `kwriteconfig6` for lock screen configuration
- Modify `~/.config/kscreenlockerrc` configuration file

## Acceptance Criteria

- [x] System detects number of connected monitors correctly
- [x] Each monitor receives a unique wallpaper when multiple images are available
- [x] Desktop wallpapers are set correctly on all monitors
- [x] Lock screen wallpaper is set correctly
- [x] Configuration persists across KDE Plasma restarts
- [x] Script handles single-monitor setups correctly
- [x] No monitors are left without a wallpaper

## Testing

### Test Cases
1. Single monitor setup
2. Dual monitor setup
3. Triple+ monitor setup
4. Monitor disconnect/reconnect scenarios

### Expected Behavior
- All monitors display wallpapers after script execution
- Different wallpapers on each monitor when sufficient images exist
- Graceful degradation when images < monitors (cycling/reuse)

## Notes
- KDE Plasma 6 required (`qdbus6`, `kwriteconfig6`)
- Implementation uses KDE-specific APIs (not portable to GNOME/other DEs)
- Future: Add support for per-monitor wallpaper preferences
