#!/usr/bin/env bash
#
# Installs the clockwork-orange command line/daemon and, unless told otherwise,
# the desktop window with its launcher entry and icon. Idempotent: safe to
# re-run.
#
# Nothing here touches your wallpapers, your config file or the history and
# blacklist databases. It builds from this checkout and copies four files into
# ~/.local. The systemd user unit is installed by `clockwork-orange service
# install` (or the GUI), not here.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_ROOT="${HOME}/.local/share/icons/hicolor"
# Every size the packages ship. One 512 px file alone is enough for GTK but
# KDE's icon loader picks the theme directory matching the requested size and
# only falls back to scaling within the sizes hicolor declares, so a menu asking
# for 48 px against a lone 512 px file came up with the placeholder icon.
ICON_SIZES="16 32 48 64 128 256 512"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

APP_ID="io.ushineko.clockwork-orange"

DRY_RUN=0
WITH_GUI=1
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        --with-gui) WITH_GUI=1 ;;
        --no-gui) WITH_GUI=0 ;;
        -h|--help)
            cat <<'USAGE'
Usage: install.sh [--dry-run] [--no-gui]

  --dry-run   Show what would be installed, change nothing
  --no-gui    Install the command line only, skipping clockwork-orange-gui

Installs:
  ~/.local/bin/clockwork-orange                                       the command line and daemon
  ~/.local/bin/clockwork-orange-gui                                   the window
  ~/.local/share/applications/io.ushineko.clockwork-orange.desktop    the launcher entry
  ~/.local/share/icons/hicolor/<size>/apps/clockwork-orange.png       its icon, at 16–512 px

The window is installed by default. It needs CGO and a C toolchain; if it will
not build, the command line is still installed and the window is skipped with a
note naming the packages to install. The command line is static, needs neither,
and does everything the window does except show pictures.

Nothing you have made is touched, on install or on uninstall: your config,
your downloaded wallpapers, and the history and blacklist databases stay where
they are. uninstall.sh prints where they live.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

run() {
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: $*"
    else
        "$@"
    fi
}

echo "Installing clockwork-orange from ${REPO_DIR} ..."

if ! command -v go >/dev/null 2>&1; then
    echo "Error: go is not installed. clockwork-orange needs Go 1.25 or newer to build." >&2
    echo "       Arch: pacman -S go   Debian/Ubuntu: apt install golang-go" >&2
    exit 1
fi

echo "Building the command line ..."
run make -C "$REPO_DIR" build

echo "Installing the command line to ${BIN_DIR} ..."
run install -Dm755 "${REPO_DIR}/bin/clockwork-orange" "${BIN_DIR}/clockwork-orange"

# The window is installed by default, but a failure to build it must not take
# the command line down with it. The CLI is the artifact everything depends on
# -- it is what builds and deploys mods -- and a missing C toolchain is a reason
# to skip the window, not a reason to leave the machine without clockwork-orange.
if [ "$WITH_GUI" -eq 1 ]; then
    echo "Building the desktop window ..."
    if [ "$DRY_RUN" -eq 1 ] || make -C "$REPO_DIR" build-gui; then
        run install -Dm755 "${REPO_DIR}/bin/clockwork-orange-gui" "${BIN_DIR}/clockwork-orange-gui"
        run install -Dm644 "${REPO_DIR}/packaging/${APP_ID}.desktop" "${APP_DIR}/${APP_ID}.desktop"
        for res in $ICON_SIZES; do
            run install -Dm644 "${REPO_DIR}/packaging/icons/clockwork-orange-${res}x${res}.png" \
                "${ICON_ROOT}/${res}x${res}/apps/clockwork-orange.png"
        done
    else
        WITH_GUI=0
        echo
        echo "The window did not build, so it was skipped. The command line is unaffected" >&2
        echo "and does everything the window does. Building it needs CGO, OpenGL and the" >&2
        echo "X11 or Wayland development headers:" >&2
        echo "    Arch:          base-devel libgl libxi libxcursor libxrandr libxinerama" >&2
        echo "    Debian/Ubuntu: build-essential libgl1-mesa-dev xorg-dev" >&2
        echo "    Fedora:        gcc mesa-libGL-devel libXi-devel libXcursor-devel libXrandr-devel libXinerama-devel" >&2
        echo "Re-run with --no-gui to skip it without this message." >&2
        echo
    fi
fi

# The Python versions' install-desktop-entry.sh wrote a launcher entry of the
# same display name that ran `python3 …/clockwork-orange.py --gui`. Left in
# place it sits beside the new one in the menu, indistinguishable, and starts
# the old program. It is removed only when it still points at the Python
# entrypoint, so a user's own entry of that name is left alone.
LEGACY_ENTRY="${APP_DIR}/clockwork-orange.desktop"
if [ -f "$LEGACY_ENTRY" ] && grep -q 'clockwork-orange\.py' "$LEGACY_ENTRY"; then
    echo "Removing the legacy Python launcher entry ${LEGACY_ENTRY} ..."
    run rm -f "$LEGACY_ENTRY"
fi

if [ "$WITH_GUI" -eq 1 ] && command -v update-desktop-database >/dev/null 2>&1; then
    run update-desktop-database "${APP_DIR}"
fi
# KDE keeps its own application and icon caches; without a rebuild the menu can
# show the new entry with the placeholder icon until the next login.
if [ "$WITH_GUI" -eq 1 ] && command -v kbuildsycoca6 >/dev/null 2>&1; then
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: kbuildsycoca6 --noincremental"
    else
        kbuildsycoca6 --noincremental >/dev/null 2>&1 || true
    fi
fi
# The icon cache is per theme directory and only some desktops need it poked;
# a failure here costs nothing but a stale icon until the next login.
if [ "$WITH_GUI" -eq 1 ] && command -v gtk-update-icon-cache >/dev/null 2>&1; then
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: gtk-update-icon-cache -f -t ${HOME}/.local/share/icons/hicolor"
    else
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
fi

echo
echo "Done."
case ":${PATH}:" in
    *":${BIN_DIR}:"*) ;;
    *) echo "Note: ${BIN_DIR} is not on your PATH." ;;
esac
echo
echo "Next:"
echo
echo "  clockwork-orange --self-test           # check the environment"
echo "  clockwork-orange --desktop -d ~/Pictures   # set a random wallpaper once"
echo "  clockwork-orange service install       # install and enable the systemd user unit"
echo "  clockwork-orange plugins list          # the wallpaper sources"
if [ "$WITH_GUI" -eq 1 ]; then
    echo
    echo "Or open the window: clockwork-orange-gui, or \"Clockwork Orange\" in your application launcher."
fi
