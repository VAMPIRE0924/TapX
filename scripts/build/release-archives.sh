#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
version="${TAPX_VERSION:-0.0.0-dev}"
dist_dir="${TAPX_DIST_DIR:-$repo_root/dist}"
work_dir="${TAPX_RELEASE_WORK_DIR:-$repo_root/build/release-archives}"

linux_build="${TAPX_LINUX_BUILD_DIR:-$repo_root/build/linux-amd64}"
openwrt_pkg_dir="${TAPX_OPENWRT_PACKAGE_DIR:-$repo_root/build/openwrt-x86-64/packages}"

linux_name="tapx-linux-amd64"
openwrt_name="tapx-openwrt-x86-64"
linux_asset="tapx-linux-amd64.tar.gz"
openwrt_asset="tapx-openwrt-x86-64.tar.gz"

need_file() {
  if [[ ! -f "$1" ]]; then
    echo "missing required file: $1" >&2
    exit 1
  fi
}

need_file "$linux_build/tapx-core"
need_file "$linux_build/tapx-panel"
need_file "$openwrt_pkg_dir/tapx-core_${version}_x86_64.ipk"
need_file "$openwrt_pkg_dir/luci-app-tapx_${version}_all.ipk"
need_file "$repo_root/scripts/install/linux-install.sh"
need_file "$repo_root/scripts/install/install.sh"

rm -rf "$work_dir" "$dist_dir"
mkdir -p "$work_dir/$linux_name" "$work_dir/$openwrt_name" "$dist_dir"

install -m 0755 "$linux_build/tapx-core" "$work_dir/$linux_name/tapx-core"
install -m 0755 "$linux_build/tapx-panel" "$work_dir/$linux_name/tapx-panel"
install -m 0755 "$repo_root/scripts/install/linux-install.sh" "$work_dir/$linux_name/tapx"
install -m 0755 "$repo_root/scripts/install/install.sh" "$work_dir/$linux_name/install.sh"

cp "$openwrt_pkg_dir/tapx-core_${version}_x86_64.ipk" "$work_dir/$openwrt_name/"
cp "$openwrt_pkg_dir/luci-app-tapx_${version}_all.ipk" "$work_dir/$openwrt_name/"

tar -C "$work_dir" -czf "$dist_dir/$linux_asset" "$linux_name"
tar -C "$work_dir" -czf "$dist_dir/$openwrt_asset" "$openwrt_name"

(
  cd "$dist_dir"
  sha256sum "$linux_asset" "$openwrt_asset" > SHA256SUMS
)

echo "$dist_dir/$linux_asset"
echo "$dist_dir/$openwrt_asset"
