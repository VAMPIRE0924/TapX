#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
sdk_root="${TAPX_OPENWRT_SDK_ROOT:-$HOME/tapx-openwrt-sdk}"
out_dir="${TAPX_BUILD_OUT:-$repo_root/build/openwrt-x86-64}"
version="${TAPX_VERSION:-dev}"

if command -v cygpath >/dev/null 2>&1; then
  sdk_root="$(cygpath -u "$sdk_root")"
  out_dir="$(cygpath -u "$out_dir")"
fi

tool="$(
  find "$sdk_root" \
    -path "*/staging_dir/toolchain-*/bin/x86_64-openwrt-linux-musl-gcc" \
    \( -type f -o -type l \) \
    -print -quit 2>/dev/null || true
)"

find_zig() {
  if [[ -n "${TAPX_OPENWRT_ZIG:-}" && -x "${TAPX_OPENWRT_ZIG}" ]]; then
    echo "${TAPX_OPENWRT_ZIG}"
    return 0
  fi
  if command -v zig >/dev/null 2>&1; then
    command -v zig
    return 0
  fi
  for candidate in /clang64/bin/zig /ucrt64/bin/zig /mingw64/bin/zig /c/msys64/clang64/bin/zig; do
    if [[ -x "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
  done
}

if [[ -n "$tool" ]]; then
  sdk="${tool%%/staging_dir/*}"
  staging_dir="${STAGING_DIR:-$sdk/staging_dir}"
  cc="${CC:-$tool}"
  ldflags="-s -w"
  echo "build mode: OpenWrt SDK"
elif zig="$(find_zig)"; then
  staging_dir="${STAGING_DIR:-}"
  cc="${CC:-$zig cc -target x86_64-linux-musl}"
  ldflags="-s -w -linkmode external -extldflags -static"
  echo "build mode: Zig x86_64-linux-musl fallback"
else
  echo "missing x86_64 OpenWrt SDK toolchain under $sdk_root" >&2
  echo "run: ./scripts/dev/prepare-openwrt-sdk.sh x86-64" >&2
  echo "or install zig and retry the musl fallback" >&2
  exit 1
fi

mkdir -p "$out_dir"

echo "build TapX binaries for OpenWrt x86-64"
echo "cc: $cc"

build_one() {
  local name="$1"
  local package="$2"
  echo "out: $out_dir/$name"
  (
    cd "$repo_root"
    STAGING_DIR="$staging_dir" \
      GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" \
      CGO_ENABLED=1 \
      GOOS=linux \
      GOARCH=amd64 \
      CC="$cc" \
      go build -trimpath -ldflags="$ldflags -X tapx/internal/buildinfo.Version=$version" -o "$out_dir/$name" "$package"
  )
  file "$out_dir/$name"
}

build_one tapx-core ./cmd/tapx-core
build_one tapx-panel ./cmd/tapx-panel
