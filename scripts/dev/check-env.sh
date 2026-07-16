#!/usr/bin/env bash
set -euo pipefail

failures=0

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing: $1" >&2
    failures=$((failures + 1))
    return 1
  fi
}

section() {
  printf '\n== %s ==\n' "$1"
}

show_cmd() {
  local name="$1"
  shift
  if command -v "$name" >/dev/null 2>&1; then
    printf '%-12s %s\n' "$name:" "$("$@" 2>/dev/null | head -n1)"
  else
    printf '%-12s missing\n' "$name:"
    failures=$((failures + 1))
  fi
}

section "Required commands"
need go
need gcc
need make
need node
need npm
need ip
need pkg-config
need git
need rg
need curl
need wget
need python3

section "Tool versions"
show_cmd go go version
show_cmd gcc gcc --version
show_cmd clang clang --version
show_cmd make make --version
show_cmd cmake cmake --version
show_cmd ninja ninja --version
show_cmd node node --version
show_cmd npm npm --version
show_cmd git git --version
show_cmd rg rg --version
show_cmd jq jq --version
show_cmd tcpdump tcpdump --version
show_cmd ethtool ethtool --version
show_cmd ping ping -V
show_cmd iperf3 iperf3 --version
show_cmd socat socat -V
show_cmd sshpass sshpass -V
show_cmd ccache ccache --version
show_cmd zstd zstd --version
show_cmd bison bison --version
show_cmd flex flex --version
show_cmd gawk gawk --version
show_cmd msgfmt msgfmt --version

section "Go environment"
GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go env GOVERSION GOOS GOARCH CGO_ENABLED CC CXX GOTOOLCHAIN GOPATH GOMODCACHE

section "Native libraries"
if pkg-config --exists sqlite3; then
  echo "sqlite3: $(pkg-config --modversion sqlite3)"
else
  echo "sqlite3: missing pkg-config entry" >&2
  failures=$((failures + 1))
fi
if pkg-config --exists openssl; then
  echo "openssl: $(pkg-config --modversion openssl)"
else
  echo "openssl: missing pkg-config entry" >&2
  failures=$((failures + 1))
fi

section "Network tools"
ip -V
if [[ -c /dev/net/tun ]]; then
  echo "tun device node: yes"
else
  echo "tun device node: missing /dev/net/tun" >&2
  failures=$((failures + 1))
fi
if [[ "$(id -u)" == "0" ]]; then
  echo "netns integration: runnable as root"
else
  echo "netns integration: requires root or CAP_NET_ADMIN"
fi

section "OpenWrt SDK hints"
echo "current OpenWrt build target: x86-64 only"
find_sdk_tool() {
  find /opt "$HOME/tapx-openwrt-sdk" "$HOME/openwrt-sdk" \
    -path "*/staging_dir/toolchain-*/bin/$1" \( -type f -o -type l \) \
    -print -quit 2>/dev/null || true
}

if command -v x86_64-openwrt-linux-musl-gcc >/dev/null 2>&1; then
  echo "openwrt x86_64 toolchain: $(command -v x86_64-openwrt-linux-musl-gcc)"
else
  x86_tool="$(find_sdk_tool x86_64-openwrt-linux-musl-gcc)"
  if [[ -n "$x86_tool" ]]; then
    echo "openwrt x86_64 toolchain: found in SDK, not in PATH: $x86_tool"
  else
    echo "openwrt x86_64 toolchain: not in PATH"
  fi
fi

echo "extra OpenWrt targets: not checked in current x86-only phase"
find /opt "$HOME/tapx-openwrt-sdk" "$HOME" -maxdepth 4 \( -iname "*openwrt*sdk*" -o -iname "staging_dir" \) 2>/dev/null | head -20 || true

section "Repository checks"
GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go test ./internal/... >/tmp/tapx-env-go-test.log
echo "go internal tests: ok"
make -C core/fastpath >/tmp/tapx-env-fastpath.log
echo "fastpath build: ok"

if (( failures > 0 )); then
  echo "environment check failed with $failures problem(s)" >&2
  exit 1
fi

echo "environment check: ok"
