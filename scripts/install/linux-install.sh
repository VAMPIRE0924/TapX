#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
build_dir="${TAPX_BUILD_DIR:-$repo_root/build/linux-amd64}"
prefix="${TAPX_PREFIX:-/usr/local}"
sysconfdir="${TAPX_SYSCONFDIR:-/etc/tapx}"
unit_dir="${TAPX_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
enable_service=0
start_service=0
admin_username="${TAPX_ADMIN_USERNAME:-admin}"
admin_password="${TAPX_ADMIN_PASSWORD:-}"
panel_base_path="${TAPX_PANEL_BASE_PATH:-}"
panel_listen="${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
db_path="${TAPX_DB_PATH:-/var/lib/tapx/tapx.db}"
init_admin=1

usage() {
  cat <<'EOF'
Usage: linux-install.sh [--build-dir DIR] [--prefix DIR] [--sysconfdir DIR] [--unit-dir DIR] [--admin-username USER] [--admin-password PASS] [--base-path PATH] [--listen ADDR] [--db PATH] [--no-init-admin] [--enable] [--start]

Installs tapx-core, tapx-panel, the default environment file, and the
tapx-panel systemd service. The script does not enable or start the service
unless --enable or --start is provided. On first install it initializes panel
login with a random password and random Web base path unless those values are
provided explicitly.
EOF
}

while (($# > 0)); do
  case "$1" in
    --build-dir)
      build_dir="$2"
      shift 2
      ;;
    --prefix)
      prefix="$2"
      shift 2
      ;;
    --sysconfdir)
      sysconfdir="$2"
      shift 2
      ;;
    --unit-dir)
      unit_dir="$2"
      shift 2
      ;;
    --admin-username)
      admin_username="$2"
      shift 2
      ;;
    --admin-password)
      admin_password="$2"
      shift 2
      ;;
    --base-path)
      panel_base_path="$2"
      shift 2
      ;;
    --listen)
      panel_listen="$2"
      shift 2
      ;;
    --db)
      db_path="$2"
      shift 2
      ;;
    --no-init-admin)
      init_admin=0
      shift
      ;;
    --enable)
      enable_service=1
      shift
      ;;
    --start)
      start_service=1
      enable_service=1
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

random_token() {
  local bytes="${1:-18}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr '+/' '-_' | tr -d '='
  else
    head -c "$bytes" /dev/urandom | base64 | tr '+/' '-_' | tr -d '='
  fi
}

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

if [[ "$(id -u)" != "0" ]]; then
  echo "linux-install.sh must run as root" >&2
  exit 1
fi

for bin in tapx-core tapx-panel; do
  if [[ ! -x "$build_dir/$bin" ]]; then
    echo "missing $build_dir/$bin" >&2
    echo "run: ./scripts/build/linux-amd64.sh" >&2
    exit 1
  fi
done

install -d -m 0755 "$prefix/bin" "$sysconfdir" "$(dirname "$db_path")" /var/log/tapx /run/tapx "$unit_dir"
install -m 0755 "$build_dir/tapx-core" "$prefix/bin/tapx-core"
install -m 0755 "$build_dir/tapx-panel" "$prefix/bin/tapx-panel"

if [[ ! -f "$sysconfdir/tapx.env" ]]; then
  cat >"$sysconfdir/tapx.env" <<EOF
TAPX_DB_PATH=$db_path
TAPX_PANEL_LISTEN=$panel_listen
TAPX_PANEL_BASE_PATH=$panel_base_path
EOF
  chmod 0644 "$sysconfdir/tapx.env"
else
  install -m 0644 "$repo_root/packaging/systemd/tapx.env" "$sysconfdir/tapx.env.example"
  echo "kept existing $sysconfdir/tapx.env; wrote tapx.env.example"
  init_admin=0
fi

install -m 0644 "$repo_root/packaging/systemd/tapx-panel.service" "$unit_dir/tapx-panel.service"

set -a
# shellcheck disable=SC1090
. "$sysconfdir/tapx.env"
set +a

if ((init_admin)); then
  "$prefix/bin/tapx-panel" \
    -db="${TAPX_DB_PATH:-/var/lib/tapx/tapx.db}" \
    -listen="${TAPX_PANEL_LISTEN:-$panel_listen}" \
    -base-path="${TAPX_PANEL_BASE_PATH:-$panel_base_path}" \
    -init-admin \
    -admin-username="$admin_username" \
    -admin-password="$admin_password"
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  if ((enable_service)); then
    systemctl enable tapx-panel.service
  fi
  if ((start_service)); then
    systemctl restart tapx-panel.service
  fi
fi

echo "installed tapx-core and tapx-panel to $prefix/bin"
echo "config: $sysconfdir/tapx.env"
echo "service: $unit_dir/tapx-panel.service"
if ((init_admin)); then
  echo "panel url: http://${TAPX_PANEL_LISTEN:-$panel_listen}${TAPX_PANEL_BASE_PATH:-$panel_base_path}/"
  echo "admin username: $admin_username"
  echo "admin password: $admin_password"
else
  echo "existing panel configuration preserved"
fi
