#!/bin/sh
# Supply missing build headers in an isolated prefix, without sudo/apt install.
set -eu
mkdir -p "${1:-/tmp/lazymusic-linux-deps}"
deps_dir=$(realpath "${1:-/tmp/lazymusic-linux-deps}")
mkdir -p "$deps_dir/downloads" "$deps_dir/sysroot"
cd "$deps_dir/downloads"
apt-get download libgstreamer1.0-dev
for package in ./*.deb; do
    dpkg-deb -x "$package" "$deps_dir/sysroot"
done
triplet=$(gcc -print-multiarch)
libdir="$deps_dir/sysroot/usr/lib/$triplet"
for pc in "$libdir/pkgconfig"/*.pc; do
    sed -i "s|^prefix=/usr$|prefix=$deps_dir/sysroot/usr|" "$pc"
done
# The dev package contains linker symlinks; use the already-installed runtime.
for link in "$libdir"/*.so; do
    target=$(readlink "$link")
    if [ ! -e "$libdir/$target" ] && [ -e "/usr/lib/$triplet/$target" ]; then
        ln -s "/usr/lib/$triplet/$target" "$libdir/$target"
    fi
done
printf '\nUse PKG_CONFIG_PATH=%s/pkgconfig for build/test.\n' "$libdir"
PKG_CONFIG_PATH="$libdir/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}" pkg-config --modversion gstreamer-1.0
