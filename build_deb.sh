#!/bin/bash
set -e

# Ensure we are in the project root
cd "$(dirname "$0")"

# Calculate version from .tag file
# Format: <tag>.<count>.g<sha> (Debian requires starting with digit)
# e.g. 2.2.6.48.g861ecad
_TAG=$(cat .tag 2>/dev/null || echo "v0.0.0")
CLEAN_TAG=${_TAG#v}

REV_COUNT=$(git rev-list --count HEAD)
SHORT_SHA=$(git rev-parse --short HEAD)

PKGVER="${CLEAN_TAG}.${REV_COUNT}.g${SHORT_SHA}"

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
# -d: Do not check build dependencies (useful on Arch where dpkg database is empty)
dpkg-buildpackage -us -uc -b -d

echo "Build complete. Moving artifacts to current directory..."
mv ../clockwork-orange_*.deb .
mv ../clockwork-orange_*.changes .
mv ../clockwork-orange_*.buildinfo . 2>/dev/null || true

ls -l *.deb
