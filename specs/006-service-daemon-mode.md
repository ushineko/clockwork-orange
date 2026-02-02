# Spec 006: Background Service and Daemon Mode

**Status**: COMPLETE
**Implementation Date**: 2025-12
**Commit**: Multiple commits, service fixes 2025-12-24

## Overview
Enable Clockwork Orange to run as a background service/daemon with automatic wallpaper cycling at configured intervals.

## Requirements

### Functional Requirements
1. Run continuously in the background without user interaction
2. Cycle wallpapers at specified intervals (e.g., every 5 minutes)
3. Support both systemd (Linux) and Task Scheduler (Windows)
4. GUI controls for starting/stopping/restarting service
5. Autostart on user login (optional)
6. Log service activity for debugging

### Technical Requirements
1. **Linux**: systemd user service (`~/.config/systemd/user/clockwork-orange.service`)
2. **Windows**: Task Scheduler or startup folder integration
3. Service reads configuration from `~/.config/clockwork-orange.yml`
4. Service respects plugin enable/disable settings
5. Graceful shutdown on SIGTERM/SIGINT

## Implementation Details

### Linux: Systemd User Service
Service file (`clockwork-orange.service`):
```ini
[Unit]
Description=Clockwork Orange Wallpaper Manager
After=graphical-session.target

[Service]
Type=simple
ExecStart=/usr/bin/python3 /path/to/clockwork-orange.py
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=default.target
```

### Service Control (Linux)
Implemented in `platform_utils.py`:
- `is_service_running()` - check systemd status
- `get_service_status()` - full status info
- `start_service()` - systemctl start
- `stop_service()` - systemctl stop
- `restart_service()` - systemctl restart
- `enable_service()` - install + enable autostart
- `disable_service()` - stop + disable autostart
- `get_service_logs()` - tail journalctl

### Service Manager UI (GUI)
Tab in main window (`gui/service_manager.py`):
- **Status Display**: Running/Stopped indicator
- **Control Buttons**: Start, Stop, Restart
- **Autostart Toggle**: Enable/Disable service on login
- **Log Viewer**: Real-time or recent logs
- **Configuration Summary**: Show active plugins, wait interval

### Wait Interval Implementation
In main script (`clockwork-orange.py`):
```python
while True:
    # Set wallpapers from enabled plugins
    set_wallpapers()

    # Wait for configured interval
    time.sleep(config['default_wait'])
```

### Wrapper Script (`run_clockwork_orange.sh`)
- Sets up environment variables (DISPLAY, DBUS_SESSION_BUS_ADDRESS)
- Ensures proper KDE session integration
- Handles logging to file or journald

## Acceptance Criteria

- [x] Systemd service file created (`clockwork-orange.service`)
- [x] Service installs to `~/.config/systemd/user/`
- [x] Service starts/stops via systemctl commands
- [x] Service respects `default_wait` configuration
- [x] GUI service manager tab functional
- [x] Start/Stop/Restart buttons work in GUI
- [x] Autostart enable/disable works
- [x] Service logs accessible via journalctl
- [x] Service survives user logout (when enabled)
- [x] Graceful shutdown on SIGTERM
- [x] Service auto-restarts on failure

## Testing

### Test Cases (Linux)
1. Install service via GUI
2. Start service and verify wallpaper changes
3. Stop service and verify cycling stops
4. Restart service and verify no interruption
5. Enable autostart and reboot system
6. Disable autostart and reboot system
7. View service logs after running for 1 hour
8. Test failure recovery (kill -9 service process)

### Test Cases (Windows)
1. Install as Task Scheduler task
2. Start via GUI service manager
3. Verify wallpaper cycling
4. Test autostart on Windows login

### Expected Behavior
- Service runs in background without visible window
- Wallpapers change at configured intervals
- Service respects all plugin configurations
- Logs are accessible for debugging
- Service survives crashes (auto-restart)

## Notes
- Linux implementation uses systemd (standard on most modern distros)
- Windows implementation varies (Task Scheduler or startup folder)
- Service requires DISPLAY and DBUS_SESSION_BUS_ADDRESS on Linux
- Future: Add service health monitoring and notifications
- Future: Support for cron-like scheduling (specific times/days)
- Future: Pause/resume functionality via GUI
