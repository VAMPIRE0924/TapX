#!/usr/bin/env bash
set -euo pipefail

repo="${TAPX_REPO:-VAMPIRE0924/TapX}"
version="${TAPX_VERSION:-latest}"
prefix="${TAPX_PREFIX:-/usr/local}"
sysconfdir="${TAPX_SYSCONFDIR:-/etc/tapx}"
unit_dir="${TAPX_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
panel_listen="${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
panel_base_path="${TAPX_PANEL_BASE_PATH:-}"
db_path="${TAPX_DB_PATH:-/var/lib/tapx/tapx.db}"
admin_username="${TAPX_ADMIN_USERNAME:-admin}"
admin_password="${TAPX_ADMIN_PASSWORD:-}"
enable_service="${TAPX_ENABLE_SERVICE:-1}"
start_service="${TAPX_START_SERVICE:-1}"

usage() {
  cat <<'EOF'
Usage: install.sh [--version VERSION] [--listen ADDR] [--base-path PATH] [--prefix DIR] [--no-enable] [--no-start]

Environment variables:
  TAPX_VERSION             latest, v0.1.0, or 0.1.0
  TAPX_PANEL_LISTEN        default 127.0.0.1:8080
  TAPX_PANEL_BASE_PATH     random when empty
  TAPX_ADMIN_USERNAME      default admin
  TAPX_ADMIN_PASSWORD      random when empty
EOF
}

while (($# > 0)); do
  case "$1" in
    --version)
      version="$2"
      shift 2
      ;;
    --listen)
      panel_listen="$2"
      shift 2
      ;;
    --base-path)
      panel_base_path="$2"
      shift 2
      ;;
    --prefix)
      prefix="$2"
      shift 2
      ;;
    --no-enable)
      enable_service=0
      shift
      ;;
    --no-start)
      start_service=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

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
    ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    echo "current public Linux release supports amd64 only" >&2
    exit 1
    ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need tar
need install

download() {
  local url="$1"
  local out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 15 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  else
    echo "missing required command: curl or wget" >&2
    exit 1
  fi
}

random_token() {
  local bytes="${1:-18}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr '+/' '-_' | tr -d '='
  else
    head -c "$bytes" /dev/urandom | base64 | tr '+/' '-_' | tr -d '='
  fi
}

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

bundle_dir="$tmp/tapx-linux-amd64"
if [[ ! -d "$bundle_dir" ]]; then
  bundle_dir="$(find "$tmp" -type f -name tapx-core -exec dirname {} \; | head -n 1)"
fi
if [[ -z "$bundle_dir" || ! -x "$bundle_dir/tapx-core" || ! -x "$bundle_dir/tapx-panel" ]]; then
  echo "invalid TapX Linux package: missing tapx-core or tapx-panel" >&2
  exit 1
fi

first_install=0
if [[ ! -f "$sysconfdir/tapx.env" ]]; then
  first_install=1
  if [[ -z "$admin_password" ]]; then
    admin_password="$(random_token 18)"
  fi
  if [[ -z "$panel_base_path" || "$panel_base_path" == "/" ]]; then
    panel_base_path="/tapx-$(random_token 9)"
  fi
  if [[ "$panel_base_path" != /* ]]; then
    panel_base_path="/$panel_base_path"
  fi
  panel_base_path="${panel_base_path%/}"
fi

install -d -m 0755 "$prefix/bin" "$sysconfdir" "$(dirname "$db_path")" /var/log/tapx /run/tapx "$unit_dir"
install -m 0755 "$bundle_dir/tapx-core" "$prefix/bin/tapx-core"
install -m 0755 "$bundle_dir/tapx-panel" "$prefix/bin/tapx-panel"

if ((first_install)); then
  cat >"$sysconfdir/tapx.env" <<EOF
TAPX_DB_PATH=$db_path
TAPX_PANEL_LISTEN=$panel_listen
TAPX_PANEL_BASE_PATH=$panel_base_path
EOF
  chmod 0644 "$sysconfdir/tapx.env"
else
  echo "kept existing $sysconfdir/tapx.env"
fi

cat >"$unit_dir/tapx-panel.service" <<EOF
[Unit]
Description=TapX panel
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
EnvironmentFile=-$sysconfdir/tapx.env
ExecStart=$prefix/bin/tapx-panel -db=\${TAPX_DB_PATH} -listen=\${TAPX_PANEL_LISTEN} -base-path=\${TAPX_PANEL_BASE_PATH}
Restart=on-failure
RestartSec=2s
LimitNOFILE=1048576
User=root
Group=root
RuntimeDirectory=tapx
StateDirectory=tapx
LogsDirectory=tapx
ReadWritePaths=$sysconfdir /run/tapx /var/lib/tapx /var/log/tapx
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=false
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_SETPCAP CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$unit_dir/tapx-panel.service"

set -a
# shellcheck disable=SC1090
. "$sysconfdir/tapx.env"
set +a

if ((first_install)); then
  "$prefix/bin/tapx-panel" \
    -db="${TAPX_DB_PATH:-$db_path}" \
    -listen="${TAPX_PANEL_LISTEN:-$panel_listen}" \
    -base-path="${TAPX_PANEL_BASE_PATH:-$panel_base_path}" \
    -init-admin \
    -admin-username="$admin_username" \
    -admin-password="$admin_password"
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  if [[ "$enable_service" == "1" ]]; then
    systemctl enable tapx-panel.service
  fi
  if [[ "$start_service" == "1" ]]; then
    systemctl restart tapx-panel.service
  fi
else
  echo "systemctl not found; service file installed but not started"
fi

echo
echo "TapX installed"
echo "binaries: $prefix/bin/tapx-core, $prefix/bin/tapx-panel"
echo "config: $sysconfdir/tapx.env"
echo "service: tapx-panel.service"
if ((first_install)); then
  echo "panel url: http://${TAPX_PANEL_LISTEN:-$panel_listen}${TAPX_PANEL_BASE_PATH:-$panel_base_path}/"
  echo "admin username: $admin_username"
  echo "admin password: $admin_password"
else
  echo "existing panel configuration preserved"
fi
