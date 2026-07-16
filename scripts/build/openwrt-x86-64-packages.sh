#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
sdk_root="${TAPX_OPENWRT_SDK_ROOT:-$HOME/tapx-openwrt-sdk}"
out_dir="${TAPX_BUILD_OUT:-$repo_root/build/openwrt-x86-64}"
pkg_dir="${TAPX_PACKAGE_OUT:-$out_dir/packages}"
version="${TAPX_VERSION:-0.0.0-dev}"

if command -v cygpath >/dev/null 2>&1; then
  sdk_root="$(cygpath -u "$sdk_root")"
  out_dir="$(cygpath -u "$out_dir")"
  pkg_dir="$(cygpath -u "$pkg_dir")"
fi

if [[ ! -x "$out_dir/tapx-core" || ! -x "$out_dir/tapx-panel" ]]; then
  "$repo_root/scripts/build/openwrt-x86-64.sh"
fi

tool="$(
  find "$sdk_root" \
    -path "*/staging_dir/toolchain-*/bin/x86_64-openwrt-linux-musl-gcc" \
    \( -type f -o -type l \) -print -quit 2>/dev/null || true
)"
if [[ -z "$tool" ]]; then
  echo "missing x86-64 OpenWrt SDK under $sdk_root" >&2
  echo "run: ./scripts/dev/prepare-openwrt-sdk.sh x86-64" >&2
  exit 1
fi

sdk="${tool%%/staging_dir/*}"
feed_dir="$sdk/package/tapx"
rm -rf "$feed_dir"
mkdir -p "$feed_dir"
cp -a "$repo_root/openwrt/." "$feed_dir/"

echo "build native OpenWrt packages with SDK: $sdk"
if [[ ! -d "$sdk/package/feeds/base/iproute2" || ! -d "$sdk/package/feeds/luci/luci-base" ]]; then
  echo "initialize OpenWrt package feeds"
  if ! timeout "${TAPX_OPENWRT_FEEDS_TIMEOUT:-300}" "$sdk/scripts/feeds" update -a; then
    echo "OpenWrt git server did not complete; retry official OpenWrt GitHub mirrors" >&2
    cp "$sdk/feeds.conf.default" "$sdk/feeds.conf"
    sed -i \
      -e 's#https://git.openwrt.org/openwrt/openwrt.git#https://github.com/openwrt/openwrt.git#' \
      -e 's#https://git.openwrt.org/feed/packages.git#https://github.com/openwrt/packages.git#' \
      -e 's#https://git.openwrt.org/project/luci.git#https://github.com/openwrt/luci.git#' \
      -e 's#https://git.openwrt.org/feed/routing.git#https://github.com/openwrt/routing.git#' \
      -e 's#https://git.openwrt.org/feed/telephony.git#https://github.com/openwrt/telephony.git#' \
      "$sdk/feeds.conf"
    "$sdk/scripts/feeds" clean
    "$sdk/scripts/feeds" update -a
  fi
  "$sdk/scripts/feeds" install -a
fi
make -C "$sdk" defconfig
mkdir -p "$sdk/bin"
find "$sdk/bin" -type f \
  \( -name 'tapx-core*.apk' -o -name 'tapx-panel*.apk' -o -name 'luci-app-tapx*.apk' \
     -o -name 'tapx-core*.ipk' -o -name 'tapx-panel*.ipk' -o -name 'luci-app-tapx*.ipk' \) \
  -delete
TAPX_PREBUILT_DIR="$out_dir" TAPX_VERSION="$version" \
  make -C "$sdk" package/tapx/compile V=sc

rm -rf "$pkg_dir"
mkdir -p "$pkg_dir"
mapfile -t packages < <(find "$sdk/bin" -type f \
  \( -name 'tapx-core-*.apk' -o -name 'tapx-panel-*.apk' -o -name 'luci-app-tapx-*.apk' \
     -o -name 'tapx-core_*.ipk' -o -name 'tapx-panel_*.ipk' -o -name 'luci-app-tapx_*.ipk' \) \
  -print | sort)
if ((${#packages[@]} != 3)); then
  printf 'expected 3 TapX packages, found %d:\n' "${#packages[@]}" >&2
  printf '  %s\n' "${packages[@]:-}" >&2
  exit 1
fi
cp "${packages[@]}" "$pkg_dir/"
printf '%s\n' "$pkg_dir"/*
