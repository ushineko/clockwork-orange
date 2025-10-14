#!/bin/bash
# Install desktop entry for Clockwork Orange

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$SCRIPT_DIR"

echo "Installing Clockwork Orange desktop entry..."
echo "Project directory: $PROJECT_DIR"

# Create applications directory if it doesn't exist
mkdir -p ~/.local/share/applications

# Create desktop entry with correct paths
cat > ~/.local/share/applications/clockwork-orange.desktop << EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Clockwork Orange
Comment=Wallpaper Manager for KDE Plasma 6
Exec=python3 $PROJECT_DIR/clockwork-orange.py --gui
Icon=$PROJECT_DIR/gui/icons/clockwork-orange-128x128.png
Path=$PROJECT_DIR
Terminal=false
StartupNotify=true
Categories=Graphics;Photography;System;
Keywords=wallpaper;desktop;background;lockscreen;kde;plasma;
StartupWMClass=clockwork-orange
MimeType=
EOF

# Make it executable
chmod +x ~/.local/share/applications/clockwork-orange.desktop

# Update desktop database
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database ~/.local/share/applications/
    echo "Desktop database updated."
else
    echo "Warning: update-desktop-database not found. You may need to log out and back in."
fi

echo "Desktop entry installed successfully!"
echo "You can now find 'Clockwork Orange' in your application launcher."
echo "You can also pin it to your taskbar by right-clicking on the application."
