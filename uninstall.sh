#!/usr/bin/env bash
#
# Removes exactly what install.sh puts in place: two binaries, a launcher entry
# and the icon at each size. Everything clockwork-orange has made for you stays, and this prints
# where it is. Idempotent.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_ROOT="${HOME}/.local/share/icons/hicolor"
ICON_SIZES="16 32 48 64 128 256 512"

APP_ID="io.ushineko.clockwork-orange"

DRY_RUN=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: uninstall.sh [--dry-run]

  --dry-run   List what would be removed, change nothing

Removes only the files install.sh placed. Your config, your downloaded
wallpapers and the history and blacklist databases are left alone, and their
locations are printed so you can remove them by hand if you want to. The
systemd user unit, if installed, is removed with `clockwork-orange service
uninstall`.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

echo "Removing clockwork-orange ..."

removed=0
files=("${BIN_DIR}/clockwork-orange" "${BIN_DIR}/clockwork-orange-gui" "${APP_DIR}/${APP_ID}.desktop")
for res in $ICON_SIZES; do
    files+=("${ICON_ROOT}/${res}x${res}/apps/clockwork-orange.png")
done
for f in "${files[@]}"; do
    if [ -e "$f" ]; then
        removed=$((removed + 1))
        if [ "$DRY_RUN" -eq 1 ]; then
            echo "  would remove $f"
        else
            echo "  removing $f"
            rm -f "$f"
        fi
    fi
done
if [ "$removed" -eq 0 ]; then
    echo "  nothing to remove; install.sh has not run, or has already been undone"
fi

if [ "$DRY_RUN" -eq 0 ]; then
    command -v update-desktop-database >/dev/null 2>&1 && \
        update-desktop-database "${APP_DIR}" >/dev/null 2>&1 || true
    command -v gtk-update-icon-cache >/dev/null 2>&1 && \
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
    command -v kbuildsycoca6 >/dev/null 2>&1 && kbuildsycoca6 --noincremental >/dev/null 2>&1 || true
fi

CONFIG_HOME="${HOME}/.config"

echo
echo "Done."
echo
echo "Nothing you made was removed. It is in:"
echo
kept=(
    "${CONFIG_HOME}/clockwork-orange.yml|your settings"
    "${CONFIG_HOME}/clockwork-orange/history.db|download history"
    "${CONFIG_HOME}/clockwork-orange/blacklist.db|the blacklist and its thumbnails"
    "${HOME}/Pictures/Wallpapers|downloaded wallpapers (per-plugin folders)"
    "${CONFIG_HOME}/systemd/user/clockwork-orange.service|the daemon unit, if installed"
    "${XDG_CONFIG_HOME:-${HOME}/.config}/fyne/${APP_ID}|the window's colour scheme and font"
)
width=0
for entry in "${kept[@]}"; do
    path="${entry%%|*}"
    if [ "${#path}" -gt "$width" ]; then
        width="${#path}"
    fi
done
for entry in "${kept[@]}"; do
    printf '    %-*s  %s
' "$width" "${entry%%|*}" "${entry#*|}"
done
echo
echo "If the daemon is installed, stop it first: clockwork-orange service uninstall"
