#!/usr/bin/env bash
#
# Builds the Debian package from pre-built binaries with a staged tree and
# dpkg-deb (spec 010 R8.3). No debhelper, no dh-golang: the Makefile has already
# produced bin/clockwork-orange and bin/clockwork-orange-gui with the version
# stamped in, so all that is left is laying files out and writing control.
#
# Version: <.tag minus v>.<rev count>.g<short sha>, the same shape the Python
# build_deb.sh produced.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

for bin in bin/clockwork-orange bin/clockwork-orange-gui; do
    [ -x "$bin" ] || { echo "missing $bin; run 'make build build-gui' first" >&2; exit 1; }
done
command -v dpkg-deb >/dev/null || { echo "dpkg-deb not found" >&2; exit 1; }

TAG="$(tr -d 'v[:space:]' < .tag)"
REV="$(git rev-list --count HEAD)"
SHA="$(git rev-parse --short HEAD)"
VERSION="${TAG}.${REV}.g${SHA}"
ARCH="$(dpkg --print-architecture)"
PKG="clockwork-orange_${VERSION}_${ARCH}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

install -Dm755 bin/clockwork-orange "$STAGE/usr/bin/clockwork-orange"
install -Dm755 bin/clockwork-orange-gui "$STAGE/usr/bin/clockwork-orange-gui"
install -Dm644 packaging/io.ushineko.clockwork-orange.desktop \
    "$STAGE/usr/share/applications/io.ushineko.clockwork-orange.desktop"
for res in 16 32 48 64 128 256 512; do
    install -Dm644 "packaging/icons/clockwork-orange-${res}x${res}.png" \
        "$STAGE/usr/share/icons/hicolor/${res}x${res}/apps/clockwork-orange.png"
done
install -Dm644 internal/platform/clockwork-orange.service \
    "$STAGE/usr/lib/systemd/user/clockwork-orange.service"
install -Dm644 LICENSE "$STAGE/usr/share/doc/clockwork-orange/copyright"
install -Dm644 README.md "$STAGE/usr/share/doc/clockwork-orange/README.md"

mkdir -p "$STAGE/DEBIAN"
# qdbus6 and kwriteconfig6 come from qt6-tools / kconfig on Debian and Ubuntu
# (package names verified for Ubuntu 24.04: qt6-tools-dev-tools ships qdbus6,
# kconfig ships kwriteconfig6). The Fyne runtime set matches nmsbonker's.
cat > "$STAGE/DEBIAN/control" <<CTRL
Package: clockwork-orange
Version: ${VERSION}
Section: graphics
Priority: optional
Architecture: ${ARCH}
Maintainer: ushineko <ushineko@users.noreply.github.com>
Depends: libgl1, libx11-6, libxcursor1, libxrandr2, libxinerama1, libxi6, libxxf86vm1, libxkbcommon0, qt6-tools-dev-tools, kconfig
Homepage: https://github.com/ushineko/clockwork-orange
Description: Wallpaper manager for KDE Plasma 6 with plugin sources
 Sets desktop and lock-screen wallpapers from local folders, Wallhaven and
 DuckDuckGo Images, with a shared blacklist and history, a systemd user
 daemon and a desktop window.
CTRL
cat > "$STAGE/DEBIAN/postinst" <<'POST'
#!/bin/sh
set -e
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
POST
chmod 755 "$STAGE/DEBIAN/postinst"
cp "$STAGE/DEBIAN/postinst" "$STAGE/DEBIAN/postrm"

mkdir -p dist
dpkg-deb --build --root-owner-group "$STAGE" "dist/${PKG}.deb"
ls -l "dist/${PKG}.deb"
