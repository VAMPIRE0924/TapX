#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
version="${TAPX_VERSION:-0.0.0-dev}"
dist_dir="${TAPX_DIST_DIR:-$repo_root/dist}"
work_dir="${TAPX_RELEASE_WORK_DIR:-$repo_root/build/release-archives}"

openwrt_pkg_dir="${TAPX_OPENWRT_PACKAGE_DIR:-$repo_root/build/openwrt-x86-64/packages}"

openwrt_name="tapx-openwrt-x86-64"
openwrt_asset="tapx-openwrt-x86-64.tar.gz"
linux_arches=(amd64 arm64)

need_file() {
  if [[ ! -f "$1" ]]; then
    echo "missing required file: $1" >&2
    exit 1
  fi
}

need_file "$repo_root/scripts/install/linux-install.sh"
need_file "$repo_root/scripts/install/install.sh"
need_file "$repo_root/scripts/install/openwrt-install.sh"

mapfile -t openwrt_packages < <(find "$openwrt_pkg_dir" -maxdepth 1 -type f \
  \( -name 'tapx-core_*.ipk' -o -name 'tapx-panel_*.ipk' -o -name 'luci-app-tapx_*.ipk' \
     -o -name 'tapx-core-*.apk' -o -name 'tapx-panel-*.apk' -o -name 'luci-app-tapx-*.apk' \) \
  -print | sort)
if ((${#openwrt_packages[@]} != 3)); then
  echo "expected 3 OpenWrt packages in $openwrt_pkg_dir" >&2
  exit 1
fi

rm -rf "$work_dir" "$dist_dir"
mkdir -p "$work_dir/$openwrt_name" "$dist_dir"

linux_assets=()
for arch in "${linux_arches[@]}"; do
  linux_build="$repo_root/build/linux-$arch"
  linux_name="tapx-linux-$arch"
  linux_asset="$linux_name.tar.gz"
  need_file "$linux_build/tapx-core"
  need_file "$linux_build/tapx-panel"
  mkdir -p "$work_dir/$linux_name"
  install -m 0755 "$linux_build/tapx-core" "$work_dir/$linux_name/tapx-core"
  install -m 0755 "$linux_build/tapx-panel" "$work_dir/$linux_name/tapx-panel"
  install -m 0755 "$repo_root/scripts/install/linux-install.sh" "$work_dir/$linux_name/tapx"
  install -m 0755 "$repo_root/scripts/install/install.sh" "$work_dir/$linux_name/install.sh"
  tar -C "$work_dir" -czf "$dist_dir/$linux_asset" "$linux_name"
  linux_assets+=("$linux_asset")
done

cp "${openwrt_packages[@]}" "$work_dir/$openwrt_name/"
install -m 0755 "$repo_root/scripts/install/openwrt-install.sh" "$work_dir/$openwrt_name/install.sh"

tar -C "$work_dir" -czf "$dist_dir/$openwrt_asset" "$openwrt_name"

(
  cd "$dist_dir"
  sha256sum "${linux_assets[@]}" "$openwrt_asset" > SHA256SUMS
)

linux_amd64_sha256="$(sha256sum "$dist_dir/tapx-linux-amd64.tar.gz" | awk '{print $1}')"
linux_arm64_sha256="$(sha256sum "$dist_dir/tapx-linux-arm64.tar.gz" | awk '{print $1}')"
openwrt_sha256="$(sha256sum "$dist_dir/$openwrt_asset" | awk '{print $1}')"
embedded_xray_version="$(awk '$1 == "github.com/xtls/xray-core" { print $2; exit }' "$repo_root/go.mod")"
generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

cat >"$dist_dir/tapx-update-manifest.json" <<EOF
{
  "schemaVersion": 1,
  "releaseVersion": "$version",
  "generatedAt": "$generated_at",
  "compatibility": {
    "panel": "$version",
    "tapxCore": "$version",
    "embeddedXray": "${embedded_xray_version#v}"
  },
  "platforms": {
    "linux-amd64": {
      "asset": "tapx-linux-amd64.tar.gz",
      "sha256": "$linux_amd64_sha256",
      "components": ["tapx-panel", "tapx-core", "embedded-xray"]
    },
    "linux-arm64": {
      "asset": "tapx-linux-arm64.tar.gz",
      "sha256": "$linux_arm64_sha256",
      "components": ["tapx-panel", "tapx-core", "embedded-xray"]
    },
    "openwrt-x86-64": {
      "asset": "$openwrt_asset",
      "sha256": "$openwrt_sha256",
      "components": ["tapx-panel", "tapx-core", "embedded-xray", "luci-app-tapx"]
    }
  }
}
EOF

echo "$dist_dir/tapx-linux-amd64.tar.gz"
echo "$dist_dir/tapx-linux-arm64.tar.gz"
echo "$dist_dir/$openwrt_asset"
echo "$dist_dir/tapx-update-manifest.json"
