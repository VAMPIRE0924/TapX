#!/usr/bin/env bash
set -euo pipefail

repo="${TAPX_REPO:-VAMPIRE0924/TapX}"
version="${TAPX_VERSION:-latest}"

read_tty() {
  local __name="$1" __prompt="$2" __value=""
  if [[ -r /dev/tty ]]; then
    read -r -p "$__prompt" __value </dev/tty
  else
    read -r -p "$__prompt" __value
  fi
  printf -v "$__name" '%s' "$__value"
}

choose_language() {
  if [[ "${TAPX_LANG:-}" == "en" || "${TAPX_LANG:-}" == "zh" ]]; then
    return
  fi
  printf '1,English (default)\n2,中文\n\n'
  local choice=""
  read_tty choice '> '
  case "$choice" in
    2|zh|ZH) TAPX_LANG=zh ;;
    *) TAPX_LANG=en ;;
  esac
  export TAPX_LANG
}

message() {
  if [[ "$TAPX_LANG" == "zh" ]]; then
    printf '%s' "$2"
  else
    printf '%s' "$1"
  fi
}

detect_architecture() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    armv7l|armv7) printf 'armv7' ;;
    i386|i486|i586|i686) printf '386' ;;
    riscv64) printf 'riscv64' ;;
    *) return 1 ;;
  esac
}

download() {
  local url="$1" output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --connect-timeout 15 -o "$output" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
  else
    printf '%s\n' "$(message 'curl or wget is required.' '需要安装 curl 或 wget。')" >&2
    return 1
  fi
}

choose_language

if [[ "$(uname -s)" != "Linux" ]]; then
  printf '%s\n' "$(message 'TapX supports Linux installation only.' 'TapX 一键安装仅支持 Linux。')" >&2
  exit 1
fi
if [[ "$(id -u)" != "0" ]]; then
  printf '%s\n' "$(message 'Run this installer as root.' '请使用 root 权限运行安装脚本。')" >&2
  exit 1
fi

arch="$(detect_architecture)" || {
  printf '%s %s\n' "$(message 'Unsupported architecture:' '不支持的系统架构：')" "$(uname -m)" >&2
  exit 1
}
asset="tapx-linux-${arch}.tar.gz"
package_dir="tapx-linux-${arch}"

if [[ -x /usr/local/bin/tapx && "${1:-}" != "install" && "${1:-}" != "reinstall" ]]; then
  exec env TAPX_LANG="$TAPX_LANG" /usr/local/bin/tapx "$@"
fi

if [[ -n "${TAPX_RELEASE_BASE_URL:-}" ]]; then
  base_url="${TAPX_RELEASE_BASE_URL%/}"
elif [[ "$version" == "latest" ]]; then
  base_url="https://github.com/${repo}/releases/latest/download"
else
  base_url="https://github.com/${repo}/releases/download/v${version#v}"
fi

tmp="$(mktemp -d /tmp/tapx-installer.XXXXXX)"
trap 'rm -rf "$tmp"' EXIT INT TERM

printf '%s %s (%s)\n' "$(message 'Downloading TapX for' '正在下载 TapX：')" "$arch" "$asset"
if ! download "$base_url/$asset" "$tmp/$asset"; then
  printf '%s\n' "$(message "No TapX release is available for ${arch}." "当前 Release 没有 ${arch} 架构的 TapX 安装包。")" >&2
  exit 1
fi
download "$base_url/SHA256SUMS" "$tmp/SHA256SUMS"
expected="$(awk -v file="$asset" '$2 == file || $2 == "*" file { print $1; exit }' "$tmp/SHA256SUMS")"
if [[ ! "$expected" =~ ^[0-9a-fA-F]{64}$ ]]; then
  printf '%s\n' "$(message 'The release checksum is missing.' 'Release 缺少有效的校验值。')" >&2
  exit 1
fi
actual="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
if [[ "${actual,,}" != "${expected,,}" ]]; then
  printf '%s\n' "$(message 'Release checksum verification failed.' 'Release 文件校验失败。')" >&2
  exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp"
bundle_dir="$tmp/$package_dir"
if [[ ! -d "$bundle_dir" ]]; then
  bundle_dir="$(find "$tmp" -type f -name tapx-core -exec dirname {} \; | head -n 1)"
fi
if [[ -z "$bundle_dir" || ! -x "$bundle_dir/tapx-core" || ! -x "$bundle_dir/tapx-panel" || ! -x "$bundle_dir/tapx" ]]; then
  printf '%s\n' "$(message 'The TapX release package is incomplete.' 'TapX Release 安装包不完整。')" >&2
  exit 1
fi

export TAPX_BUILD_DIR="$bundle_dir"
command="${1:-install}"
shift || true
env TAPX_LANG="$TAPX_LANG" TAPX_BUILD_DIR="$bundle_dir" "$bundle_dir/tapx" "$command" "$@"
