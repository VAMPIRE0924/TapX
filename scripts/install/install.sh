#!/usr/bin/env bash
set -euo pipefail

repo="${TAPX_REPO:-VAMPIRE0924/TapX}"
version="${TAPX_VERSION:-latest}"

usage() {
  cat <<'EOF'
用法：install.sh [install|menu|status|start|stop|restart|settings|set-panel|logs|uninstall]

这个脚本会下载最新的 TapX Linux amd64 安装包，然后启动交互式
TapX 管理器。安装完成后，可以再次运行 `tapx` 打开管理菜单。

环境变量：
  TAPX_VERSION=latest|v0.1.0|0.1.0
  TAPX_REPO=VAMPIRE0924/TapX
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "${1:-}" == "help" ]]; then
  usage
  exit 0
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "TapX 安装脚本必须在 Linux 上运行" >&2
  exit 1
fi

if [[ "$(id -u)" != "0" ]]; then
  echo "TapX 安装脚本必须使用 root 权限运行" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64)
    asset="tapx-linux-amd64.tar.gz"
    package_dir="tapx-linux-amd64"
    ;;
  *)
    echo "不支持的架构：$(uname -m)" >&2
    echo "当前公开 Linux 版本仅支持 amd64" >&2
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
    echo "缺少必要命令：curl 或 wget" >&2
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

echo "正在下载：$url"
download "$url" "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

bundle_dir="$tmp/$package_dir"
if [[ ! -d "$bundle_dir" ]]; then
  bundle_dir="$(find "$tmp" -type f -name tapx-core -exec dirname {} \; | head -n 1)"
fi
if [[ -z "$bundle_dir" || ! -x "$bundle_dir/tapx-core" || ! -x "$bundle_dir/tapx-panel" || ! -x "$bundle_dir/tapx" ]]; then
  echo "TapX Linux 安装包无效：缺少 tapx-core、tapx-panel 或 tapx 管理器" >&2
  exit 1
fi

export TAPX_BUILD_DIR="$bundle_dir"
cmd="${1:-install}"
shift || true
exec "$bundle_dir/tapx" "$cmd" "$@"
