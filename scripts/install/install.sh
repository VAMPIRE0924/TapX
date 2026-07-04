#!/usr/bin/env bash
set -euo pipefail

repo="${TAPX_REPO:-VAMPIRE0924/TapX}"
version="${TAPX_VERSION:-latest}"

usage() {
  cat <<'EOF'
Usage: install.sh [install|menu|status|start|stop|restart|settings|set-panel|logs|uninstall]

This script downloads the latest TapX Linux amd64 bundle, then runs the
interactive TapX manager. After installation, run `tapx` again for the menu.

Environment:
  TAPX_VERSION=latest|v0.1.0|0.1.0
  TAPX_REPO=VAMPIRE0924/TapX
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "${1:-}" == "help" ]]; then
  usage
  exit 0
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "TapX installer must run on Linux" >&2
  exit 1
fi

if [[ "$(id -u)" != "0" ]]; then
  echo "TapX installer must run as root" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64)
    asset="tapx-linux-amd64.tar.gz"
    package_dir="tapx-linux-amd64"
    ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    echo "current public Linux release supports amd64 only" >&2
    exit 1
    ;;
esac

download() {
  local url="$1" out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 15 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  else
    echo "missing required command: curl or wget" >&2
    exit 1
  fi
}

if [[ -x /usr/local/bin/tapx && "${1:-}" != "install" && "${1:-}" != "reinstall" ]]; then
  exec /usr/local/bin/tapx "$@"
fi

if [[ "$version" == "latest" ]]; then
  url="https://github.com/${repo}/releases/latest/download/${asset}"
else
  tag="${version#v}"
  url="https://github.com/${repo}/releases/download/v${tag}/${asset}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $url"
download "$url" "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

bundle_dir="$tmp/$package_dir"
if [[ ! -d "$bundle_dir" ]]; then
  bundle_dir="$(find "$tmp" -type f -name tapx-core -exec dirname {} \; | head -n 1)"
fi
if [[ -z "$bundle_dir" || ! -x "$bundle_dir/tapx-core" || ! -x "$bundle_dir/tapx-panel" || ! -x "$bundle_dir/tapx" ]]; then
  echo "invalid TapX Linux package: missing tapx-core, tapx-panel, or tapx manager" >&2
  exit 1
fi

export TAPX_BUILD_DIR="$bundle_dir"
cmd="${1:-install}"
shift || true
exec "$bundle_dir/tapx" "$cmd" "$@"
