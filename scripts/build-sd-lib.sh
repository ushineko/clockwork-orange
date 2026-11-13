#!/usr/bin/env bash
# Build the stable-diffusion.cpp shared libraries the stable_diffusion plugin
# embeds (spec 031 R9).
#
# Usage: scripts/build-sd-lib.sh <release-tag> <out-dir> [vulkan|cpu]
#
#   <release-tag>  the stable-diffusion.cpp release malina pins, e.g.
#                  master-1088-32d516c (the Makefile's SD_VERSION)
#   <out-dir>      receives the libraries, VERSION and SHA256SUMS
#
# Environment:
#   SD_GLIBC_CEILING  highest glibc the libraries may need; default 2.39, the
#                     floor of the Ubuntu 24.04 runner and of the GUI today.
#                     The script fails when a library needs more.
#   SD_SRC            an existing checkout of that tag to build instead of
#                     cloning (local use)
#   JOBS              parallel build jobs; default: all CPUs
#
# The flags are the ones upstream's own Linux release job uses, plus a
# Release build type, so the result matches what malina's binding was written
# against: a split ggml, every CPU variant loaded at run time, and an $ORIGIN
# rpath so the files find each other in whatever directory they are extracted to.
#
# Needs: git, cmake, a C++ toolchain, and for vulkan the loader headers and
# glslc (Debian/Ubuntu: libvulkan-dev glslc spirv-headers).

set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
    sed -n '2,12p' "$0" >&2
    exit 2
fi

tag=$1
out=$2
backend=${3:-vulkan}
ceiling=${SD_GLIBC_CEILING:-2.39}
jobs=${JOBS:-$(nproc 2>/dev/null || echo 2)}

case "$backend" in
    vulkan) backend_flags=(-DSD_VULKAN=ON) ;;
    cpu) backend_flags=() ;;
    *) echo "unknown backend '$backend' (want vulkan or cpu)" >&2; exit 2 ;;
esac

work=$(mktemp -d)
cleanup() { rm -rf "$work"; }
trap cleanup EXIT

src=${SD_SRC:-}
if [ -z "$src" ]; then
    src="$work/src"
    echo "cloning stable-diffusion.cpp $tag ..."
    git clone --quiet --depth 1 --branch "$tag" --recurse-submodules --shallow-submodules \
        https://github.com/leejet/stable-diffusion.cpp.git "$src"
fi

built=$(git -C "$src" rev-parse --short=7 HEAD)
case "$tag" in
    *"-$built") ;;
    *) echo "checkout is $built but the tag is $tag: refusing to build a different version" >&2; exit 1 ;;
esac

echo "configuring ($backend) ..."
# shellcheck disable=SC2016 # $ORIGIN is for the dynamic loader, not the shell
cmake -S "$src" -B "$work/build" \
    -DCMAKE_BUILD_TYPE=Release \
    -DSD_BUILD_SHARED_LIBS=ON \
    -DSD_BUILD_SHARED_GGML_LIB=ON \
    -DGGML_NATIVE=OFF \
    -DGGML_BACKEND_DL=ON \
    -DGGML_CPU_ALL_VARIANTS=ON \
    -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON \
    -DCMAKE_INSTALL_RPATH='$ORIGIN' \
    "${backend_flags[@]}"
echo "building ..."
cmake --build "$work/build" --parallel "$jobs"

bin="$work/build/bin"
[ -f "$bin/libstable-diffusion.so" ] || { echo "no libstable-diffusion.so in $bin" >&2; exit 1; }

rm -rf "$out"
mkdir -p "$out"
# cp -L: the build leaves versioned names as symlinks, and an embedded copy
# cannot carry a symlink.
for f in "$bin"/lib*.so*; do
    cp -L "$f" "$out/"
done
printf '%s\n' "$tag" > "$out/VERSION"

# Highest glibc, and for information the highest libstdc++, any library needs.
worst=""
worstxx=""
for f in "$out"/lib*.so*; do
    g=$(objdump -T "$f" 2>/dev/null | grep -o 'GLIBC_[0-9][0-9.]*' | sed 's/GLIBC_//' | sort -V | tail -1 || true)
    x=$(objdump -T "$f" 2>/dev/null | grep -o 'GLIBCXX_[0-9][0-9.]*' | sed 's/GLIBCXX_//' | sort -V | tail -1 || true)
    if [ -n "$g" ] && [ "$(printf '%s\n%s\n' "${worst:-0}" "$g" | sort -V | tail -1)" = "$g" ]; then worst=$g; fi
    if [ -n "$x" ] && [ "$(printf '%s\n%s\n' "${worstxx:-0}" "$x" | sort -V | tail -1)" = "$x" ]; then worstxx=$x; fi
done
echo "highest glibc needed: ${worst:-none}   highest libstdc++ needed: GLIBCXX_${worstxx:-none}"
if [ -n "$worst" ] && [ "$(printf '%s\n%s\n' "$ceiling" "$worst" | sort -V | tail -1)" != "$ceiling" ]; then
    echo "FAIL: the libraries need glibc $worst, above the ceiling $ceiling" >&2
    exit 1
fi

(cd "$out" && sha256sum lib*.so* VERSION > SHA256SUMS)
echo "built $(find "$out" -name 'lib*.so*' | wc -l) libraries ($(du -sh "$out" | cut -f1)) in $out"
