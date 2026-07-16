#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
arch="${1:-amd64}"
version="${TAPX_VERSION:-dev}"

case "$arch" in
  amd64)
    cc="${CC:-gcc}"
    ;;
  arm64)
    cc="${CC:-aarch64-linux-gnu-gcc}"
    ;;
  *)
    echo "unsupported Linux build architecture: $arch" >&2
    exit 1
    ;;
esac

if ! command -v "$cc" >/dev/null 2>&1; then
  echo "missing C compiler for linux/$arch: $cc" >&2
  exit 1
fi

out_dir="${TAPX_BUILD_OUT:-$repo_root/build/linux-$arch}"
mkdir -p "$out_dir"

build_one() {
  local name="$1" package="$2"
  echo "build $name for linux/$arch"
  (
    cd "$repo_root"
    GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" \
      CGO_ENABLED=1 \
      GOOS=linux \
      GOARCH="$arch" \
      CC="$cc" \
      go build -trimpath \
        -ldflags="-s -w -X tapx/internal/buildinfo.Version=$version" \
        -o "$out_dir/$name" \
        "$package"
  )
  file "$out_dir/$name"
}

build_one tapx-core ./cmd/tapx-core
build_one tapx-panel ./cmd/tapx-panel
