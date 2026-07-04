#!/usr/bin/env bash
set -euo pipefail

green='\033[0;32m'
yellow='\033[0;33m'
red='\033[0;31m'
blue='\033[0;34m'
plain='\033[0m'

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
build_dir="${TAPX_BUILD_DIR:-$repo_root/build/linux-amd64}"
prefix="${TAPX_PREFIX:-/usr/local}"
sysconfdir="${TAPX_SYSCONFDIR:-/etc/tapx}"
unit_dir="${TAPX_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
service_name="${TAPX_SERVICE_NAME:-tapx-panel.service}"
db_path_default="${TAPX_DB_PATH:-/var/lib/tapx/tapx.db}"

need_root() {
  if [[ "$(id -u)" != "0" ]]; then
    echo -e "${red}TapX installer must run as root.${plain}" >&2
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

read_prompt() {
  local __var="$1" __prompt="$2" __secret="${3:-0}" __value=""
  if [[ "$__secret" == "1" ]]; then
    if [[ -r /dev/tty ]]; then
      read -r -s -p "$__prompt" __value </dev/tty
    else
      read -r -s -p "$__prompt" __value
    fi
    echo
  else
    if [[ -r /dev/tty ]]; then
      read -r -p "$__prompt" __value </dev/tty
    else
      read -r -p "$__prompt" __value
    fi
  fi
  printf -v "$__var" '%s' "$__value"
}

prompt_default() {
  local var="$1" prompt="$2" default="$3"
  local value=""
  read_prompt value "$prompt [$default]: "
  printf -v "$var" '%s' "${value:-$default}"
}

prompt_secret_default() {
  local var="$1" prompt="$2" default="$3"
  local value=""
  read_prompt value "$prompt [empty=random]: " 1
  printf -v "$var" '%s' "${value:-$default}"
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

normalize_base_path() {
  local path="$1"
  if [[ -z "$path" || "$path" == "/" ]]; then
    path="/tapx-$(random_token 9)"
  fi
  [[ "$path" == /* ]] || path="/$path"
  path="${path%/}"
  [[ -n "$path" ]] || path="/"
  echo "$path"
}

env_file() {
  echo "$sysconfdir/tapx.env"
}

result_file() {
  echo "$sysconfdir/install-result.env"
}

load_env() {
  local file
  file="$(env_file)"
  if [[ -f "$file" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$file"
    set +a
  fi
}

panel_listen() {
  load_env
  echo "${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
}

panel_base_path() {
  load_env
  echo "${TAPX_PANEL_BASE_PATH:-}"
}

panel_db_path() {
  load_env
  echo "${TAPX_DB_PATH:-$db_path_default}"
}

panel_url_host() {
  local listen="$1"
  local host="${listen%:*}"
  local port="${listen##*:}"
  if [[ "$host" == "0.0.0.0" || "$host" == "::" || "$host" == "[::]" || -z "$host" ]]; then
    host="127.0.0.1"
  fi
  echo "http://${host}:${port}$(panel_base_path)/"
}

write_env_file() {
  local listen="$1" base_path="$2" db_path="$3"
  install -d -m 0755 "$sysconfdir"
  cat >"$(env_file)" <<EOF
TAPX_DB_PATH=$db_path
TAPX_PANEL_LISTEN=$listen
TAPX_PANEL_BASE_PATH=$base_path
EOF
  chmod 0644 "$(env_file)"
}

write_result_file() {
  local username="$1" password="$2" listen="$3" base_path="$4"
  local result
  result="$(result_file)"
  install -d -m 0755 "$sysconfdir"
  umask 077
  cat >"$result" <<EOF
TAPX_ADMIN_USERNAME=$(printf '%q' "$username")
TAPX_ADMIN_PASSWORD=$(printf '%q' "$password")
TAPX_PANEL_LISTEN=$(printf '%q' "$listen")
TAPX_PANEL_BASE_PATH=$(printf '%q' "$base_path")
TAPX_PANEL_URL=$(printf '%q' "$(panel_url_host "$listen")")
EOF
  chmod 0600 "$result"
}

install_service_file() {
  install -d -m 0755 "$unit_dir"
  cat >"$unit_dir/$service_name" <<EOF
[Unit]
Description=TapX panel and runtime manager
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
  chmod 0644 "$unit_dir/$service_name"
}

install_cli_entry() {
  install -m 0755 "$0" "$prefix/bin/tapx"
}

require_binaries() {
  for bin in tapx-core tapx-panel; do
    if [[ ! -x "$build_dir/$bin" ]]; then
      echo -e "${red}Missing $build_dir/$bin.${plain}" >&2
      echo "Build first: make build-linux-amd64" >&2
      exit 1
    fi
  done
}

systemctl_if_available() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl "$@"
  else
    return 127
  fi
}

init_admin() {
  local username="$1" password="$2" listen="$3"
  "$prefix/bin/tapx-panel" \
    -db="$(panel_db_path)" \
    -listen="$listen" \
    -base-path="$(panel_base_path)" \
    -init-admin \
    -admin-username="$username" \
    -admin-password="$password"
}

install_wizard() {
  need_root
  require_binaries

  echo -e "${green}TapX Linux install wizard${plain}"
  echo
  local host port listen base_path username password db_path enable start
  prompt_default host "Panel listen address" "${TAPX_PANEL_HOST:-127.0.0.1}"
  prompt_default port "Panel listen port" "${TAPX_PANEL_PORT:-8080}"
  listen="${host}:${port}"
  prompt_default base_path "Panel URL path" "${TAPX_PANEL_BASE_PATH:-/tapx-$(random_token 9)}"
  base_path="$(normalize_base_path "$base_path")"
  prompt_default username "Admin username" "${TAPX_ADMIN_USERNAME:-admin}"
  prompt_secret_default password "Admin password" "${TAPX_ADMIN_PASSWORD:-$(random_token 18)}"
  prompt_default db_path "SQLite database path" "$db_path_default"
  prompt_default enable "Enable systemd autostart" "y"
  prompt_default start "Start panel now" "y"

  install -d -m 0755 "$prefix/bin" "$(dirname "$db_path")" /var/log/tapx /run/tapx
  install -m 0755 "$build_dir/tapx-core" "$prefix/bin/tapx-core"
  install -m 0755 "$build_dir/tapx-panel" "$prefix/bin/tapx-panel"
  write_env_file "$listen" "$base_path" "$db_path"
  install_service_file
  install_cli_entry
  load_env
  init_admin "$username" "$password" "$listen"
  write_result_file "$username" "$password" "$listen" "$base_path"

  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    [[ "$enable" =~ ^[Yy] ]] && systemctl enable "$service_name"
    [[ "$start" =~ ^[Yy] ]] && systemctl restart "$service_name"
  fi

  echo
  echo -e "${green}TapX installed.${plain}"
  show_settings
  echo -e "${yellow}Admin password is stored once in $(result_file) with mode 600.${plain}"
}

is_installed() {
  [[ -x "$prefix/bin/tapx-panel" && -x "$prefix/bin/tapx-core" && -f "$(env_file)" ]]
}

show_status() {
  echo -e "${blue}Panel service:${plain}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl status "$service_name" --no-pager || true
  else
    pgrep -a tapx-panel || true
  fi
  echo
  echo -e "${blue}Binaries:${plain}"
  "$prefix/bin/tapx-panel" -version 2>/dev/null || true
  "$prefix/bin/tapx-core" -version 2>/dev/null || true
}

show_settings() {
  load_env
  echo -e "${blue}Panel settings:${plain}"
  echo "  listen:    ${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
  echo "  path:      ${TAPX_PANEL_BASE_PATH:-/}"
  echo "  db:        ${TAPX_DB_PATH:-$db_path_default}"
  echo "  url:       $(panel_url_host "${TAPX_PANEL_LISTEN:-127.0.0.1:8080}")"
  echo "  env:       $(env_file)"
  echo "  result:    $(result_file)"
}

modify_panel_settings() {
  need_root
  load_env
  local current_listen="${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
  local current_host="${current_listen%:*}"
  local current_port="${current_listen##*:}"
  local current_base="${TAPX_PANEL_BASE_PATH:-/}"
  local current_db="${TAPX_DB_PATH:-$db_path_default}"
  local host port listen base_path db_path username password change_auth

  echo -e "${green}Modify TapX panel settings${plain}"
  prompt_default host "Panel listen address" "$current_host"
  prompt_default port "Panel listen port" "$current_port"
  listen="${host}:${port}"
  prompt_default base_path "Panel URL path" "$current_base"
  base_path="$(normalize_base_path "$base_path")"
  prompt_default db_path "SQLite database path" "$current_db"
  prompt_default change_auth "Change admin username/password" "n"

  write_env_file "$listen" "$base_path" "$db_path"
  if [[ "$change_auth" =~ ^[Yy] ]]; then
    prompt_default username "Admin username" "${TAPX_ADMIN_USERNAME:-admin}"
    prompt_secret_default password "Admin password" "$(random_token 18)"
    init_admin "$username" "$password" "$listen"
    write_result_file "$username" "$password" "$listen" "$base_path"
  fi

  if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl restart "$service_name" || true
  fi
  show_settings
}

api_base_url() {
  load_env
  local listen="${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
  local host="${listen%:*}"
  local port="${listen##*:}"
  [[ "$host" == "0.0.0.0" || "$host" == "::" || "$host" == "[::]" || -z "$host" ]] && host="127.0.0.1"
  echo "http://${host}:${port}${TAPX_PANEL_BASE_PATH:-}"
}

api_call() {
  local method="$1" path="$2"
  local base cookie result username password
  base="$(api_base_url)"
  cookie="$(mktemp)"
  trap 'rm -f "$cookie"' RETURN
  username=""
  password=""
  if [[ -r "$(result_file)" ]]; then
    # shellcheck disable=SC1090
    . "$(result_file)"
    username="${TAPX_ADMIN_USERNAME:-}"
    password="${TAPX_ADMIN_PASSWORD:-}"
  fi
  if [[ -n "$username" && -n "$password" ]]; then
    local login_payload
    login_payload="{\"username\":\"$(json_escape "$username")\",\"password\":\"$(json_escape "$password")\"}"
    curl -fsS -c "$cookie" -H 'content-type: application/json' \
      -d "$login_payload" \
      "$base/api/auth/login" >/dev/null 2>&1 || true
  fi
  result="$(curl -fsS -b "$cookie" -X "$method" "$base$path" 2>&1)" || {
    echo -e "${red}API call failed:${plain} $result" >&2
    return 1
  }
  echo "$result"
}

core_state() {
  api_call GET /api/runtime/state
}

core_apply() {
  api_call POST /api/runtime/apply
}

core_stop() {
  api_call POST /api/runtime/stop
}

uninstall_tapx() {
  need_root
  local confirm=""
  read_prompt confirm "Uninstall TapX panel, binaries, and systemd unit? [y/N]: "
  [[ "$confirm" =~ ^[Yy] ]] || return 0
  systemctl_if_available stop "$service_name" >/dev/null 2>&1 || true
  systemctl_if_available disable "$service_name" >/dev/null 2>&1 || true
  rm -f "$unit_dir/$service_name" "$prefix/bin/tapx" "$prefix/bin/tapx-core" "$prefix/bin/tapx-panel"
  systemctl_if_available daemon-reload >/dev/null 2>&1 || true
  echo -e "${yellow}Kept data directory and config:${plain} $sysconfdir /var/lib/tapx /var/log/tapx"
}

show_logs() {
  if command -v journalctl >/dev/null 2>&1; then
    journalctl -u "$service_name" -n 120 --no-pager
  else
    tail -n 120 /var/log/tapx/* 2>/dev/null || true
  fi
}

show_menu() {
  while true; do
    echo
    echo -e "${green}TapX management menu${plain}"
    echo "  1. Install / reinstall"
    echo "  2. Panel status"
    echo "  3. Start panel"
    echo "  4. Stop panel"
    echo "  5. Restart panel"
    echo "  6. Core runtime state"
    echo "  7. Core runtime apply"
    echo "  8. Core runtime stop"
    echo "  9. View panel parameters"
    echo " 10. Modify panel parameters"
    echo " 11. View logs"
    echo " 12. Enable autostart"
    echo " 13. Disable autostart"
    echo " 14. Uninstall"
    echo "  0. Exit"
    read_prompt choice "Choose: "
    case "$choice" in
      1) install_wizard ;;
      2) show_status ;;
      3) need_root; systemctl_if_available start "$service_name" || true ;;
      4) need_root; systemctl_if_available stop "$service_name" || true ;;
      5) need_root; systemctl_if_available restart "$service_name" || true ;;
      6) core_state || true ;;
      7) core_apply || true ;;
      8) core_stop || true ;;
      9) show_settings ;;
      10) modify_panel_settings ;;
      11) show_logs ;;
      12) need_root; systemctl_if_available enable "$service_name" || true ;;
      13) need_root; systemctl_if_available disable "$service_name" || true ;;
      14) uninstall_tapx ;;
      0) exit 0 ;;
      *) echo -e "${red}Invalid option.${plain}" ;;
    esac
  done
}

usage() {
  cat <<EOF
Usage: tapx [command]

Commands:
  install       Run install wizard
  menu          Show management menu
  status        Show panel status and binary versions
  start         Start panel service
  stop          Stop panel service
  restart       Restart panel service
  core-state    Show TapX runtime state through panel API
  core-apply    Apply TapX runtime through panel API
  core-stop     Stop TapX runtime through panel API
  settings      View panel parameters
  set-panel     Modify panel parameters
  logs          Show panel logs
  enable        Enable autostart
  disable       Disable autostart
  uninstall     Uninstall binaries and service, keep data
EOF
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    "" )
      if is_installed; then show_menu; else install_wizard; fi
      ;;
    install) shift; install_wizard "$@" ;;
    menu) show_menu ;;
    status) show_status ;;
    start) need_root; systemctl_if_available start "$service_name" ;;
    stop) need_root; systemctl_if_available stop "$service_name" ;;
    restart) need_root; systemctl_if_available restart "$service_name" ;;
    core-state) core_state ;;
    core-apply) core_apply ;;
    core-stop) core_stop ;;
    settings) show_settings ;;
    set-panel) modify_panel_settings ;;
    logs) show_logs ;;
    enable) need_root; systemctl_if_available enable "$service_name" ;;
    disable) need_root; systemctl_if_available disable "$service_name" ;;
    uninstall) uninstall_tapx ;;
    -h|--help|help) usage ;;
    *) echo "unknown command: $cmd" >&2; usage >&2; exit 2 ;;
  esac
}

main "$@"
