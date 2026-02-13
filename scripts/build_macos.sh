#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

echo "Cleaning previous builds..."
rm -rf build dist

# Generate version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "Unknown")
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
FULL_VERSION="${VERSION}-${BRANCH}"
echo "$FULL_VERSION" > version.txt
echo "Detected version: $FULL_VERSION"

echo "=== Building Clockwork Orange for macOS ==="

# Verify framework build (required for .app bundles)
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
    --noconfirm \
    --clean \
    --onedir \
    --windowed \
    --name "Clockwork Orange" \
    --icon gui/icons/clockwork-orange.icns \
    --add-data "version.txt:." \
    --add-data "README.md:." \
    --add-data "img:img" \
    --add-data "plugins:plugins" \
    --add-data "gui/icons:gui/icons" \
    --collect-all gui \
    --collect-all plugins \
    --hidden-import AppKit \
    --hidden-import Foundation \
    --hidden-import objc \
    --hidden-import PyObjCTools \
    --hidden-import watchdog \
    --hidden-import watchdog.events \
    --hidden-import watchdog.observers \
    --hidden-import watchdog.observers.api \
    --hidden-import watchdog.observers.fsevents \
    --hidden-import watchdog.observers.polling \
    --hidden-import watchdog.utils \
    --hidden-import watchdog.utils.bricks \
    --hidden-import watchdog.utils.delayed_queue \
    --hidden-import watchdog.utils.dirsnapshot \
    --hidden-import watchdog.utils.event_debouncer \
    --hidden-import watchdog.utils.patterns \
    --hidden-import watchdog.utils.platform \
    clockwork-orange.py

if [ $? -eq 0 ]; then
    echo ""
    echo "Build successful! App is at: dist/Clockwork Orange.app"
    echo ""
    echo "Test with:"
    echo "  open 'dist/Clockwork Orange.app'"
    echo "  # or"
    echo "  'dist/Clockwork Orange.app/Contents/MacOS/Clockwork Orange' --self-test"

    # Clean up temporary version file
    rm -f version.txt
else
    echo "Build failed!"
    exit 1
fi
