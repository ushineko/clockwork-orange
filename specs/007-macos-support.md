# Spec 007: macOS Platform Support

**Status**: COMPLETE
**Target**: macOS 13+ (Ventura and later), Apple Silicon and Intel

## Overview

Add macOS as a third supported platform alongside Linux (KDE Plasma 6) and Windows 10/11. macOS provides native APIs for per-screen wallpaper management via `AppKit.NSWorkspace` (accessible through `pyobjc`).

**Execution model**: Same as Windows — the GUI must be running for wallpaper cycling. The app lives in the system tray (macOS menu bar) and cycles wallpapers via an internal timer. No background service (no `launchd` agent).

**Distribution**: PyInstaller `.app` bundle, similar to the Windows `.exe` build.

## Environment Findings

Probed on macOS Tahoe 26.2 (Darwin 25.2.0, Apple M1 Max):

| Capability | Method | Verified |
|------------|--------|----------|
| Set desktop wallpaper (per-screen) | `NSWorkspace.setDesktopImageURL_forScreen_options_error_()` via pyobjc | Yes |
| Read current wallpaper | `NSWorkspace.desktopImageURLForScreen_()` via pyobjc | Yes |
| Set wallpaper (fallback) | `osascript` AppleScript: `tell "System Events" to set picture of desktop N` | Yes |
| Multi-monitor enumeration | `AppKit.NSScreen.screens()` | Yes (2 logical screens detected) |
| Lock screen image location | `/Library/Caches/Desktop Pictures/<UUID>/lockscreen.png` | Yes (requires sudo) |
| Python | 3.13.9 available | Yes |
| pyobjc | 11.1 (pyobjc-core, pyobjc-framework-cocoa) | Yes |
| PyQt6 | Not installed (needs `pip install`) | -- |
| Homebrew | Available at `/opt/homebrew/bin/brew` | Yes |

### Python Interpreter Survey

Three interpreters found on this system:

| Interpreter | Version | Framework Build | Suitable |
|-------------|---------|-----------------|----------|
| Homebrew `/opt/homebrew/bin/python3.13` | 3.13.9 | Yes (`Python`) | **Best candidate** |
| miniforge3 `~/miniforge3/bin/python3` | 3.13.9 | No | Not suitable — non-framework builds cannot produce proper `.app` bundles |
| Apple CLT `/usr/bin/python3` | 3.9.6 | Yes (`Python3`) | Too old (project requires 3.10+) |

**Selected interpreter**: Homebrew `python@3.13` (framework build, arm64 native, 3.13.9).

PyInstaller on macOS requires a **framework build** of Python to embed `Python.framework` into `.app` bundles. The Homebrew python@3.13 provides this at `/opt/homebrew/opt/python@3.13/Frameworks/Python.framework/Versions/3.13`. The miniforge3 interpreter lacks a framework build (`PYTHONFRAMEWORK` is empty) and would produce broken bundles.

## Requirements

### Functional Requirements

1. **Platform detection**: Detect macOS at runtime (`sys.platform == "darwin"`)
2. **Desktop wallpaper**: Set wallpaper using `NSWorkspace` API (per-screen support)
3. **Multi-monitor**: Set different wallpapers per screen via `NSScreen.screens()`
4. **Lock screen**: Not supported (requires root); return `False` with info message (same as Windows)
5. **GUI with tray**: PyQt6 GUI with macOS menu bar tray icon; wallpaper cycling handled by internal timer (same model as Windows)
6. **Activity Log**: Show `ActivityLogWidget` on macOS (same as Windows, no service controls)
7. **Config location**: Use `~/.config/clockwork-orange.yml` (same as Linux, for consistency)
8. **Instance locking**: Use file-based locking (same as Linux, `fcntl` works on macOS)
9. **No background service**: No `launchd` agent. App must be running for wallpaper cycling.
10. **`.app` bundle**: PyInstaller-built macOS `.app` that runs standalone without a Python installation.

### Technical Requirements

1. Add `IS_MACOS` / `is_macos()` detection to `platform_utils.py`
2. Implement macOS wallpaper backend using `pyobjc` (`AppKit.NSWorkspace`)
3. Implement macOS multi-monitor wallpaper using `NSScreen.screens()` enumeration
4. Add `pyobjc-core` and `pyobjc-framework-Cocoa` to requirements (macOS-only)
5. Update GUI to show Activity Log on macOS (same as Windows — no service controls)
6. Provide `osascript` fallback if `pyobjc` is not available
7. Service management functions return no-ops / informational stubs on macOS (same as Windows)
8. PyInstaller build script producing a working `.app` bundle

### Out of Scope (Future Work)

- Lock screen wallpaper changes (requires `sudo` or SIP-bypassing hacks)
- `launchd` background service (may add later; for now GUI must be running)
- Homebrew formula
- macOS native menu bar integration (beyond what PyQt6 provides automatically)
- Code signing and notarization (required for distribution outside dev machine, but not for this initial port)
- DMG packaging (drag-to-Applications installer)

## Implementation Plan

### Phase 0: Toolchain Validation (DO THIS FIRST)

**Goal**: Before writing any application code, verify that the build toolchain works end-to-end. This is a new platform port — we need confidence that the environment can produce working `.app` bundles before committing to the full implementation.

**Interpreter**: Homebrew `python@3.13` at `/opt/homebrew/bin/python3.13`

#### Step 0a: Create and validate the venv

```bash
cd /Users/nverenin/git/clockwork-orange
/opt/homebrew/bin/python3.13 -m venv .venv
source .venv/bin/activate

# Verify framework build carried through to venv
python3 -c "import sysconfig; print('PYTHONFRAMEWORK:', sysconfig.get_config_var('PYTHONFRAMEWORK'))"
# Expected: "Python" (non-empty = framework build)

python3 -c "import sys; print(sys.version, sys.executable)"
# Expected: 3.13.x, .venv/bin/python3
```

#### Step 0b: Install core dependencies

```bash
pip install PyQt6 pyobjc-core pyobjc-framework-Cocoa Pillow PyInstaller
```

Verify each:
```bash
python3 -c "from PyQt6.QtWidgets import QApplication; print('PyQt6 OK')"
python3 -c "import AppKit; print('AppKit OK')"
python3 -c "from PIL import Image; print('Pillow OK')"
python3 -c "import PyInstaller; print('PyInstaller', PyInstaller.__version__)"
```

#### Step 0c: Minimal PyInstaller `.app` test

Create a throwaway test script and verify PyInstaller can produce a working `.app`:

```python
# /tmp/test_macos_app.py
import sys
print(f"Hello from bundled app! Python {sys.version}")
print(f"Frozen: {getattr(sys, 'frozen', False)}")
try:
    import AppKit
    print(f"AppKit: OK")
    ws = AppKit.NSWorkspace.sharedWorkspace()
    screens = AppKit.NSScreen.screens()
    print(f"Screens: {len(screens)}")
except Exception as e:
    print(f"AppKit: FAILED - {e}")
try:
    from PyQt6.QtWidgets import QApplication
    print("PyQt6: OK")
except Exception as e:
    print(f"PyQt6: FAILED - {e}")
```

Build and run:
```bash
pyinstaller --onedir --windowed --name TestMacApp /tmp/test_macos_app.py
open dist/TestMacApp.app
# Or: dist/TestMacApp.app/Contents/MacOS/TestMacApp
```

**Validation criteria for Phase 0**:
- [ ] venv reports `PYTHONFRAMEWORK=Python` (framework build)
- [ ] PyQt6, pyobjc, Pillow, PyInstaller all import without error
- [ ] PyInstaller produces a `.app` bundle in `dist/`
- [ ] The `.app` launches and prints expected output
- [ ] AppKit and PyQt6 both work inside the frozen bundle
- [ ] The `.app` works when launched from Finder (not just terminal)

**If Phase 0 fails**: Stop and troubleshoot before proceeding. Possible issues:
- venv lost framework build → try `--copies` flag or use interpreter directly
- PyInstaller can't find `Python.framework` → check `PYTHONFRAMEWORK` config var
- pyobjc fails in frozen bundle → may need `--collect-all pyobjc` or `--hidden-import` flags
- PyQt6 fails in frozen bundle → check PyInstaller hooks for Qt

**Only proceed to Phase 1+ after Phase 0 passes.**

### Phase 1: Platform Detection and Constants

**File**: [platform_utils.py](platform_utils.py)

Add macOS detection alongside existing Windows/Linux:

```python
IS_WINDOWS = sys.platform == "win32"
IS_MACOS = sys.platform == "darwin"
IS_LINUX = sys.platform == "linux"

def is_windows():
    return IS_WINDOWS

def is_macos():
    return IS_MACOS

def is_linux():
    return IS_LINUX
```

Update all existing `if IS_WINDOWS: ... else:` branches to three-way dispatch:
`if IS_WINDOWS: ... elif IS_MACOS: ... else: (Linux)`

This affects every public function in `platform_utils.py`.

### Phase 2: Desktop Wallpaper (macOS)

**File**: [platform_utils.py](platform_utils.py)

Primary implementation using `pyobjc`:

```python
def _set_wallpaper_macos(image_path: Path) -> bool:
    """Set wallpaper on macOS using NSWorkspace."""
    try:
        import AppKit
        import Foundation

        workspace = AppKit.NSWorkspace.sharedWorkspace()
        file_url = Foundation.NSURL.fileURLWithPath_(str(image_path))
        options = {
            AppKit.NSWorkspaceDesktopImageScalingKey: AppKit.NSImageScaleProportionallyUpOrDown,
            AppKit.NSWorkspaceDesktopImageAllowClippingKey: True,
        }

        # Set on all screens
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
```

Fallback using `osascript`:

```python
def _set_wallpaper_macos_osascript(image_path: Path) -> bool:
    """Fallback: set wallpaper via AppleScript."""
    script = f'tell application "System Events" to set picture of every desktop to "{image_path}"'
    try:
        subprocess.run(["osascript", "-e", script], check=True, capture_output=True)
        return True
    except subprocess.CalledProcessError as e:
        print(f"[ERROR] osascript failed: {e.stderr}")
        return False
```

### Phase 3: Multi-Monitor Wallpaper (macOS)

**File**: [platform_utils.py](platform_utils.py)

```python
def get_monitor_count() -> int:
    # ... existing Windows/Linux ...
    elif IS_MACOS:
        try:
            import AppKit
            return len(AppKit.NSScreen.screens())
        except ImportError:
            # Fallback: osascript
            result = subprocess.run(
                ["osascript", "-e",
                 'tell application "System Events" to count of desktops'],
                capture_output=True, text=True, check=True
            )
            return int(result.stdout.strip())

def _set_wallpaper_multi_monitor_macos(image_paths: list) -> bool:
    """Set per-screen wallpapers on macOS."""
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
            path = image_paths[i % len(image_paths)]
            file_url = Foundation.NSURL.fileURLWithPath_(str(Path(path).resolve()))
            result, error = workspace.setDesktopImageURL_forScreen_options_error_(
                file_url, screen, options, None
            )
            if not result:
                print(f"[ERROR] Failed to set wallpaper on screen {i}: {error}")
                return False
        return True
    except ImportError:
        # Fallback: set all screens to first image via osascript
        return _set_wallpaper_macos_osascript(Path(image_paths[0]))
```

### Phase 4: Lock Screen (macOS)

**File**: [platform_utils.py](platform_utils.py)

Lock screen wallpaper on macOS requires writing to `/Library/Caches/Desktop Pictures/<UUID>/lockscreen.png`, which needs root privileges. Return `False` with info message (same approach as Windows):

```python
def _set_lockscreen_wallpaper_macos(image_path: Path) -> bool:
    print("[INFO] Lock screen wallpaper on macOS requires elevated privileges. Not supported.")
    return False
```

### Phase 5: Service Management Stubs (macOS)

**File**: [platform_utils.py](platform_utils.py)

macOS follows the Windows model: no background service, GUI must be running. All service functions return no-ops or informational messages:

```python
def _service_is_active_macos() -> str:
    return "inactive"

def _service_get_status_details_macos() -> str:
    return "macOS mode uses the GUI app with system tray. No background service."

def _service_start_macos():
    pass

def _service_stop_macos():
    pass

def _service_restart_macos():
    pass

def _service_install_macos(base_path: Path):
    print("Service installation is not used on macOS. Use the GUI app.")

def _service_uninstall_macos():
    pass

def _service_get_logs_macos() -> str:
    return "Check the Activity Log in the GUI."
```

### Phase 6: GUI Adjustments

**File**: [gui/main_window.py](gui/main_window.py)

Update `init_pages()` to show Activity Log on macOS (same as Windows):

```python
if platform_utils.is_linux():
    # Linux: Show Service Control (systemd)
    self.service_page = ServiceManagerWidget()
    self.add_page("Service Control", self.service_page, ...)
else:
    # Windows and macOS: Show Activity Log (GUI-based wallpaper cycling)
    from gui.activity_log import ActivityLogWidget
    self.service_page = ActivityLogWidget()
    self.add_page("Activity Log", self.service_page, ...)
```

**File**: [gui/main_window.py](gui/main_window.py) - `WallpaperWorker`

Fix the multi-monitor detection that currently assumes only Windows has multiple monitors:

```python
# Current (wrong):
monitor_count = platform_utils.get_monitor_count() if platform_utils.is_windows() else 1

# Fixed (all platforms support multi-monitor):
monitor_count = platform_utils.get_monitor_count()
```

This is actually a bug fix for Linux too — KDE multi-monitor already works via the CLI path, but the GUI worker was skipping it.

### Phase 7: Main Entry Point Updates

**File**: [clockwork-orange.py](clockwork-orange.py)

1. Remove the `import ctypes` at module level (fails on macOS; only needed on Windows)
2. Guard the `SetCurrentProcessExplicitAppUserModelID` call (already guarded by `sys.platform == "win32"`)
3. Update argparser description to mention macOS support
4. Update `load_config_file()` to not check `C:/Users/Public/clockwork_config.yml` on macOS
5. Guard `debug_lockscreen_config()` for Linux-only (KDE-specific)

### Phase 8: Instance Locking (macOS)

**File**: [platform_utils.py](platform_utils.py)

macOS supports `fcntl` (POSIX), so the existing `_acquire_lock_linux()` function works unchanged. Rename to `_acquire_lock_posix`:

```python
def acquire_instance_lock(app_id: str) -> bool:
    if IS_WINDOWS:
        return _acquire_lock_windows(app_id)
    else:
        # Both macOS and Linux use fcntl file locking
        return _acquire_lock_posix(app_id)
```

### Phase 9: Dependencies

**File**: [requirements.txt](requirements.txt)

Add platform markers to make dependencies platform-conditional:

```
PyQt6==6.10.1
Pillow==12.0.0
requests==2.32.5
PyYAML==6.0.3
watchdog==6.0.0
# Windows only:
pywin32==311; sys_platform == "win32"
pyinstaller==6.16.0; sys_platform == "win32"
screeninfo==0.8.1; sys_platform == "win32"
# macOS only:
pyobjc-core>=10.0; sys_platform == "darwin"
pyobjc-framework-Cocoa>=10.0; sys_platform == "darwin"
```

Note: `pyinstaller` is a build-time dependency, not a runtime one. It should be available in the dev venv but doesn't need a platform marker in `requirements.txt` (it's installed explicitly during build). If we want it in requirements for convenience, remove the Windows-only marker and make it cross-platform, or keep it out of requirements entirely and document it in the build scripts.

### Phase 10: Self-Test Updates

**File**: [clockwork-orange.py](clockwork-orange.py)

Update `--self-test` to test macOS-specific imports:

```python
if sys.platform == "darwin":
    macos_modules = ["AppKit", "Foundation", "objc"]
    for mod in macos_modules:
        try:
            __import__(mod)
            print(f"[OK] Import {mod}")
            results[mod] = True
        except ImportError as e:
            print(f"[FAIL] Import {mod}: {e}")
            results[mod] = False
```

### Phase 11: macOS Build Script

**New file**: `scripts/build_macos.sh`

Build script mirroring the Windows `build_windows.ps1` approach:

```bash
#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

echo "=== Clockwork Orange macOS Build ==="

# Ensure we're in the venv
if [ -z "$VIRTUAL_ENV" ]; then
    echo "Activating venv..."
    source .venv/bin/activate
fi

# Verify framework build
FRAMEWORK=$(python3 -c "import sysconfig; print(sysconfig.get_config_var('PYTHONFRAMEWORK'))")
if [ -z "$FRAMEWORK" ]; then
    echo "ERROR: Python is not a framework build. PyInstaller .app bundles require a framework build."
    echo "Use: /opt/homebrew/bin/python3.13 -m venv .venv"
    exit 1
fi

echo "Python: $(python3 --version)"
echo "Framework: $FRAMEWORK"
echo "Executable: $(python3 -c 'import sys; print(sys.executable)')"

# Build
pyinstaller \
    --name "Clockwork Orange" \
    --windowed \
    --onedir \
    --icon gui/icons/clockwork-orange.icns \
    --add-data "gui/icons:gui/icons" \
    --add-data "plugins:plugins" \
    --add-data "README.md:." \
    --collect-all pyobjc \
    --hidden-import AppKit \
    --hidden-import Foundation \
    --hidden-import objc \
    --hidden-import watchdog.observers \
    --hidden-import watchdog.observers.fsevents \
    --collect-all watchdog \
    clockwork-orange.py

echo ""
echo "Build complete: dist/Clockwork Orange.app"
echo ""
echo "Test with:"
echo "  open 'dist/Clockwork Orange.app'"
echo "  # or"
echo "  'dist/Clockwork Orange.app/Contents/MacOS/Clockwork Orange' --self-test"
```

**Note on icon**: The `.icns` file needs to be created from the existing PNG icons. This can be done with `iconutil` or `sips` during the build, or as a one-time conversion.

### Phase 12: CI/CD — GitHub Actions macOS Build

**Context**: The existing CI/CD pipeline in [.github/workflows/build_package.yml](.github/workflows/build_package.yml) builds all platforms and creates GitHub Releases on tags. It has:
- `build-arch` job (`ubuntu-latest` + archlinux container) → `.pkg.tar.zst`
- `build-deb` job (`ubuntu-latest`) → `.deb`
- `build-windows` job (`windows-latest` + `setup-python`) → `.exe`
- `release` job (downloads all artifacts, creates GitHub Release)

The Windows job uses `actions/setup-python@v5` to get Python, installs `requirements.txt`, runs the build script, runs `--self-test` on the result, and uploads the artifact.

#### The Framework Build Problem

`actions/setup-python@v5` does **not** provide a framework build of Python on macOS. This is a [known unresolved issue](https://github.com/actions/setup-python/issues/58) — the Python it installs lacks `PYTHONFRAMEWORK`, which PyInstaller needs to produce proper `.app` bundles.

Workaround: Use **Homebrew Python** on the macOS runner instead of `setup-python`. GitHub Actions `macos-latest` runners come with Homebrew pre-installed.

#### New Workflow File: `.github/workflows/build_macos.yml`

Standalone macOS build workflow (mirroring `build_windows.yml`):

```yaml
name: Build macOS

on:
  push:
    branches: [ "master", "main" ]
    tags:
      - 'v*'
  pull_request:
    branches: [ "master", "main" ]
  workflow_dispatch:

jobs:
  build:
    name: Build macOS App
    runs-on: macos-latest

    steps:
    - name: Checkout Code
      uses: actions/checkout@v4
      with:
        fetch-depth: 0

    # NOTE: We do NOT use actions/setup-python here.
    # setup-python does not provide a framework build on macOS,
    # which PyInstaller requires for .app bundles.
    # See: https://github.com/actions/setup-python/issues/58
    - name: Install Python via Homebrew
      run: |
        brew install python@3.13
        echo "/opt/homebrew/opt/python@3.13/libexec/bin" >> $GITHUB_PATH

    - name: Create venv
      run: |
        /opt/homebrew/bin/python3.13 -m venv .venv
        source .venv/bin/activate
        echo "$VIRTUAL_ENV/bin" >> $GITHUB_PATH

    - name: Verify Framework Build
      run: |
        source .venv/bin/activate
        FRAMEWORK=$(python3 -c "import sysconfig; print(sysconfig.get_config_var('PYTHONFRAMEWORK'))")
        echo "PYTHONFRAMEWORK=$FRAMEWORK"
        if [ -z "$FRAMEWORK" ]; then
          echo "::error::Python is not a framework build. Cannot produce .app bundles."
          exit 1
        fi
        python3 --version

    - name: Install Dependencies
      run: |
        source .venv/bin/activate
        python -m pip install --upgrade pip
        pip install pyobjc-core pyobjc-framework-Cocoa
        pip install PyQt6 Pillow requests PyYAML watchdog PyInstaller

    - name: Build macOS App
      run: |
        source .venv/bin/activate
        bash scripts/build_macos.sh

    - name: Verify Build (Self-Test)
      run: |
        echo "Running self-test on built .app..."
        EXIT_CODE=0
        "dist/Clockwork Orange.app/Contents/MacOS/Clockwork Orange" --self-test || EXIT_CODE=$?
        echo "Self-test exit code: $EXIT_CODE"
        if [ "$EXIT_CODE" -ne 0 ]; then
          echo "::error::Self-test FAILED!"
          exit 1
        fi
        echo "Self-test PASSED!"

    - name: Package App as ZIP
      run: |
        cd dist
        zip -r "Clockwork-Orange-macOS.zip" "Clockwork Orange.app"

    - name: Upload Artifact
      uses: actions/upload-artifact@v4
      with:
        name: clockwork-orange-macos
        path: dist/Clockwork-Orange-macOS.zip
        if-no-files-found: error
```

#### Update to `build_package.yml`

Add a `build-macos` job and include it in the release:

```yaml
  build-macos:
    name: Build macOS App
    runs-on: macos-latest
    permissions:
      contents: write

    steps:
    - name: Checkout Code
      uses: actions/checkout@v4
      with:
        fetch-depth: 0

    - name: Install Python via Homebrew
      run: |
        brew install python@3.13
        echo "/opt/homebrew/opt/python@3.13/libexec/bin" >> $GITHUB_PATH

    - name: Create venv
      run: |
        /opt/homebrew/bin/python3.13 -m venv .venv
        source .venv/bin/activate
        echo "$VIRTUAL_ENV/bin" >> $GITHUB_PATH

    - name: Verify Framework Build
      run: |
        source .venv/bin/activate
        FRAMEWORK=$(python3 -c "import sysconfig; print(sysconfig.get_config_var('PYTHONFRAMEWORK'))")
        echo "PYTHONFRAMEWORK=$FRAMEWORK"
        if [ -z "$FRAMEWORK" ]; then
          echo "::error::Python is not a framework build."
          exit 1
        fi

    - name: Install Dependencies
      run: |
        source .venv/bin/activate
        python -m pip install --upgrade pip
        pip install pyobjc-core pyobjc-framework-Cocoa
        pip install PyQt6 Pillow requests PyYAML watchdog PyInstaller

    - name: Build macOS App
      run: |
        source .venv/bin/activate
        bash scripts/build_macos.sh

    - name: Verify Build (Self-Test)
      run: |
        "dist/Clockwork Orange.app/Contents/MacOS/Clockwork Orange" --self-test

    - name: Package App as ZIP
      run: |
        cd dist
        zip -r "Clockwork-Orange-macOS.zip" "Clockwork Orange.app"

    - name: Upload macOS Artifact
      uses: actions/upload-artifact@v4
      with:
        name: macos-app
        path: dist/Clockwork-Orange-macOS.zip
```

Update the `release` job:

```yaml
  release:
    name: Create Release
    needs: [build-arch, build-deb, build-windows, build-macos]  # <-- add build-macos
    if: startsWith(github.ref, 'refs/tags/')
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
    # ... existing download steps ...

    - name: Download macOS Artifacts
      uses: actions/download-artifact@v4
      with:
        name: macos-app
        path: dist

    # ... existing release step (dist/* already picks up the zip) ...
```

#### CI/CD Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use Homebrew Python, not `setup-python` | `setup-python` does not provide framework builds on macOS ([issue #58](https://github.com/actions/setup-python/issues/58)); PyInstaller requires framework builds for `.app` bundles |
| `macos-latest` runner | Provides Apple Silicon (M1/M2) runner with Homebrew pre-installed; currently macOS 14 Sonoma |
| ZIP the `.app` for upload | `.app` is a directory, not a single file; `upload-artifact` and GitHub Releases need a single archive |
| `--self-test` verification | Same pattern as Windows build; catches missing imports and broken bundles before release |
| Separate `build_macos.yml` | Mirrors `build_windows.yml` for standalone build/test; the combined workflow in `build_package.yml` handles releases |
| arm64 only (not universal2) | `macos-latest` is arm64; universal2 builds require additional complexity and are out of scope for initial port |

#### Self-Test Limitation in CI

The `--self-test` includes a network connectivity check (`requests.get("https://www.google.com")`). This should work on GitHub Actions macOS runners (they have internet access). However, the wallpaper-setting APIs cannot be meaningfully tested in CI (headless environment, no display). The self-test validates imports and plugin loading, which is sufficient for CI.

## Files Changed

| File | Change Type | Description |
|------|-------------|-------------|
| [platform_utils.py](platform_utils.py) | Modified | Add macOS wallpaper, multi-monitor, lock screen (stub), service (stubs), instance locking |
| [clockwork-orange.py](clockwork-orange.py) | Modified | Guard Windows-specific imports; update self-test; update argparser description |
| [gui/main_window.py](gui/main_window.py) | Modified | Show Activity Log on macOS (same as Windows); fix multi-monitor detection for all platforms |
| [requirements.txt](requirements.txt) | Modified | Add platform markers; add pyobjc dependencies for macOS |
| `scripts/build_macos.sh` | New | PyInstaller build script for macOS `.app` bundle |
| `.github/workflows/build_macos.yml` | New | Standalone GitHub Actions workflow for macOS builds |
| `.github/workflows/build_package.yml` | Modified | Add `build-macos` job; add macOS artifact to release |

## Acceptance Criteria

### Toolchain (Phase 0)
- [x] venv created from Homebrew python3.13 with `PYTHONFRAMEWORK=Python`
- [x] PyQt6, pyobjc, Pillow, PyInstaller all install and import in venv
- [x] Minimal test `.app` builds, launches, and runs correctly (including from Finder)
- [x] AppKit and PyQt6 both work inside frozen test bundle

### Application
- [x] `sys.platform == "darwin"` correctly detected; `is_macos()` returns `True`
- [x] `set_wallpaper()` changes desktop wallpaper on macOS
- [x] `set_wallpaper_multi_monitor()` sets different wallpapers per screen on macOS
- [x] `get_monitor_count()` returns correct count on macOS
- [x] `set_lockscreen_wallpaper()` returns `False` with informational message on macOS
- [x] Service functions return no-op stubs on macOS (same pattern as Windows)
- [x] `acquire_instance_lock()` works on macOS (fcntl)
- [x] GUI launches on macOS with PyQt6
- [x] GUI shows Activity Log on macOS (not Service Control)
- [ ] System tray / menu bar icon works on macOS (requires manual GUI testing)
- [ ] Wallpaper cycling works via GUI timer on macOS (requires manual GUI testing)
- [x] Multi-monitor wallpaper works from GUI worker on macOS
- [x] `--self-test` checks macOS-specific imports (AppKit, Foundation)
- [x] `osascript` fallback works when pyobjc is not installed
- [x] No import errors on macOS (guarded Windows-specific imports like `ctypes.windll`, `winreg`)
- [x] Config file at `~/.config/clockwork-orange.yml` used on macOS
- [x] `requirements.txt` has platform markers for platform-specific packages

### Build
- [x] `scripts/build_macos.sh` produces `dist/Clockwork Orange.app`
- [x] `.app` launches from Finder
- [x] `--self-test` passes inside frozen `.app` bundle
- [x] All plugins load inside frozen bundle
- [ ] Wallpaper setting works from frozen bundle (requires manual GUI testing)

### CI/CD
- [x] `.github/workflows/build_macos.yml` runs on `macos-latest`
- [x] CI uses Homebrew Python (not `setup-python`) with verified framework build
- [ ] CI builds `.app` and runs `--self-test` successfully (requires push to verify)
- [x] CI uploads `Clockwork-Orange-macOS.zip` as artifact
- [x] `build_package.yml` includes `build-macos` job
- [x] Release job downloads macOS artifact and includes it in GitHub Release
- [ ] Tagged push produces release with macOS `.zip` alongside Windows `.exe` and Linux packages (requires tag push to verify)

## Testing

### Manual Test Cases

1. **Phase 0 validation**: Run all toolchain checks before writing app code
2. Run `python3 clockwork-orange.py --self-test` on macOS
3. Run `python3 clockwork-orange.py -f /path/to/image.jpg` — verify wallpaper changes
4. Run `python3 clockwork-orange.py -d /path/to/wallpapers/` — verify random selection works
5. Run `python3 clockwork-orange.py -d /path/to/wallpapers/ -w 10` — verify cycling works
6. Run `python3 clockwork-orange.py --gui` — verify GUI launches with Activity Log tab
7. Verify system tray / menu bar icon appears and context menu works
8. Verify wallpaper cycling via GUI timer (close-to-tray, cycling continues)
9. Test multi-monitor: disconnect/reconnect external display, verify `get_monitor_count()` updates
10. Test with pyobjc uninstalled: verify osascript fallback activates
11. Run `scripts/build_macos.sh` — verify `.app` is produced
12. Launch `.app` from Finder — verify it runs and sets wallpapers
13. Run `.app/Contents/MacOS/Clockwork Orange --self-test` — verify all imports pass

### Automated Tests

- Add macOS platform detection test to existing test suite
- Test that macOS-specific imports are guarded (no `ImportError` on other platforms)

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| venv loses framework build | Low | High | Verify in Phase 0; use `--copies` flag if needed |
| PyInstaller can't bundle pyobjc | Medium | High | Test in Phase 0 with minimal app; use `--collect-all` |
| pyobjc not installed (end user) | N/A | N/A | Bundled in `.app`; osascript fallback for dev installs |
| NSWorkspace API changes in future macOS | Low | Medium | osascript fallback; pin minimum macOS version |
| PyQt6 rendering differences on macOS | Medium | Low | Test GUI elements; macOS-specific style adjustments if needed |
| System tray behavior on macOS | Low | Low | PyQt6 handles menu bar tray natively; test on macOS |
| `setup-python` lacks framework build | Confirmed | High | Use Homebrew Python on CI runner instead |
| `macos-latest` runner changes | Low | Medium | Pin to `macos-14` if `macos-latest` moves to a problematic version |
| CI self-test fails (headless) | Low | Low | Self-test validates imports/plugins, not wallpaper APIs; should work headless |

## Implementation Order

1. **Phase 0**: Toolchain validation (venv, deps, minimal `.app` build) — **GATE: must pass before continuing**
2. Phase 1: Platform detection (IS_MACOS, is_macos(), is_linux())
3. Phase 8: Instance locking (rename _acquire_lock_linux to _acquire_lock_posix)
4. Phase 2: Desktop wallpaper (pyobjc + osascript fallback)
5. Phase 3: Multi-monitor wallpaper
6. Phase 4: Lock screen (stub)
7. Phase 5: Service management (stubs, matching Windows pattern)
8. Phase 9: Dependencies (requirements.txt with platform markers)
9. Phase 10: Self-test (macOS-specific imports)
10. Phase 7: Main entry point updates (guard ctypes, update argparser)
11. Phase 6: GUI adjustments (Activity Log on macOS, fix multi-monitor in worker)
12. Phase 11: macOS build script (scripts/build_macos.sh)
13. Phase 12: CI/CD (build_macos.yml + update build_package.yml)
14. Final validation: full `.app` build with real application code, push tag to verify release pipeline

## Notes

- `com.apple.desktop` defaults domain is deprecated on modern macOS; wallpaper management is now via `NSWorkspace` or `com.apple.wallpaper` plist
- The `NSWorkspace.setDesktopImageURL_forScreen_options_error_()` API provides per-screen wallpaper control and is the correct modern API
- macOS lock screen image lives at `/Library/Caches/Desktop Pictures/<UUID>/lockscreen.png` and requires root to modify — not viable for a user-space app without `sudo`
- File-based locking via `fcntl` works identically on macOS and Linux
- macOS follows the Windows execution model: GUI must be running, wallpaper cycling via internal QTimer, app minimizes to system tray (menu bar on macOS)
- PyInstaller requires a **framework build** of Python on macOS to produce `.app` bundles — the Homebrew `python@3.13` provides this, miniforge3 does not
- The `.venv` must be created from the Homebrew interpreter to inherit the framework build property
- `.icns` icon file needed for the `.app` bundle — convert from existing PNGs using `iconutil` or `sips`
- `actions/setup-python@v5` does **not** provide framework builds on macOS — this is a [known unresolved issue](https://github.com/actions/setup-python/issues/58); the CI must use Homebrew Python instead
- `macos-latest` currently resolves to macOS 14 Sonoma on Apple Silicon (M1); this produces arm64-only builds
- The `.app` bundle is a directory; it must be zipped for GitHub Releases artifact upload
- CI self-test runs in a headless environment — wallpaper-setting APIs won't actually change a desktop, but imports and plugin loading are validated
