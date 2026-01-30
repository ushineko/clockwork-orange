# Project: Clockwork Orange

A multiplatform, Python-based wallpaper manager.

Supports:
*   **Linux**: KDE Plasma 6
*   **Windows**: Windows 10/11

Linux Distros:
*   **Arch Linux**: `clockwork-orange-git` (AUR)
*   **Debian**: `clockwork-orange` (Debian)

## Commands
*   **Run**: `./clockwork-orange.py` (CLI) or `./clockwork-orange.py --gui` (GUI)
*   **Lint/Format**: `black .` and `isort .` (Project adheres to Black code style)
*   **Build Arch**: `./build_local.sh` (wraps `makepkg`)
*   **Build Debian**: `./build_deb.sh` (wraps `dpkg-buildpackage`)
*   **Update AUR**: `./scripts/update_aur.sh "Update to vX.Y.Z"` (Pushes PKGBUILD to AUR)
*   **Gen Docs**: `python3 docs/generate_screenshots.py` (Requires GUI env)
*   **Service**:
    *   Install: `cp clockwork-orange.service ~/.config/systemd/user/`
    *   Logs: `journalctl --user -u clockwork-orange.service -f`

## Architecture
*   **Core**: `clockwork-orange.py` contains the main logic, argument parsing, and KDE interaction (`qdbus6`, `kwriteconfig6`).
*   **GUI**: `gui/` directory.
    *   `main_window.py`: Application entry point, sidebar handling.
    *   `plugins_tab.py`: Dynamic plugin UI configuration.
    *   **Framework**: PyQt6.
*   **Plugins**: `plugins/` directory.
    *   `plugin_manager.py`: Handles loading and execution.
    *   Structure: Plugins return an iterator/generator yielding images or logs.

## Packaging
*   **Versioning**: Dynamic. Format `0.r<count>.<short_sha>` (e.g., `0.r30.abc1234`).
    *   Calculated via `git rev-list --count HEAD` and `git rev-parse --short HEAD`.
    *   **Injection**: Packages generate a `version.txt` file in the install directory (e.g., `/usr/lib/clockwork-orange/`). The GUI checks this file first to report the version, falling back to git if missing (dev mode).
*   **Arch Linux**: `PKGBUILD` located in root. Deps: `pacman` packages.
    *   **AUR**: Package `clockwork-orange-git` on AUR. Update via `./scripts/update_aur.sh "message"`.
    *   Script clones AUR repo to `.aur-repo/`, copies PKGBUILD, generates `.SRCINFO`, commits and pushes.
*   **Debian**: `debian/` directory. Deps: `dpkg-dev`, `debhelper`. `build_deb.sh` handles dynamic `changelog` injection.
*   **CI/CD**: `.github/workflows/build_package.yml`
    *   Builds Arch (container), Debian (ubuntu-latest), and Windows (windows-latest).
    *   Tags (`v*`) trigger a Release with all three artifacts.

## Documentation
*   `README.md`: Main entry point.
*   `GUI.md`: Generated tour. **Do not edit manually.** Run `docs/generate_screenshots.py`.
*   **Automation**: The generator script uses PyQt introspection to find text fields (`QLineEdit`, `QTextEdit`) and applies a Gaussian Blur to redact sensitive info.

## Conventions
*   **Style**: Strict Black formatting.
*   **Imports**: Sorted by `isort`.
*   **Paths**: Use `pathlib.Path` over `os.path`.
*   **PyQt**: Use `PyQt6`.
*   **KDE**: Target Plasma 6 APIs (`qdbus6`). Do not support Plasma 5.
*   **Windows**: Target Windows 10/11. Use `pywin32` for API access.

## Windows Implementation (Divergence)
The Windows version fundamentally diverge from Linux in several key areas due to OS architecture:

*   **Service Model**: 
    *   **Linux**: Uses systemd user service (`clockwork-orange.service`).
    *   **Windows**: Uses the GUI/Tray app for background execution.
    *   *Reason*: Windows Services run in Session 0 (isolation) and cannot interact with the interactive user desktop or change wallpapers via standard APIs (`SystemParametersInfo`). While a service wrapper exists (`win32service`), it is functionally limited to logging/downloading and is **not recommended** for desktop use.
*   **Wallpaper Engine**:
    *   **Linux**: DBus calls to Plasma Shell (`org.kde.PlasmaShell`).
    *   **Windows (Single)**: `SystemParametersInfoW` (Standard API).
    *   **Windows (Multi-Monitor)**: Custom engine using `Pillow`.
        *   Stitches individual monitor wallpapers into a single "Spanned" canvas based on exact monitor geometry (`EnumDisplayMonitors`).
        *   Sets registry `WallpaperStyle` to "Span" (22).
*   **Locking**:
    *   **Linux**: `fcntl` file locking (`/tmp/clockwork-orange.lock`).
    *   **Windows**: Named Mutex (`CreateMutexW` via `kernel32`).
*   **Building**:
    *   **Linux**: `makepkg` / `dpkg-deb`.
    *   **Windows**: `PyInstaller` (OneFile). Requires careful DLL handling for `_ctypes`, `libssl`, etc. (see `scripts/build_windows_production.ps1` dynamic discovery logic).
