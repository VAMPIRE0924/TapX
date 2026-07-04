#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out_dir="${TAPX_BUILD_OUT:-$repo_root/build/linux-amd64}"
version="${TAPX_VERSION:-dev}"

mkdir -p "$out_dir"

build_one() {
  local name="$1"
  local package="$2"
  echo "build $name for linux amd64"
  (
    cd "$repo_root"
    GOTOOLCHAIN="${GOTOOLCHAIN:-local}" \
      CGO_ENABLED=1 \
      GOOS=linux \
      GOARCH=amd64 \
      go build -trimpath -ldflags="-s -w -X tapx/internal/buildinfo.Version=$version" -o "$out_dir/$name" "$package"
  )
  file "$out_dir/$name"
}

build_one tapx-core ./cmd/tapx-core
build_one tapx-panel ./cmd/tapx-panel
