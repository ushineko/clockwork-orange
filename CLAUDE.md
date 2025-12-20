# Project: Clockwork Orange

A Python-based wallpaper manager for KDE Plasma 6.

## Commands
*   **Run**: `./clockwork-orange.py` (CLI) or `./clockwork-orange.py --gui` (GUI)
*   **Lint/Format**: `black .` and `isort .` (Project adheres to Black code style)
*   **Build Arch**: `./build_local.sh` (wraps `makepkg`)
*   **Build Debian**: `./build_deb.sh` (wraps `dpkg-buildpackage`)
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
*   **Arch Linux**: `PKGBUILD` located in root (symlinked or moved). Deps: `pacman` packages.
*   **Debian**: `debian/` directory. Deps: `dpkg-dev`, `debhelper`. `build_deb.sh` handles dynamic `changelog` injection.
*   **CI/CD**: `.github/workflows/build_package.yml`
    *   Builds both Arch (container) and Debian (ubuntu-latest).
    *   Tags (`v*`) trigger a Release with both artifacts.

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
