#!/bin/bash
set -e

# Ensure we are in the project root
cd "$(dirname "$0")"

# Calculate version (Same logic as Arch PKGBUILD)
# Format: 0.r<count>.<sha> (Debian requires starting with digit)
REV_COUNT=$(git rev-list --count HEAD)
SHORT_SHA=$(git rev-parse --short HEAD)
PKGVER="0.r${REV_COUNT}.${SHORT_SHA}"

echo "Detected version: ${PKGVER}"

# Update debian/changelog with new version
# We replace the first line to update the version of the "unstable" release
# Format: clockwork-orange (1.0.0-1) unstable; urgency=medium
sed -i "1s/clockwork-orange (.*) unstable/clockwork-orange (${PKGVER}-1) unstable/" debian/changelog

# Check for build tool
if ! command -v dpkg-buildpackage &> /dev/null; then
    echo "Error: dpkg-buildpackage not found. Install dpkg-dev and debhelper."
    exit 1
fi

# Build package
# -us -uc: Do not sign source or changes (no GPG key needed for local build)
# -b: Build binary-only
# -d: Do not check build dependencies (useful on Arch where dpkg database is empty)
dpkg-buildpackage -us -uc -b -d

echo "Build complete."
ls -l ../clockwork-orange_*.deb
