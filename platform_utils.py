import os
import subprocess
import sys
from pathlib import Path

# Windows specific flag to hide console window when spawning processes
# This prevents blank CMD windows from popping up in GUI applications
CREATE_NO_WINDOW = 0x08000000 if sys.platform == "win32" else 0

# Note: import ctypes moved to function scope to avoid PyInstaller freeze issues?

IS_WINDOWS = sys.platform == "win32"
IS_MACOS = sys.platform == "darwin"
IS_LINUX = sys.platform == "linux"


def is_windows():
    return IS_WINDOWS


def is_macos():
    return IS_MACOS


def is_linux():
    return IS_LINUX


def set_wallpaper(image_path: Path) -> bool:
    """
    Set the desktop wallpaper.
    """
    image_path = image_path.resolve()

    if IS_WINDOWS:
        return _set_wallpaper_windows(image_path)
    elif IS_MACOS:
        return _set_wallpaper_macos(image_path)
    else:
        return _set_wallpaper_linux(image_path)


def set_lockscreen_wallpaper(image_path: Path) -> bool:
    """
    Set the lock screen wallpaper.
    Returns False on Windows and macOS as it is not supported/required.
    """
    if IS_WINDOWS:
        print("[INFO] Lock screen wallpaper not supported on Windows.")
        return False
    elif IS_MACOS:
        print("[INFO] Lock screen wallpaper on macOS requires elevated privileges. Not supported.")
        return False
    else:
        return _set_lockscreen_wallpaper_linux(image_path)


# --- Windows Implementations ---


def get_monitor_count() -> int:
    """Get the number of connected monitors."""
    if IS_WINDOWS:
        try:
            import screeninfo

            return len(screeninfo.get_monitors())
        except Exception as e:
            print(
                f"[DEBUG] Failed to get monitor count via screeninfo, assuming 1: {e}"
            )
            return 1
    elif IS_MACOS:
        return _get_monitor_count_macos()
    else:
        # Linux: Use qdbus6 to query Plasma Shell for desktop count
        cmd = [
            "qdbus6",
            "org.kde.plasmashell",
            "/PlasmaShell",
            "org.kde.PlasmaShell.evaluateScript",
            "print(desktops().length)",
        ]
        try:
            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            count = int(result.stdout.strip())
            print(f"[DEBUG] Detected {count} monitors")
            return count
        except (subprocess.CalledProcessError, ValueError) as e:
            print(f"[DEBUG] Failed to detect monitor count: {e}")
            return 1
        except FileNotFoundError:
            print(f"[DEBUG] qdbus6 not found, assuming 1 monitor")
            return 1


def set_wallpaper_multi_monitor(image_paths: list) -> bool:
    """Set different wallpaper for each monitor."""
    if IS_MACOS:
        return _set_wallpaper_multi_monitor_macos(image_paths)
    elif IS_LINUX:
        return _set_wallpaper_multi_monitor_linux(image_paths)

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

    # Spanned Wallpaper Approach with screeninfo
    try:
        import ctypes
        import os
        import winreg

        import screeninfo
        from PIL import Image

        # 1. Get monitor geometries via screeninfo
        # screeninfo returns a list of Monitor(x, y, width, height, ...)
        monitors_obj = screeninfo.get_monitors()
        if not monitors_obj:
            print("[ERROR] No monitors detected via screeninfo")
            return False

        monitors = [
            {"x": m.x, "y": m.y, "w": m.width, "h": m.height} for m in monitors_obj
        ]

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


def _set_wallpaper_multi_monitor_linux(image_paths: list) -> bool:
    """Set different wallpapers for each monitor on Linux (KDE Plasma 6)."""
    if not image_paths:
        return False

    # Resolve all paths and validate
    resolved_paths = []
    for p in image_paths:
        path = Path(p).resolve()
        if not path.exists():
            print(f"[ERROR] File does not exist: {path}")
            continue
        resolved_paths.append(str(path))

    if not resolved_paths:
        return False

    print(f"[DEBUG] Setting wallpapers for multiple monitors: {resolved_paths}")

    # Create JS array string: '["file://...", "file://..."]'
    js_images = ", ".join([f'"file://{p}"' for p in resolved_paths])

    script = f"""
    var allDesktops = desktops();
    var images = [{js_images}];

    for (var i = 0; i < allDesktops.length; i++) {{
        var d = allDesktops[i];
        d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
        var img = images[i % images.length];
        d.writeConfig("Image", img);
        d.reloadConfig();
    }}
    """

    cmd = [
        "qdbus6",
        "org.kde.plasmashell",
        "/PlasmaShell",
        "org.kde.PlasmaShell.evaluateScript",
        script,
    ]

    try:
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        return True
    except subprocess.CalledProcessError as e:
        print(f"[ERROR] qdbus6 command failed: {e.stderr}")
        return False
    except FileNotFoundError:
        print("[ERROR] qdbus6 not found")
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


# --- macOS Implementations ---


def _set_wallpaper_macos(image_path: Path) -> bool:
    """Set wallpaper on macOS using NSWorkspace."""
    print(f"[DEBUG] Setting macOS wallpaper: {image_path}")
    if not image_path.exists():
        print(f"[ERROR] File does not exist: {image_path}")
        return False

    try:
        import AppKit
        import Foundation

        workspace = AppKit.NSWorkspace.sharedWorkspace()
        file_url = Foundation.NSURL.fileURLWithPath_(str(image_path))
        options = {
            AppKit.NSWorkspaceDesktopImageScalingKey: AppKit.NSImageScaleProportionallyUpOrDown,
            AppKit.NSWorkspaceDesktopImageAllowClippingKey: True,
        }

        for screen in AppKit.NSScreen.screens():
            result, error = workspace.setDesktopImageURL_forScreen_options_error_(
                file_url, screen, options, None
            )
            if not result:
                print(f"[ERROR] Failed to set wallpaper: {error}")
                return False
        return True
    except ImportError:
        return _set_wallpaper_macos_osascript(image_path)


def _set_wallpaper_macos_osascript(image_path: Path) -> bool:
    """Fallback: set wallpaper via AppleScript."""
    script = f'tell application "System Events" to set picture of every desktop to "{image_path}"'
    try:
        subprocess.run(["osascript", "-e", script], check=True, capture_output=True)
        return True
    except subprocess.CalledProcessError as e:
        print(f"[ERROR] osascript failed: {e.stderr}")
        return False


def _get_monitor_count_macos() -> int:
    """Get monitor count on macOS."""
    try:
        import AppKit

        count = len(AppKit.NSScreen.screens())
        print(f"[DEBUG] Detected {count} monitors (macOS)")
        return count
    except ImportError:
        try:
            result = subprocess.run(
                [
                    "osascript",
                    "-e",
                    'tell application "System Events" to count of desktops',
                ],
                capture_output=True,
                text=True,
                check=True,
            )
            return int(result.stdout.strip())
        except Exception as e:
            print(f"[DEBUG] Failed to detect monitor count on macOS: {e}")
            return 1


def _set_wallpaper_multi_monitor_macos(image_paths: list) -> bool:
    """Set per-screen wallpapers on macOS."""
    if not image_paths:
        return False

    try:
        import AppKit
        import Foundation

        workspace = AppKit.NSWorkspace.sharedWorkspace()
        screens = AppKit.NSScreen.screens()
        options = {
            AppKit.NSWorkspaceDesktopImageScalingKey: AppKit.NSImageScaleProportionallyUpOrDown,
            AppKit.NSWorkspaceDesktopImageAllowClippingKey: True,
        }

        for i, screen in enumerate(screens):
            path = Path(image_paths[i % len(image_paths)]).resolve()
            if not path.exists():
                print(f"[ERROR] File does not exist: {path}")
                continue
            file_url = Foundation.NSURL.fileURLWithPath_(str(path))
            result, error = workspace.setDesktopImageURL_forScreen_options_error_(
                file_url, screen, options, None
            )
            if not result:
                print(f"[ERROR] Failed to set wallpaper on screen {i}: {error}")
                return False
        return True
    except ImportError:
        # Fallback: set all screens to first image via osascript
        return _set_wallpaper_macos_osascript(Path(image_paths[0]).resolve())


# --- Service Management ---

SERVICE_NAME_LINUX = "clockwork-orange.service"
SERVICE_NAME_WINDOWS = "ClockworkOrangeService"


def get_service_name():
    if IS_WINDOWS:
        return SERVICE_NAME_WINDOWS
    elif IS_MACOS:
        return None
    else:
        return SERVICE_NAME_LINUX


def service_is_active() -> str:
    """Returns 'active', 'inactive', 'failed', or 'unknown'."""
    if IS_WINDOWS or IS_MACOS:
        return "inactive"
    else:
        return _service_is_active_linux()


def service_get_status_details() -> str:
    if IS_WINDOWS:
        return "Windows mode uses System Tray app, not a background service."
    elif IS_MACOS:
        return "macOS mode uses the GUI app with system tray. No background service."
    else:
        return _service_get_status_details_linux()


def service_start():
    if IS_WINDOWS or IS_MACOS:
        pass
    else:
        return _service_start_linux()


def service_stop():
    if IS_WINDOWS or IS_MACOS:
        pass
    else:
        return _service_stop_linux()


def service_restart():
    if IS_WINDOWS or IS_MACOS:
        pass
    else:
        return _service_restart_linux()


def service_install(base_path: Path):
    if IS_WINDOWS:
        print("Service installation is not used on Windows. Use the Tray App.")
    elif IS_MACOS:
        print("Service installation is not used on macOS. Use the GUI app.")
    else:
        return _service_install_linux(base_path)


def service_uninstall():
    if IS_WINDOWS or IS_MACOS:
        pass
    else:
        return _service_uninstall_linux()


def service_get_logs() -> str:
    if IS_WINDOWS:
        return "Check console output or %TEMP% for logs."
    elif IS_MACOS:
        return "Check the Activity Log in the GUI."
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
        # Both macOS and Linux use fcntl file locking
        return _acquire_lock_posix(app_id)


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


def _acquire_lock_posix(app_id: str) -> bool:
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
