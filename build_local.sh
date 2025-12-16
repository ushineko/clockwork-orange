#!/bin/bash
set -e

echo "Building package from local source..."

# Create a temporary PKGBUILD
cp PKGBUILD PKGBUILD.local

# Replace the source URL with the local directory
# We use ${PWD} to get the absolute path
sed -i "s|source=(\"git+https://github.com/ushineko/clockwork-orange.git\")|source=(\"git+file://${PWD}\")|g" PKGBUILD.local

echo "Modified source to: git+file://${PWD}"

# Run makepkg using the local PKGBUILD
# -f: Force overwrite built package
# -p: Specify buildfile
makepkg -f -p PKGBUILD.local

# Cleanup
echo "Cleaning up..."
rm PKGBUILD.local
rm -rf src/ clockwork-orange/

echo "Build complete."
