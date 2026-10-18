#!/usr/bin/env bash
#
# Stages the fynstall installer's config and payload for one target (spec 025).
#
#   packaging/fynstall/stage.sh VERSION OS/ARCH BINDIR [OUTDIR]
#
# BINDIR holds clockwork-orange and clockwork-orange-gui (.exe on Windows),
# already built for OS/ARCH. OUTDIR defaults to build/installer. The result is
# OUTDIR/fynstall.yaml with the version and target filled in, and the payload
# beside it, which `fynstall build -c OUTDIR/fynstall.yaml` turns into an
# installer. Run from the repository root; bash on Windows (Git Bash) too, so
# the CI job there stages the same way.

set -euo pipefail

if [ $# -lt 3 ]; then
    echo "usage: $0 VERSION OS/ARCH BINDIR [OUTDIR]" >&2
    exit 2
fi
version="${1#v}"
target="$2"
bindir="$3"
out="${4:-build/installer}"
os="${target%/*}"
arch="${target#*/}"
exe=""
[ "$os" = windows ] && exe=".exe"

rm -rf "$out"
mkdir -p "$out/$os-$arch"
for bin in clockwork-orange clockwork-orange-gui; do
    cp "$bindir/$bin$exe" "$out/$os-$arch/$bin$exe"
done
cp packaging/icons/clockwork-orange-512x512.png "$out/icon.png"
cp README.md LICENSE "$out/"
sed -e "s|@VERSION@|$version|" -e "s|@TARGET@|$target|" \
    packaging/fynstall/fynstall.yaml.in > "$out/fynstall.yaml"
echo "Staged $target $version in $out"
