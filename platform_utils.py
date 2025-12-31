import os
import subprocess
import sys
from pathlib import Path

# Windows specific flag to hide console window when spawning processes
# This prevents blank CMD windows from popping up in GUI applications
CREATE_NO_WINDOW = 0x08000000 if sys.platform == "win32" else 0

# Note: import ctypes moved to function scope to avoid PyInstaller freeze issues?

IS_WINDOWS = sys.platform == "win32"


def is_windows():
    return IS_WINDOWS


def set_wallpaper(image_path: Path) -> bool:
    """
    Set the desktop wallpaper.
    """
    image_path = image_path.resolve()

    if IS_WINDOWS:
        return _set_wallpaper_windows(image_path)
    else:
        return _set_wallpaper_linux(image_path)


def set_lockscreen_wallpaper(image_path: Path) -> bool:
    """
    Set the lock screen wallpaper.
    Returns False on Windows as it is not supported/required.
    """
    if IS_WINDOWS:
        # Windows lockscreen is not supported/requested
        print("[INFO] Lock screen wallpaper not supported on Windows.")
        return False
    else:
        return _set_lockscreen_wallpaper_linux(image_path)


# --- Windows Implementations ---


def get_monitor_count() -> int:
    """Get the number of connected monitors on Windows using PowerShell."""
    if not IS_WINDOWS:
        return 1
    try:
        # Use PowerShell to get monitor count from Screens collection
        # This is very fast and robust
        cmd = [
            "powershell",
            "-NoProfile",
            "-Command",
            "[Reflection.Assembly]::LoadWithPartialName('System.Windows.Forms') > $null; [System.Windows.Forms.Screen]::AllScreens.Count",
        ]
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=5,
            creationflags=CREATE_NO_WINDOW,
        )
        if result.returncode == 0:
            count = int(result.stdout.strip())
            print(f"[DEBUG] Detected {count} monitor(s) via PowerShell")
            return count
    except Exception as e:
        print(f"[DEBUG] Failed to get monitor count via PS, assuming 1: {e}")
    return 1


def set_wallpaper_multi_monitor(image_paths: list) -> bool:
    """Set different wallpaper for each monitor on Windows via PowerShell."""
    if not IS_WINDOWS:
        return False

    if not image_paths:
        print(f"[ERROR] No image paths provided")
        return False

    # Resolve absolute paths and validate
    abs_paths = []
    for p in image_paths:
        path = Path(p).resolve()
        if path.exists():
            abs_paths.append(str(path))

    if not abs_paths:
        print("[ERROR] No valid image paths found")
        return False

    # Spanned Wallpaper Approach:
    # Instead of fighting with finicky COM interfaces that fail when Elevated,
    # we stitch the images together into one giant "canvas" that spans all monitors.
    # Windows then treats this as a single "Spanned" wallpaper.
    try:
        import ctypes
        import os
        import winreg
        from ctypes import wintypes

        from PIL import Image

        # 1. Get monitor geometries
        monitors = []

        def _enum_proc(hMonitor, hdcMonitor, lprcMonitor, dwData):
            rect = lprcMonitor.contents
            monitors.append(
                {
                    "x": rect.left,
                    "y": rect.top,
                    "w": rect.right - rect.left,
                    "h": rect.bottom - rect.top,
                }
            )
            return True

        MonitorEnumProc = ctypes.WINFUNCTYPE(
            ctypes.c_bool,
            wintypes.HMONITOR,
            wintypes.HDC,
            ctypes.POINTER(wintypes.RECT),
            wintypes.LPARAM,
        )
        ctypes.windll.user32.EnumDisplayMonitors(
            None, None, MonitorEnumProc(_enum_proc), 0
        )

        if not monitors:
            print("[ERROR] No monitors detected via EnumDisplayMonitors")
            return False

        print(f"[DEBUG] Stitching wallpapers for {len(monitors)} monitor(s)")

        # 2. Determine canvas bounding box
        min_x = min(m["x"] for m in monitors)
        min_y = min(m["y"] for m in monitors)
        max_x = max(m["x"] + m["w"] for m in monitors)
        max_y = max(m["y"] + m["h"] for m in monitors)
        canvas_w = max_x - min_x
        canvas_h = max_y - min_y

        # 3. Create black canvas
        canvas = Image.new("RGB", (canvas_w, canvas_h), (0, 0, 0))

        # 4. Paste each wallpaper into its monitor area
        for i, m in enumerate(monitors):
            if i >= len(abs_paths):
                break

            try:
                with Image.open(abs_paths[i]) as img:
                    # Convert to RGB (to handle PNG/RGBA or CMYK)
                    if img.mode != "RGB":
                        img = img.convert("RGB")

                    # Calculate "Cover" resize (fill monitor area)
                    iw, ih = img.size
                    mw, mh = m["w"], m["h"]

                    aspect_img = iw / ih
                    aspect_mon = mw / mh

                    if aspect_img > aspect_mon:
                        # Image is wider than monitor
                        new_h = mh
                        new_w = int(aspect_img * new_h)
                    else:
                        # Image is taller than monitor
                        new_w = mw
                        new_h = int(new_w / aspect_img)

                    # Resize
                    img = img.resize(
                        (new_w, new_h),
                        (
                            Image.Resampling.LANCZOS
                            if hasattr(Image, "Resampling")
                            else Image.LANCZOS
                        ),
                    )

                    # Center crop to exact monitor size
                    left = (new_w - mw) // 2
                    top = (new_h - mh) // 2
                    img = img.crop((left, top, left + mw, top + mh))

                    # Paste at monitor's relative position on canvas
                    canvas.paste(img, (m["x"] - min_x, m["y"] - min_y))
            except Exception as e:
                print(f"[ERROR] Failed to process image {abs_paths[i]}: {e}")

        # 5. Save the spanned image to a temporary file
        temp_dir = Path(os.environ.get("TEMP", os.environ.get("USERPROFILE", ".")))
        spanned_path = temp_dir / "clockwork_spanned.jpg"
        canvas.save(str(spanned_path), "JPEG", quality=90)

        # 6. Set registry for "Span" style (Style 22)
        try:
            key = winreg.OpenKey(
                winreg.HKEY_CURRENT_USER,
                r"Control Panel\Desktop",
                0,
                winreg.KEY_SET_VALUE,
            )
            winreg.SetValueEx(key, "WallpaperStyle", 0, winreg.REG_SZ, "22")
            winreg.SetValueEx(key, "TileWallpaper", 0, winreg.REG_SZ, "0")
            winreg.CloseKey(key)
        except Exception as e:
            print(f"[ERROR] Failed to set registry for spanning: {e}")

        # 7. Apply the stitched wallpaper
        SPI_SETDESKWALLPAPER = 0x0014
        SPIF_UPDATEINIFILE = 0x01
        SPIF_SENDWININICHANGE = 0x02

        print(f"[DEBUG] Applying spanned wallpaper: {spanned_path}")
        result = ctypes.windll.user32.SystemParametersInfoW(
            SPI_SETDESKWALLPAPER,
            0,
            str(spanned_path),
            SPIF_UPDATEINIFILE | SPIF_SENDWININICHANGE,
        )
        return bool(result)

    except Exception as e:
        print(f"[ERROR] Spanned wallpaper stitching failed: {e}")
        import traceback

        traceback.print_exc()
        return False


def _set_wallpaper_windows(image_path: Path) -> bool:
    """Set wallpaper on Windows (single monitor or all monitors)."""
    print(f"[DEBUG] Setting Windows wallpaper: {image_path}")
    if not image_path.exists():
        print(f"[ERROR] File does not exist: {image_path}")
        return False

    # Try multi-monitor API first (works for single monitor too)
    try:
        return set_wallpaper_multi_monitor([image_path])
    except Exception as e:
        print(
            f"[DEBUG] Multi-monitor API failed, falling back to SystemParametersInfoW: {e}"
        )

    # Fallback to SystemParametersInfoW
    try:
        import ctypes

        abs_path = str(image_path)
        # SPI_SETDESKWALLPAPER = 20
        # SPIF_UPDATEINIFILE = 0x01
        # SPIF_SENDWININICHANGE = 0x02
        res = ctypes.windll.user32.SystemParametersInfoW(
            20, 0, abs_path, 3  # SPIF_UPDATEINIFILE | SPIF_SENDWININICHANGE
        )
        if res:
            return True
        else:
            print(
                f"[ERROR] SystemParametersInfoW failed. Code: {ctypes.GetLastError()}"
            )
            return False
    except Exception as e:
        print(f"[ERROR] Failed to set Windows wallpaper: {e}")
        return False


# --- Linux Implementations (Moved from clockwork-orange.py) ---


def _set_wallpaper_linux(p: Path) -> bool:
    print(f"[DEBUG] Setting KDE wallpaper from path: {p}")
    if not p.exists():
        print(f"[ERROR] File does not exist: {p}")
        return False

    script = """
    desktops().forEach(d => {
        d.currentConfigGroup = Array("Wallpaper",
                                     "org.kde.image",
                                     "General");
        d.writeConfig("Image", "file://FILEPATH");
        d.reloadConfig();
    });
    """.replace(
        "FILEPATH", str(p)
    )

    cmd = [
        "qdbus6",
        "org.kde.plasmashell",
        "/PlasmaShell",
        "org.kde.PlasmaShell.evaluateScript",
        script,
    ]

    try:
        # We don't check=True here to allow graceful failure handling if qdbus6 is missing
        # But wait, original code did check=True inside a try/except for CalledProcessError
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return True
    except subprocess.CalledProcessError as e:
        print(f"[ERROR] qdbus6 failed: {e.stderr}")
        return False
    except FileNotFoundError:
        print(f"[ERROR] qdbus6 not found.")
        return False


def _set_lockscreen_wallpaper_linux(image_path: Path) -> bool:
    image_path = image_path.resolve()
    if not image_path.exists():
        return False

    wallpaper_path = f"file://{image_path}"

    cmd = [
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
        "Image",
        wallpaper_path,
    ]

    try:
        subprocess.run(cmd, capture_output=True, text=True, check=True)
        # Assuming clean up and reload logic remains in main or we move it here?
        # The original code had helper functions like _clean_lockscreen_config inside main.
        # For now, let's keep the core "set" primitive here.
        # Ideally we'd move all that logic here but let's start with the basic setter.
        # Wait, the reload logic is important for it to take effect?
        # The original code called _reload_screensaver_config() after this.
        # We should probably expose that too or include it.
        # Let's include _reload_screensaver_config logic if possible, or leave it to the caller.
        # The caller (clockwork-orange.py) calls _reload_screensaver_config.
        # I will expose a 'reload_lockscreen_config' function and call it there?
        # Or better, just do it here.

        _reload_screensaver_config_linux()
        return True
    except Exception as e:
        print(f"[ERROR] Failed to set lockscreen: {e}")
        return False


def _reload_screensaver_config_linux():
    # Attempt to reload kscreenlocker
    # Original code: qdbus6 org.freedesktop.ScreenSaver /ScreenSaver configure
    # But wait, looking at original code...
    # It seems it was calling _reload_screensaver_config() which does some qdbus magic.

    try:
        # Try to find the service/object, it might be different depending on system state
        # Original code used:
        # qdbus6 org.freedesktop.ScreenSaver /ScreenSaver configure
        subprocess.run(
            ["qdbus6", "org.freedesktop.ScreenSaver", "/ScreenSaver", "configure"],
            capture_output=True,
            check=False,
        )

    except:
        pass


# --- Service Management ---

SERVICE_NAME_LINUX = "clockwork-orange.service"
SERVICE_NAME_WINDOWS = "ClockworkOrangeService"


def get_service_name():
    return SERVICE_NAME_WINDOWS if IS_WINDOWS else SERVICE_NAME_LINUX


def service_is_active() -> str:
    """Returns 'active', 'inactive', 'failed', or 'unknown'."""
    if IS_WINDOWS:
        return _service_is_active_windows()
    else:
        return _service_is_active_linux()


def service_get_status_details() -> str:
    if IS_WINDOWS:
        return _service_get_status_details_windows()
    else:
        return _service_get_status_details_linux()


def service_start():
    if IS_WINDOWS:
        return _service_start_windows()
    else:
        return _service_start_linux()


def service_stop():
    if IS_WINDOWS:
        return _service_stop_windows()
    else:
        return _service_stop_linux()


def service_restart():
    if IS_WINDOWS:
        return _service_restart_windows()
    else:
        return _service_restart_linux()


def service_install(base_path: Path):
    if IS_WINDOWS:
        return _service_install_windows(base_path)
    else:
        return _service_install_linux(base_path)


def service_uninstall():
    if IS_WINDOWS:
        return _service_uninstall_windows()
    else:
        return _service_uninstall_linux()


def service_get_logs() -> str:
    if IS_WINDOWS:
        return _service_get_logs_windows()
    else:
        return _service_get_logs_linux()


# --- Linux Service Implementation ---


def _service_is_active_linux() -> str:
    try:
        result = subprocess.run(
            ["systemctl", "--user", "is-active", SERVICE_NAME_LINUX],
            capture_output=True,
            text=True,
            timeout=5,
        )
        return result.stdout.strip()
    except:
        return "unknown"


def _service_get_status_details_linux() -> str:
    try:
        result = subprocess.run(
            ["systemctl", "--user", "status", SERVICE_NAME_LINUX, "--no-pager"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        return result.stdout
    except Exception as e:
        return str(e)


def _service_start_linux():
    subprocess.run(["systemctl", "--user", "start", SERVICE_NAME_LINUX], check=True)


def _service_stop_linux():
    subprocess.run(["systemctl", "--user", "stop", SERVICE_NAME_LINUX], check=True)


def _service_restart_linux():
    subprocess.run(["systemctl", "--user", "restart", SERVICE_NAME_LINUX], check=True)


def _service_install_linux(base_path: Path):
    # Logic copied from original service_manager.py, but simplified for invalid paths
    # We expect base_path to be the location of the repo/script root
    service_file = base_path / "clockwork-orange.service"
    if not service_file.exists():
        raise FileNotFoundError("Service file not found!")

    systemd_dir = Path.home() / ".config" / "systemd" / "user"
    systemd_dir.mkdir(parents=True, exist_ok=True)

    import shutil

    shutil.copy2(service_file, systemd_dir / SERVICE_NAME_LINUX)

    subprocess.run(["systemctl", "--user", "daemon-reload"], check=True)
    subprocess.run(["systemctl", "--user", "enable", SERVICE_NAME_LINUX], check=True)


def _service_uninstall_linux():
    subprocess.run(["systemctl", "--user", "stop", SERVICE_NAME_LINUX], check=False)
    subprocess.run(["systemctl", "--user", "disable", SERVICE_NAME_LINUX], check=False)

    service_file = Path.home() / ".config" / "systemd" / "user" / SERVICE_NAME_LINUX
    if service_file.exists():
        service_file.unlink()

    subprocess.run(["systemctl", "--user", "daemon-reload"], check=True)


def _service_get_logs_linux() -> str:
    try:
        result = subprocess.run(
            [
                "journalctl",
                "--user",
                "-u",
                SERVICE_NAME_LINUX,
                "--no-pager",
                "-n",
                "50",
            ],
            capture_output=True,
            text=True,
            timeout=10,
        )
        return result.stdout
    except Exception as e:
        return f"Error retrieving logs: {e}"


# --- Windows Service Implementation ---


def _service_is_active_windows() -> str:
    try:
        import win32service
        import win32serviceutil

        status = win32serviceutil.QueryServiceStatus(SERVICE_NAME_WINDOWS)[1]
        if status == win32service.SERVICE_RUNNING:
            return "active"
        elif status == win32service.SERVICE_STOPPED:
            return "inactive"
        elif status == win32service.SERVICE_START_PENDING:
            return "activating"
        elif status == win32service.SERVICE_STOP_PENDING:
            return "deactivating"
        else:
            return "unknown"
    except Exception:
        # Service likely not installed
        return "inactive"  # Or "not-installed"?


def _service_get_status_details_windows() -> str:
    try:
        import win32service
        import win32serviceutil

        # Check if installed
        try:
            win32serviceutil.QueryServiceStatus(SERVICE_NAME_WINDOWS)
        except:
            return "Service not installed."

        status = _service_is_active_windows()
        return f"Service Status: {status}\\nService Name: {SERVICE_NAME_WINDOWS}"
    except Exception as e:
        return str(e)


def _service_restart_windows():
    import win32serviceutil

    win32serviceutil.RestartService(SERVICE_NAME_WINDOWS)


def _run_as_admin_windows(exe_path: str, params: str = "") -> bool:
    """
    Execute a command with UAC elevation using ShellExecuteW.
    Returns True if the elevated process was started successfully.
    """
    import ctypes

    # ShellExecuteW parameters
    # lpVerb: "runas" triggers UAC elevation
    # nShow: SW_SHOWNORMAL = 1
    return ctypes.windll.shell32.ShellExecuteW(
        None,  # hwnd
        "runas",  # lpVerb (run as administrator)
        exe_path,  # lpFile
        params,  # lpParameters
        None,  # lpDirectory
        1,  # nShowCmd (SW_SHOWNORMAL)
    ) > 32


def _service_install_windows(base_path: Path):
    if getattr(sys, "frozen", False):
        exe_path = sys.executable
        if not _run_as_admin_windows(exe_path, "install"):
            raise RuntimeError(
                "Failed to elevate for service installation. User may have cancelled UAC prompt."
            )
    else:
        raise RuntimeError(
            "Service installation only supported in frozen (exe) mode on Windows."
        )


def _service_uninstall_windows():
    if getattr(sys, "frozen", False):
        exe_path = sys.executable
        if not _run_as_admin_windows(exe_path, "remove"):
            raise RuntimeError(
                "Failed to elevate for service uninstallation. User may have cancelled UAC prompt."
            )
    else:
        raise RuntimeError(
            "Service uninstallation only supported in frozen (exe) mode on Windows."
        )


def _service_start_windows():
    if getattr(sys, "frozen", False):
        exe_path = sys.executable
        if not _run_as_admin_windows(exe_path, "start"):
            raise RuntimeError(
                "Failed to elevate for service start. User may have cancelled UAC prompt."
            )
    else:
        import win32serviceutil

        win32serviceutil.StartService(SERVICE_NAME_WINDOWS)


def _service_stop_windows():
    if getattr(sys, "frozen", False):
        exe_path = sys.executable
        if not _run_as_admin_windows(exe_path, "stop"):
            raise RuntimeError(
                "Failed to elevate for service stop. User may have cancelled UAC prompt."
            )
    else:
        import win32serviceutil

        win32serviceutil.StopService(SERVICE_NAME_WINDOWS)


def _service_get_logs_windows() -> str:
    # Windows doesn't have journalctl.
    # We can read from Event Log or a log file.
    # Our implementation plan wrote to a log file in home directory.
    # Let's assume standard log file location.

    log_file = (
        Path.home() / "clockwork_service_test.log"
    )  # Make this consistent with service implementation
    if log_file.exists():
        try:
            # Read last 50 lines
            with open(log_file, "r") as f:
                lines = f.readlines()
            return "".join(lines[-50:])
        except Exception as e:
            return f"Error reading log file: {e}"
    else:
        return "No log file found."


# --- Instance Locking ---
global_lock_handle = None


def acquire_instance_lock(app_id: str) -> bool:
    """
    Acquire a named lock to ensure single instance.
    Returns True if lock acquired (we are the only instance).
    Returns False if another instance holds the lock.
    """
    if IS_WINDOWS:
        return _acquire_lock_windows(app_id)
    else:
        return _acquire_lock_linux(app_id)


def _acquire_lock_windows(app_id: str) -> bool:
    global global_lock_handle
    import ctypes
    from ctypes import wintypes

    # Create unique mutex name (Global\ ensures it works across sessions)
    mutex_name = f"Global\\{app_id}"

    kernel32 = ctypes.windll.kernel32

    # CreateMutexW will return a handle even if it already exists,
    # but GetLastError will be ERROR_ALREADY_EXISTS (183)
    mutex = kernel32.CreateMutexW(None, True, mutex_name)
    last_error = ctypes.GetLastError()

    if not mutex:
        print(f"[ERROR] CreateMutexW failed. Error: {last_error}")
        return True  # Default to allow run on system error? Or fail safe?

    global_lock_handle = mutex

    if last_error == 183:  # ERROR_ALREADY_EXISTS
        return False

    return True


def _acquire_lock_linux(app_id: str) -> bool:
    global global_lock_handle
    import fcntl

    lock_file = Path(f"/tmp/{app_id}.lock")
    try:
        fp = open(lock_file, "w")
        # Try to acquire an exclusive lock without blocking (LOCK_NB)
        fcntl.flock(fp, fcntl.LOCK_EX | fcntl.LOCK_NB)
        global_lock_handle = fp  # Keep file open
        return True
    except IOError:
        return False
