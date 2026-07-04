#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out_dir="${TAPX_BUILD_OUT:-$repo_root/build/openwrt-x86-64}"
pkg_dir="${TAPX_IPK_OUT:-$out_dir/packages}"
version="${TAPX_VERSION:-0.0.0-dev}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need tar
need gzip

if [[ ! -f "$out_dir/tapx-core" ]]; then
  "$repo_root/scripts/build/openwrt-x86-64.sh"
fi

mkdir -p "$pkg_dir"

package_ipk() {
  local package="$1"
  local arch="$2"
  local depends="$3"
  local description="$4"
  local data_root="$5"
  local conffiles="${6:-}"
  local work="$out_dir/ipk-work/$package"
  local control_dir="$work/control"
  local file="$pkg_dir/${package}_${version}_${arch}.ipk"

  rm -rf "$work"
  mkdir -p "$control_dir"

  cat > "$control_dir/control" <<EOF
Package: $package
Version: $version
Architecture: $arch
Maintainer: TapX
Section: net
Priority: optional
Depends: $depends
Description: $description
EOF

  if [[ -n "$conffiles" ]]; then
    printf '%s\n' "$conffiles" > "$control_dir/conffiles"
  fi

  printf '2.0\n' > "$work/debian-binary"
  tar -C "$control_dir" -czf "$work/control.tar.gz" .
  tar -C "$data_root" -czf "$work/data.tar.gz" .
  if command -v ar >/dev/null 2>&1; then
    (cd "$work" && ar rcs "$file" debian-binary control.tar.gz data.tar.gz)
  else
    GOTOOLCHAIN="${GOTOOLCHAIN:-local}" go run "$repo_root/scripts/build/mkipk.go" \
      "$file" "$work/debian-binary" "$work/control.tar.gz" "$work/data.tar.gz"
  fi
  echo "$file"
}

core_root="$out_dir/ipk-root/tapx-core"
rm -rf "$core_root"
mkdir -p "$core_root/usr/bin"
install -m 0755 "$out_dir/tapx-core" "$core_root/usr/bin/tapx-core"
cp -a "$repo_root/openwrt/tapx-core/files/." "$core_root/"
chmod 0755 "$core_root/etc/init.d/tapx"
chmod 0644 "$core_root/etc/config/tapx" "$core_root/etc/tapx/runtime.json.example"

luci_root="$out_dir/ipk-root/luci-app-tapx"
rm -rf "$luci_root"
mkdir -p "$luci_root"
cp -a "$repo_root/openwrt/luci-app-tapx/root/." "$luci_root/"
find "$luci_root" -type d -exec chmod 0755 {} +
find "$luci_root" -type f -exec chmod 0644 {} +

package_ipk tapx-core x86_64 libc "TapX TAP/TUN raw transport core" "$core_root" "/etc/config/tapx"
package_ipk luci-app-tapx all "luci-base, tapx-core" "LuCI app for TapX" "$luci_root"
