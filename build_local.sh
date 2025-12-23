#!/bin/bash
# Build the Arch Linux package locally for testing

# Clean previous builds
rm -rf pkg/ src/ *.pkg.tar.zst clockwork-orange-git-*.pkg.tar.zst

# Generate icons
python3 create_logo.py

# Build package
# -s: install dependencies
# -i: install package
# -c: clean up
# -f: force overwrite
# --noconfirm: do not ask for confirmation
echo "Building package..."
makepkg -f --noconfirm

echo "Build complete!"
