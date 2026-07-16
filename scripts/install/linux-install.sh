#!/usr/bin/env bash
set -euo pipefail

repo="${TAPX_REPO:-VAMPIRE0924/TapX}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." 2>/dev/null && pwd || true)"
build_dir="${TAPX_BUILD_DIR:-${repo_root:-/tmp}/build/linux-amd64}"
prefix="${TAPX_PREFIX:-/usr/local}"
sysconfdir="${TAPX_SYSCONFDIR:-/etc/tapx}"
unit_dir="${TAPX_SYSTEMD_UNIT_DIR:-/etc/systemd/system}"
service_name="${TAPX_SERVICE_NAME:-tapx-panel.service}"
db_path_default="${TAPX_DB_PATH:-/var/lib/tapx/tapx.db}"
systemctl_cmd="${TAPX_SYSTEMCTL:-systemctl}"

green='\033[0;32m'
yellow='\033[0;33m'
red='\033[0;31m'
blue='\033[0;34m'
plain='\033[0m'

text() {
  if [[ "${TAPX_LANG:-en}" == "zh" ]]; then
    printf '%s' "$2"
  else
    printf '%s' "$1"
  fi
}

has_tty() {
  [[ "${TAPX_NONINTERACTIVE:-0}" != "1" && -r /dev/tty ]]
}

read_value() {
  local __name="$1" __prompt="$2" __secret="${3:-0}" __value=""
  if has_tty; then
    if [[ "$__secret" == "1" ]]; then
      read -r -s -p "$__prompt" __value </dev/tty
      printf '\n' >/dev/tty
    else
      read -r -p "$__prompt" __value </dev/tty
    fi
  else
    if [[ "$__secret" == "1" ]]; then
      read -r -s -p "$__prompt" __value
      printf '\n'
    else
      read -r -p "$__prompt" __value
    fi
  fi
  printf -v "$__name" '%s' "$__value"
}

choose_language() {
  if [[ "${TAPX_LANG:-}" == "en" || "${TAPX_LANG:-}" == "zh" ]]; then
    return
  fi
  printf '1,English (default)\n2,中文\n\n'
  local choice=""
  read_value choice '> '
  case "$choice" in
    2|zh|ZH) TAPX_LANG=zh ;;
    *) TAPX_LANG=en ;;
  esac
  export TAPX_LANG
}

need_root() {
  if [[ "$(id -u)" != "0" ]]; then
    printf '%b%s%b\n' "$red" "$(text 'Run TapX installation and management as root.' '请使用 root 权限运行 TapX 安装和管理脚本。')" "$plain" >&2
    exit 1
  fi
}

need_systemd() {
  if ! command -v "$systemctl_cmd" >/dev/null 2>&1; then
    printf '%s\n' "$(text 'This installer currently requires systemd.' '当前安装脚本需要 systemd。')" >&2
    exit 1
  fi
}

random_token() {
  local bytes="${1:-18}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr '+/' '-_' | tr -d '=\n'
  else
    head -c "$bytes" /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n'
  fi
}

random_number() {
  local min="$1" max="$2" raw
  raw="$(od -An -N4 -tu4 /dev/urandom | tr -d ' ')"
  printf '%d' "$((min + raw % (max - min + 1)))"
}

is_yes() {
  case "${1:-}" in
    y|Y|yes|YES|Yes|1|是) return 0 ;;
    *) return 1 ;;
  esac
}

env_file() {
  printf '%s/tapx.env\n' "$sysconfdir"
}

validate_env_value() {
  [[ "$1" != *$'\n'* && "$1" != *$'\r'* && "$1" != *"'"* ]]
}

write_env_file() {
  local listen="$1" base_path="$2" driver="$3" source="$4" https="$5"
  local value
  for value in "$listen" "$base_path" "$driver" "$source" "$https"; do
    if ! validate_env_value "$value"; then
      printf '%s\n' "$(text 'A setting contains an unsupported character.' '配置中包含不支持的字符。')" >&2
      return 1
    fi
  done
  install -d -m 0700 "$sysconfdir"
  umask 077
  cat >"$(env_file)" <<EOF
TAPX_DB_DRIVER='$driver'
TAPX_DB_SOURCE='$source'
TAPX_PANEL_LISTEN='$listen'
TAPX_PANEL_BASE_PATH='$base_path'
TAPX_PANEL_HTTPS='$https'
EOF
  chmod 0600 "$(env_file)"
}

load_env() {
  if [[ -f "$(env_file)" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$(env_file)"
    set +a
  fi
}

require_bundle() {
  local binary
  for binary in tapx-core tapx-panel; do
    if [[ ! -x "$build_dir/$binary" ]]; then
      printf '%s\n' "$(text "Missing $build_dir/$binary." "缺少 $build_dir/$binary。")" >&2
      exit 1
    fi
  done
}

normalize_base_path() {
  local value="$1"
  [[ "$value" == /* ]] || value="/$value"
  while [[ "$value" != "/" && "$value" == */ ]]; do
    value="${value%/}"
  done
  if [[ ! "$value" =~ ^/[A-Za-z0-9._~/-]+$ ]]; then
    return 1
  fi
  printf '%s' "$value"
}

port_available_with() {
  local binary="$1" port="$2"
  "$binary" -listen="0.0.0.0:$port" -check-listen >/dev/null 2>&1
}

choose_available_port() {
  local binary="$1" requested="$2" port attempt
  if [[ -n "$requested" ]]; then
    if [[ ! "$requested" =~ ^[0-9]+$ || "$requested" -lt 1 || "$requested" -gt 65535 ]]; then
      printf '%s\n' "$(text 'Port must be between 1 and 65535.' '端口必须在 1 到 65535 之间。')" >&2
      return 1
    fi
    if ! port_available_with "$binary" "$requested"; then
      printf '%s\n' "$(text "Port $requested is already in use." "端口 $requested 已被占用。")" >&2
      return 1
    fi
    printf '%s' "$requested"
    return
  fi
  for attempt in {1..128}; do
    port="$(random_number 20000 60000)"
    if port_available_with "$binary" "$port"; then
      printf '%s' "$port"
      return
    fi
  done
  printf '%s\n' "$(text 'Unable to find an available port.' '无法找到可用端口。')" >&2
  return 1
}

choose_database() {
  local default_driver="${1:-sqlite}" first_install="${2:-0}" choice="" dsn=""
  printf '\n%s\n\n' "$(text 'Database' '数据库选择')"
  if [[ "$default_driver" == "postgres" && "$first_install" != "1" ]]; then
    printf '1,SQLite\n2,PostgreSQL (default)\n\n'
  else
    printf '%s\n' "$(text '1,SQLite (default)' '1，SQLite （默认）')"
    printf '2,PostgreSQL\n\n'
  fi
  read_value choice '> '
  if [[ "$choice" == "2" || "$choice" == "postgres" || "$choice" == "PostgreSQL" || ( -z "$choice" && "$default_driver" == "postgres" && "$first_install" != "1" ) ]]; then
    DB_DRIVER=postgres
    read_value dsn "$(text 'PostgreSQL DSN: ' 'PostgreSQL 连接地址：')" 1
    if [[ ! "$dsn" =~ ^postgres(ql)?:// ]]; then
      printf '%s\n' "$(text 'Enter a postgres:// or postgresql:// DSN.' '请输入 postgres:// 或 postgresql:// 格式的连接地址。')" >&2
      return 1
    fi
    DB_SOURCE="$dsn"
  else
    DB_DRIVER=sqlite
    DB_SOURCE="$db_path_default"
  fi
}

choose_first_install_settings() {
  local input="" normalized=""
  choose_database sqlite 1

  printf '\n%s\n\n' "$(text 'Panel port' '配置面板端口')"
  read_value input "$(text 'Port: ' '输入端口：')"
  PANEL_PORT="$(choose_available_port "$build_dir/tapx-panel" "$input")"

  printf '\n%s\n\n' "$(text 'Panel path' '配置面板入口')"
  read_value input '/xxxxx: '
  input="${input:-/tapx-$(random_token 9)}"
  normalized="$(normalize_base_path "$input")" || {
    printf '%s\n' "$(text 'Panel path must start with / and contain URL-safe characters.' '面板入口必须以 / 开头，并且只能包含安全的 URL 字符。')" >&2
    return 1
  }
  PANEL_PATH="$normalized"

  printf '\n%s\n\n' "$(text 'Administrator username' '配置用户名')"
  read_value input "$(text 'Username: ' '输入用户名：')"
  ADMIN_USERNAME="${input:-tapx_$(random_token 6)}"

  printf '\n%s\n\n' "$(text 'Administrator password' '配置密码')"
  read_value input "$(text 'Password: ' '输入密码：')" 1
  ADMIN_PASSWORD="${input:-$(random_token 21)}"

  printf '\n%s\n\n' "$(text 'Panel certificate' '设置面板证书路径')"
  read_value PANEL_CERT_FILE "$(text 'Skip or enter certificate path: ' '跳过或输入证书路径：')"
  PANEL_KEY_FILE=""
  if [[ -n "$PANEL_CERT_FILE" ]]; then
    read_value PANEL_KEY_FILE "$(text 'Private key path: ' '私钥路径：')"
    if [[ ! -r "$PANEL_CERT_FILE" || ! -r "$PANEL_KEY_FILE" ]]; then
      printf '%s\n' "$(text 'The certificate or private key cannot be read.' '证书或私钥无法读取。')" >&2
      return 1
    fi
  fi
  PANEL_HTTPS=0
  [[ -n "$PANEL_CERT_FILE" ]] && PANEL_HTTPS=1
}

hash_password() {
  local binary="$1" password="$2"
  printf '%s' "$password" | "$binary" -hash-password-stdin
}

initialize_database() {
  local binary="$1" hash="$2"
  local -a args
  args=(
    -listen="0.0.0.0:$PANEL_PORT"
    -base-path="$PANEL_PATH"
    -init-admin
    -admin-username="$ADMIN_USERNAME"
    -admin-password-hash="$hash"
  )
  if [[ -n "$PANEL_CERT_FILE" ]]; then
    args+=( -panel-cert-file="$PANEL_CERT_FILE" -panel-key-file="$PANEL_KEY_FILE" )
  fi
  TAPX_DB_DRIVER="$DB_DRIVER" TAPX_DB_SOURCE="$DB_SOURCE" "$binary" "${args[@]}" >/dev/null
  if [[ -z "$PANEL_CERT_FILE" ]]; then
    TAPX_DB_DRIVER="$DB_DRIVER" TAPX_DB_SOURCE="$DB_SOURCE" "$binary" \
      -set-panel-endpoint \
      -listen="0.0.0.0:$PANEL_PORT" \
      -base-path="$PANEL_PATH" \
      -disable-panel-https >/dev/null
  fi
}

install_service_file() {
  install -d -m 0755 "$unit_dir"
  cat >"$unit_dir/$service_name" <<EOF
[Unit]
Description=TapX control panel and local runtime manager
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
EnvironmentFile=-$sysconfdir/tapx.env
ExecStart=$prefix/bin/tapx-panel -listen=\${TAPX_PANEL_LISTEN} -base-path=\${TAPX_PANEL_BASE_PATH}
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

open_firewall_port() {
  local port="$1"
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
    ufw allow "$port/tcp" >/dev/null
  elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="$port/tcp" >/dev/null
    firewall-cmd --reload >/dev/null
  fi
}

public_host() {
  local host=""
  if command -v hostname >/dev/null 2>&1; then
    host="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  printf '%s' "${host:-SERVER_IP}"
}

panel_url() {
  load_env
  local scheme=http
  [[ "${TAPX_PANEL_HTTPS:-0}" == "1" ]] && scheme=https
  printf '%s://%s:%s%s/\n' "$scheme" "$(public_host)" "${TAPX_PANEL_LISTEN##*:}" "${TAPX_PANEL_BASE_PATH%/}"
}

install_wizard() {
  need_root
  need_systemd
  require_bundle
  if "$systemctl_cmd" is-active --quiet "$service_name" 2>/dev/null; then
    "$systemctl_cmd" stop "$service_name"
  fi

  if [[ "${TAPX_NONINTERACTIVE:-0}" == "1" ]]; then
    DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}"
    DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}"
    PANEL_PORT="$(choose_available_port "$build_dir/tapx-panel" "${TAPX_PANEL_PORT:-}")"
    PANEL_PATH="$(normalize_base_path "${TAPX_PANEL_BASE_PATH:-/tapx-$(random_token 9)}")"
    ADMIN_USERNAME="${TAPX_ADMIN_USERNAME:-tapx_$(random_token 6)}"
    ADMIN_PASSWORD="${TAPX_ADMIN_PASSWORD:-$(random_token 21)}"
    PANEL_CERT_FILE="${TAPX_PANEL_CERT_FILE:-}"
    PANEL_KEY_FILE="${TAPX_PANEL_KEY_FILE:-}"
    PANEL_HTTPS=0
    [[ -n "$PANEL_CERT_FILE" ]] && PANEL_HTTPS=1
  else
    choose_first_install_settings
  fi

  if [[ "$DB_DRIVER" != "sqlite" && "$DB_DRIVER" != "postgres" ]]; then
    printf '%s\n' "$(text 'Database must be sqlite or postgres.' '数据库必须为 SQLite 或 PostgreSQL。')" >&2
    exit 1
  fi
  if [[ "$DB_DRIVER" == "postgres" && ! "$DB_SOURCE" =~ ^postgres(ql)?:// ]]; then
    printf '%s\n' "$(text 'PostgreSQL requires a postgres:// or postgresql:// DSN.' 'PostgreSQL 需要 postgres:// 或 postgresql:// 格式的连接地址。')" >&2
    exit 1
  fi
  if [[ "$DB_DRIVER" == "sqlite" ]]; then
    install -d -m 0700 "$(dirname "$DB_SOURCE")"
  fi
  install -d -m 0755 "$prefix/bin" /run/tapx /var/log/tapx

  local password_hash
  password_hash="$(hash_password "$build_dir/tapx-panel" "$ADMIN_PASSWORD")"
  initialize_database "$build_dir/tapx-panel" "$password_hash"

  install -m 0755 "$build_dir/tapx-core" "$prefix/bin/tapx-core"
  install -m 0755 "$build_dir/tapx-panel" "$prefix/bin/tapx-panel"
  install -m 0755 "${BASH_SOURCE[0]}" "$prefix/bin/tapx"
  write_env_file "0.0.0.0:$PANEL_PORT" "$PANEL_PATH" "$DB_DRIVER" "$DB_SOURCE" "$PANEL_HTTPS"
  install_service_file
  open_firewall_port "$PANEL_PORT"

  "$systemctl_cmd" daemon-reload
  "$systemctl_cmd" enable "$service_name" >/dev/null
  "$systemctl_cmd" restart "$service_name"

  printf '\n%b%s%b\n' "$green" "$(text 'TapX installation completed.' 'TapX 安装完成。')" "$plain"
  printf '%s %s\n' "$(text 'Panel:' '面板：')" "$(panel_url)"
  printf '%s %s\n' "$(text 'Username:' '用户名：')" "$ADMIN_USERNAME"
  printf '%s %s\n' "$(text 'Password:' '密码：')" "$ADMIN_PASSWORD"
  printf '%b%s%b\n' "$yellow" "$(text 'The generated password is shown only once.' '随机生成的密码只显示这一次。')" "$plain"
}

is_installed() {
  [[ -x "$prefix/bin/tapx-panel" && -x "$prefix/bin/tapx-core" && -f "$(env_file)" ]]
}

show_status() {
  "$systemctl_cmd" status "$service_name" --no-pager || true
  printf '\n'
  "$prefix/bin/tapx-panel" -version || true
  "$prefix/bin/tapx-core" -version || true
}

show_settings() {
  load_env
  printf '%b%s%b\n' "$blue" "$(text 'Panel settings' '面板设置')" "$plain"
  printf '%s %s\n' "$(text 'Listen:' '监听：')" "${TAPX_PANEL_LISTEN:-}"
  printf '%s %s\n' "$(text 'Path:' '入口：')" "${TAPX_PANEL_BASE_PATH:-/}"
  printf '%s %s\n' "$(text 'Database:' '数据库：')" "${TAPX_DB_DRIVER:-sqlite}"
  if [[ "${TAPX_DB_DRIVER:-sqlite}" == "sqlite" ]]; then
    printf '%s %s\n' "$(text 'Database file:' '数据库文件：')" "${TAPX_DB_SOURCE:-$db_path_default}"
  else
    printf '%s\n' "$(text 'PostgreSQL DSN: configured' 'PostgreSQL 连接地址：已配置')"
  fi
  printf '%s %s\n' "$(text 'Public URL:' '公网地址：')" "$(panel_url)"
}

modify_endpoint() {
  need_root
  load_env
  local current_port="${TAPX_PANEL_LISTEN##*:}" port_input="" path_input="" cert_mode="" cert="" key=""
  "$systemctl_cmd" stop "$service_name" || true

  read_value port_input "$(text "Panel port [$current_port]: " "面板端口 [$current_port]：")"
  if ! PANEL_PORT="$(choose_available_port "$prefix/bin/tapx-panel" "${port_input:-$current_port}")"; then
    "$systemctl_cmd" start "$service_name" || true
    return 1
  fi
  read_value path_input "$(text "Panel path [${TAPX_PANEL_BASE_PATH:-/}]: " "面板入口 [${TAPX_PANEL_BASE_PATH:-/}]：")"
  if ! PANEL_PATH="$(normalize_base_path "${path_input:-${TAPX_PANEL_BASE_PATH:-/}}")"; then
    "$systemctl_cmd" start "$service_name" || true
    printf '%s\n' "$(text 'Invalid panel path.' '面板入口格式无效。')" >&2
    return 1
  fi

  printf '\n1,%s\n2,%s\n3,%s\n' \
    "$(text 'Keep certificate setting' '保持证书设置')" \
    "$(text 'Configure certificate' '配置证书')" \
    "$(text 'Disable HTTPS' '关闭 HTTPS')"
  read_value cert_mode '> '
  local -a tls_args=()
  PANEL_HTTPS="${TAPX_PANEL_HTTPS:-0}"
  case "$cert_mode" in
    2)
      read_value cert "$(text 'Certificate path: ' '证书路径：')"
      read_value key "$(text 'Private key path: ' '私钥路径：')"
      [[ -r "$cert" && -r "$key" ]] || {
        printf '%s\n' "$(text 'The certificate or private key cannot be read.' '证书或私钥无法读取。')" >&2
        "$systemctl_cmd" start "$service_name" || true
        return 1
      }
      tls_args=( -panel-cert-file="$cert" -panel-key-file="$key" )
      PANEL_HTTPS=1
      ;;
    3)
      tls_args=( -disable-panel-https )
      PANEL_HTTPS=0
      ;;
  esac

  if ! "$prefix/bin/tapx-panel" \
      -set-panel-endpoint \
      -listen="0.0.0.0:$PANEL_PORT" \
      -base-path="$PANEL_PATH" \
      "${tls_args[@]}"; then
    "$systemctl_cmd" start "$service_name" || true
    return 1
  fi
  write_env_file "0.0.0.0:$PANEL_PORT" "$PANEL_PATH" "${TAPX_DB_DRIVER:-sqlite}" "${TAPX_DB_SOURCE:-$db_path_default}" "$PANEL_HTTPS"
  open_firewall_port "$PANEL_PORT"
  "$systemctl_cmd" restart "$service_name"
  show_settings
}

reset_credentials() {
  need_root
  load_env
  local username="" password="" hash
  read_value username "$(text 'Username: ' '用户名：')"
  username="${username:-tapx_$(random_token 6)}"
  read_value password "$(text 'Password: ' '密码：')" 1
  password="${password:-$(random_token 21)}"
  hash="$(hash_password "$prefix/bin/tapx-panel" "$password")"
  "$prefix/bin/tapx-panel" \
    -listen="${TAPX_PANEL_LISTEN}" \
    -base-path="${TAPX_PANEL_BASE_PATH}" \
    -init-admin \
    -admin-username="$username" \
    -admin-password-hash="$hash"
  "$systemctl_cmd" restart "$service_name"
  printf '%s %s\n%s %s\n' "$(text 'Username:' '用户名：')" "$username" "$(text 'Password:' '密码：')" "$password"
}

change_database() {
  need_root
  load_env
  local old_driver="${TAPX_DB_DRIVER:-sqlite}" old_source="${TAPX_DB_SOURCE:-$db_path_default}"
  local listen="${TAPX_PANEL_LISTEN}" path="${TAPX_PANEL_BASE_PATH}" https="${TAPX_PANEL_HTTPS:-0}"
  local backup
  backup="$(mktemp /tmp/tapx-database-migration.XXXXXX.db)"
  TAPX_DB_DRIVER="$old_driver" TAPX_DB_SOURCE="$old_source" "$prefix/bin/tapx-panel" -export-backup "$backup" >/dev/null
  choose_database "$old_driver" 0
  if [[ "$DB_DRIVER" == "$old_driver" && "$DB_SOURCE" == "$old_source" ]]; then
    printf '%s\n' "$(text 'Database setting is unchanged.' '数据库设置没有变化。')"
    rm -f "$backup"
    return
  fi
  "$systemctl_cmd" stop "$service_name" || true
  if ! TAPX_DB_DRIVER="$DB_DRIVER" TAPX_DB_SOURCE="$DB_SOURCE" "$prefix/bin/tapx-panel" -restore-backup "$backup" >/dev/null; then
    "$systemctl_cmd" start "$service_name" || true
    rm -f "$backup"
    printf '%s\n' "$(text 'Database migration failed; the previous database is still configured.' '数据库迁移失败，当前配置未改变。')" >&2
    return 1
  fi
  write_env_file "$listen" "$path" "$DB_DRIVER" "$DB_SOURCE" "$https"
  "$systemctl_cmd" restart "$service_name"
  rm -f "$backup"
  printf '%s\n' "$(text 'Database migrated.' '数据库迁移完成。')"
}

show_logs() {
  journalctl -u "$service_name" -n 150 --no-pager
}

download_file() {
  local url="$1" destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --connect-timeout 15 -o "$destination" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$destination" "$url"
  else
    printf '%s\n' "$(text 'curl or wget is required.' '需要安装 curl 或 wget。')" >&2
    return 1
  fi
}

self_update() {
  need_root
  local script
  script="$(mktemp /tmp/tapx-install.XXXXXX.sh)"
  download_file "https://raw.githubusercontent.com/${repo}/main/scripts/install/install.sh" "$script"
  chmod 0700 "$script"
  TAPX_LANG="$TAPX_LANG" bash "$script" install
  rm -f "$script"
}

uninstall_tapx() {
  need_root
  local confirm=""
  read_value confirm "$(text 'Uninstall TapX binaries and service? [y/N]: ' '卸载 TapX 程序和服务？[y/N]：')"
  is_yes "$confirm" || return
  "$systemctl_cmd" disable --now "$service_name" >/dev/null 2>&1 || true
  rm -f "$unit_dir/$service_name" "$prefix/bin/tapx" "$prefix/bin/tapx-core" "$prefix/bin/tapx-panel"
  "$systemctl_cmd" daemon-reload
  printf '%s\n' "$(text 'Configuration and database were preserved.' '配置和数据库已保留。')"
}

show_menu() {
  need_systemd
  while true; do
    printf '\n%bTapX%b\n' "$green" "$plain"
    printf '  1. %s\n' "$(text 'Status' '查看状态')"
    printf '  2. %s\n' "$(text 'Start' '启动面板')"
    printf '  3. %s\n' "$(text 'Stop' '停止面板')"
    printf '  4. %s\n' "$(text 'Restart' '重启面板')"
    printf '  5. %s\n' "$(text 'Show settings' '查看设置')"
    printf '  6. %s\n' "$(text 'Change panel endpoint and certificate' '修改面板入口和证书')"
    printf '  7. %s\n' "$(text 'Reset administrator credentials' '重置管理员账号密码')"
    printf '  8. %s\n' "$(text 'Change database' '更换数据库')"
    printf '  9. %s\n' "$(text 'Logs' '查看日志')"
    printf ' 10. %s\n' "$(text 'Enable autostart' '启用开机启动')"
    printf ' 11. %s\n' "$(text 'Disable autostart' '关闭开机启动')"
    printf ' 12. %s\n' "$(text 'Update or reinstall' '更新或重新安装')"
    printf ' 13. %s\n' "$(text 'Uninstall' '卸载')"
    printf '  0. %s\n' "$(text 'Exit' '退出')"
    local choice=""
    read_value choice '> '
    case "$choice" in
      1) show_status ;;
      2) need_root; "$systemctl_cmd" start "$service_name" ;;
      3) need_root; "$systemctl_cmd" stop "$service_name" ;;
      4) need_root; "$systemctl_cmd" restart "$service_name" ;;
      5) show_settings ;;
      6) modify_endpoint ;;
      7) reset_credentials ;;
      8) change_database ;;
      9) show_logs ;;
      10) need_root; "$systemctl_cmd" enable "$service_name" ;;
      11) need_root; "$systemctl_cmd" disable "$service_name" ;;
      12) self_update; return ;;
      13) uninstall_tapx; return ;;
      0) return ;;
      *) printf '%s\n' "$(text 'Invalid option.' '无效选项。')" ;;
    esac
  done
}

usage() {
  cat <<EOF
tapx [menu|status|start|stop|restart|settings|set-panel|set-auth|set-database|logs|enable|disable|update|uninstall]
EOF
}

main() {
  choose_language
  local command="${1:-}"
  case "$command" in
    "") if is_installed; then show_menu; else install_wizard; fi ;;
    install|reinstall) install_wizard ;;
    menu) show_menu ;;
    status) show_status ;;
    start) need_root; "$systemctl_cmd" start "$service_name" ;;
    stop) need_root; "$systemctl_cmd" stop "$service_name" ;;
    restart) need_root; "$systemctl_cmd" restart "$service_name" ;;
    settings) show_settings ;;
    set-panel) modify_endpoint ;;
    set-auth) reset_credentials ;;
    set-database) change_database ;;
    logs) show_logs ;;
    enable) need_root; "$systemctl_cmd" enable "$service_name" ;;
    disable) need_root; "$systemctl_cmd" disable "$service_name" ;;
    update) self_update ;;
    uninstall) uninstall_tapx ;;
    help|-h|--help) usage ;;
    *) usage >&2; return 2 ;;
  esac
}

main "$@"
