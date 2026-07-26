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
  [[ "${TAPX_NONINTERACTIVE:-0}" != "1" && ( -t 0 || -t 1 || -t 2 ) && -r /dev/tty ]]
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

ensure_runtime_dependencies() {
  local missing=() command
  for command in ip tc dnsmasq dhcrelay; do
    command -v "$command" >/dev/null 2>&1 || missing+=("$command")
  done
  if ! command -v nft >/dev/null 2>&1 && ! command -v iptables >/dev/null 2>&1; then
    missing+=("nftables/iptables")
  fi
  ((${#missing[@]})) || return 0

  printf '%s %s\n' "$(text 'Installing runtime dependencies:' '正在安装运行依赖：')" "${missing[*]}"
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates iproute2 nftables iptables dnsmasq-base isc-dhcp-relay
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y ca-certificates iproute nftables iptables dnsmasq dhcp-relay
  elif command -v yum >/dev/null 2>&1; then
    yum install -y ca-certificates iproute nftables iptables dnsmasq dhcp-relay
  elif command -v zypper >/dev/null 2>&1; then
    zypper --non-interactive install ca-certificates iproute2 nftables iptables dnsmasq dhcp-relay
  elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm ca-certificates iproute2 nftables iptables-nft dnsmasq dhcp
  else
    printf '%s %s\n' "$(text 'Install these commands first:' '请先安装这些命令：')" "${missing[*]}" >&2
    return 1
  fi

  missing=()
  for command in ip tc dnsmasq dhcrelay; do
    command -v "$command" >/dev/null 2>&1 || missing+=("$command")
  done
  if ! command -v nft >/dev/null 2>&1 && ! command -v iptables >/dev/null 2>&1; then
    missing+=("nftables/iptables")
  fi
  if ((${#missing[@]})); then
    printf '%s %s\n' "$(text 'Missing required runtime commands:' '仍缺少运行命令：')" "${missing[*]}" >&2
    return 1
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
  for value in "$listen" "$base_path" "$driver" "$source" "$https" "${TAPX_PUBLIC_HOST:-}"; do
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
TAPX_PUBLIC_HOST='${TAPX_PUBLIC_HOST:-}'
TAPX_LANG='${TAPX_LANG:-en}'
EOF
  chmod 0600 "$(env_file)"
}

set_language() {
  local choice="" next="en" file
  printf '\n1,English\n2,中文\n\n'
  read_value choice '> '
  case "$choice" in
    2|zh|ZH) next=zh ;;
    1|en|EN|'') next=en ;;
    *)
      printf '%s\n' "$(text 'Invalid option.' '无效选项。')"
      return 1
      ;;
  esac
  TAPX_LANG="$next"
  export TAPX_LANG
  file="$(env_file)"
  if [[ -f "$file" ]]; then
    if grep -q '^TAPX_LANG=' "$file"; then
      sed -i "s/^TAPX_LANG=.*/TAPX_LANG='$TAPX_LANG'/" "$file"
    else
      printf "TAPX_LANG='%s'\n" "$TAPX_LANG" >>"$file"
    fi
    chmod 0600 "$file"
  fi
  printf '%s\n' "$(text 'Language changed to English.' '界面语言已切换为中文。')"
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
  if [[ -n "$PANEL_CERT_FILE" ]]; then
    PANEL_HTTPS=1
  fi
}

validate_certificate_paths() {
  local cert="${1:-}" key="${2:-}" resolved
  if [[ -z "$cert" && -z "$key" ]]; then
    return 0
  fi
  if [[ ! -r "$cert" || ! -r "$key" ]]; then
    printf '%s\n' "$(text 'The certificate or private key cannot be read.' '证书或私钥无法读取。')" >&2
    return 1
  fi
  for resolved in "$(readlink -f "$cert")" "$(readlink -f "$key")"; do
    case "$resolved" in
      /tmp/*|/run/*|/dev/shm/*)
        printf '%s\n' "$(text 'Certificate files must use a persistent path such as /etc/letsencrypt.' '证书文件必须使用持久路径，例如 /etc/letsencrypt。')" >&2
        return 1
        ;;
    esac
  done
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
ExecStart=$prefix/bin/tapx-panel
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
ProtectHome=read-only
PrivateTmp=true
NoNewPrivileges=false
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SETPCAP CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER

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
  if [[ -n "${TAPX_PUBLIC_HOST:-}" ]]; then
    printf '%s' "$TAPX_PUBLIC_HOST"
    return
  fi
  if command -v curl >/dev/null 2>&1; then
    host="$(curl -4 -fsS --max-time 4 https://api.ipify.org 2>/dev/null || true)"
  elif command -v wget >/dev/null 2>&1; then
    host="$(wget -4 -qO- -T 4 https://api.ipify.org 2>/dev/null || true)"
  fi
  if command -v hostname >/dev/null 2>&1; then
    host="${host:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
  fi
  printf '%s' "${host:-SERVER_IP}"
}

certificate_public_host() {
  local cert="${1:-}"
  [[ -n "$cert" && -r "$cert" ]] || return 0
  command -v openssl >/dev/null 2>&1 || return 0
  openssl x509 -in "$cert" -noout -ext subjectAltName 2>/dev/null |
    tr ',' '\n' |
    sed -n -e 's/^[[:space:]]*IP Address://p' -e 's/^[[:space:]]*DNS://p' |
    head -n 1
}

panel_url() {
  load_env
  local scheme=http host
  [[ "${TAPX_PANEL_HTTPS:-0}" == "1" ]] && scheme=https
  host="$(public_host)"
  if [[ "$host" == *:* && "$host" != \[*\] ]]; then host="[$host]"; fi
  printf '%s://%s:%s%s/\n' "$scheme" "$host" "${TAPX_PANEL_LISTEN##*:}" "${TAPX_PANEL_BASE_PATH%/}"
}

wait_for_service() {
  local stable=0 attempt
  for attempt in {1..20}; do
    sleep 0.5
    if "$systemctl_cmd" is-active --quiet "$service_name"; then
      stable=$((stable + 1))
      if [[ "$stable" -ge 4 ]]; then return 0; fi
    else
      stable=0
    fi
  done
  printf '%s\n' "$(text 'TapX failed to remain running after installation.' 'TapX 安装后未能保持运行。')" >&2
  "$systemctl_cmd" status "$service_name" --no-pager -l >&2 || true
  journalctl -u "$service_name" -n 20 --no-pager >&2 || true
  return 1
}

install_wizard() {
  need_root
  need_systemd
  ensure_runtime_dependencies
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
    if [[ -n "$PANEL_CERT_FILE" ]]; then
      PANEL_HTTPS=1
    fi
  else
    choose_first_install_settings
  fi

  validate_certificate_paths "$PANEL_CERT_FILE" "$PANEL_KEY_FILE"

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
  TAPX_PUBLIC_HOST="${TAPX_PUBLIC_HOST:-$(certificate_public_host "$PANEL_CERT_FILE")}"

  install -m 0755 "$build_dir/tapx-core" "$prefix/bin/tapx-core"
  install -m 0755 "$build_dir/tapx-panel" "$prefix/bin/tapx-panel"
  install -m 0755 "${BASH_SOURCE[0]}" "$prefix/bin/tapx"
  write_env_file "0.0.0.0:$PANEL_PORT" "$PANEL_PATH" "$DB_DRIVER" "$DB_SOURCE" "$PANEL_HTTPS"
  install_service_file
  open_firewall_port "$PANEL_PORT"

  "$systemctl_cmd" daemon-reload
  "$systemctl_cmd" enable "$service_name" >/dev/null
  "$systemctl_cmd" restart "$service_name"
  wait_for_service

  printf '\n%b%s%b\n' "$green" "$(text 'TapX installation completed.' 'TapX 安装完成。')" "$plain"
  printf '%s %s\n' "$(text 'Panel:' '面板：')" "$(panel_url)"
  printf '%s %s\n' "$(text 'Username:' '用户名：')" "$ADMIN_USERNAME"
  printf '%s %s\n' "$(text 'Password:' '密码：')" "$ADMIN_PASSWORD"
  printf '%b%s%b\n' "$yellow" "$(text 'The password is shown only once; store it securely.' '密码只显示这一次，请妥善保存。')" "$plain"
}

upgrade_bundle() {
  need_root
  need_systemd
  require_bundle
  if ! is_installed; then
    printf '%s\n' "$(text 'TapX is not installed; run the installer first.' 'TapX 尚未安装，请先执行安装。')" >&2
    return 1
  fi

  local backup_dir was_active=0 had_unit=0 candidate_core candidate_panel
  backup_dir="$(mktemp -d /tmp/tapx-upgrade.XXXXXX)"
  candidate_core="$backup_dir/tapx-core.new"
  candidate_panel="$backup_dir/tapx-panel.new"
  install -m 0755 "$build_dir/tapx-core" "$candidate_core"
  install -m 0755 "$build_dir/tapx-panel" "$candidate_panel"
  "$candidate_core" -version >/dev/null
  "$candidate_panel" -version >/dev/null

  cp -a "$prefix/bin/tapx-core" "$backup_dir/tapx-core"
  cp -a "$prefix/bin/tapx-panel" "$backup_dir/tapx-panel"
  cp -a "$prefix/bin/tapx" "$backup_dir/tapx"
  if [[ -f "$unit_dir/$service_name" ]]; then
    cp -a "$unit_dir/$service_name" "$backup_dir/$service_name"
    had_unit=1
  fi
  if "$systemctl_cmd" is-active --quiet "$service_name"; then
    was_active=1
    "$systemctl_cmd" stop "$service_name"
  fi

  if ! install -m 0755 "$candidate_core" "$prefix/bin/tapx-core" ||
     ! install -m 0755 "$candidate_panel" "$prefix/bin/tapx-panel"; then
    install -m 0755 "$backup_dir/tapx-core" "$prefix/bin/tapx-core"
    install -m 0755 "$backup_dir/tapx-panel" "$prefix/bin/tapx-panel"
    ((was_active)) && "$systemctl_cmd" start "$service_name" || true
    rm -rf "$backup_dir"
    return 1
  fi

  if ! install_service_file || ! "$systemctl_cmd" daemon-reload; then
    install -m 0755 "$backup_dir/tapx-core" "$prefix/bin/tapx-core"
    install -m 0755 "$backup_dir/tapx-panel" "$prefix/bin/tapx-panel"
    if ((had_unit)); then install -m 0644 "$backup_dir/$service_name" "$unit_dir/$service_name"; fi
    "$systemctl_cmd" daemon-reload || true
    ((was_active)) && "$systemctl_cmd" start "$service_name" || true
    rm -rf "$backup_dir"
    return 1
  fi

  if ((was_active)); then
    "$systemctl_cmd" start "$service_name" || true
    if ! wait_for_service; then
      "$systemctl_cmd" stop "$service_name" || true
      install -m 0755 "$backup_dir/tapx-core" "$prefix/bin/tapx-core"
      install -m 0755 "$backup_dir/tapx-panel" "$prefix/bin/tapx-panel"
      if ((had_unit)); then install -m 0644 "$backup_dir/$service_name" "$unit_dir/$service_name"; fi
      "$systemctl_cmd" daemon-reload || true
      "$systemctl_cmd" start "$service_name" || true
      rm -rf "$backup_dir"
      printf '%s\n' "$(text 'Upgrade failed; the previous binaries were restored.' '升级失败，已恢复原二进制文件。')" >&2
      return 1
    fi
  fi

  install -m 0755 "${BASH_SOURCE[0]}" "$prefix/bin/tapx"
  rm -rf "$backup_dir"
  printf '%s\n' "$(text 'TapX was upgraded without changing its database or settings.' 'TapX 已升级，数据库和现有设置保持不变。')"
  show_status
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
  local stored_listen stored_path stored_https
  stored_listen="$(TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -panel-endpoint-field listen)"
  stored_path="$(TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -panel-endpoint-field base-path)"
  stored_https="$(TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -panel-endpoint-field https)"
  TAPX_PANEL_LISTEN="$stored_listen"
  TAPX_PANEL_BASE_PATH="$stored_path"
  TAPX_PANEL_HTTPS="$stored_https"
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

check_installation() {
  need_systemd
  load_env
  local failures=0 command mode
  for command in "$prefix/bin/tapx-core" "$prefix/bin/tapx-panel" "$prefix/bin/tapx" "$(env_file)"; do
    if [[ -e "$command" ]]; then
      printf '%bOK%b  %s\n' "$green" "$plain" "$command"
    else
      printf '%bFAIL%b  %s\n' "$red" "$plain" "$command"
      failures=$((failures + 1))
    fi
  done
  if [[ -f "$(env_file)" ]]; then
    mode="$(stat -c '%a' "$(env_file)" 2>/dev/null || true)"
    if [[ "$mode" != "600" ]]; then
      printf '%bFAIL%b  %s (%s)\n' "$red" "$plain" "$(text 'Environment file permission' '环境文件权限')" "${mode:-unknown}"
      failures=$((failures + 1))
    fi
  fi
  if ! TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -check >/dev/null; then
    printf '%bFAIL%b  %s\n' "$red" "$plain" "$(text 'Database and panel configuration' '数据库与面板配置')"
    failures=$((failures + 1))
  else
    printf '%bOK%b  %s\n' "$green" "$plain" "$(text 'Database and panel configuration' '数据库与面板配置')"
  fi
  if "$systemctl_cmd" is-active --quiet "$service_name"; then
    printf '%bOK%b  %s\n' "$green" "$plain" "$(text 'Panel service is running' '面板服务正在运行')"
  else
    printf '%bFAIL%b  %s\n' "$red" "$plain" "$(text 'Panel service is not running' '面板服务未运行')"
    failures=$((failures + 1))
  fi
  if ((failures)); then
    printf '%s %d\n' "$(text 'Failed checks:' '失败检查：')" "$failures" >&2
    return 1
  fi
  printf '%s\n' "$(text 'All installation checks passed.' '安装自检全部通过。')"
}

modify_endpoint() {
  need_root
  load_env
  TAPX_PANEL_LISTEN="$(TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -panel-endpoint-field listen)"
  TAPX_PANEL_BASE_PATH="$(TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -panel-endpoint-field base-path)"
  TAPX_PANEL_HTTPS="$(TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" "$prefix/bin/tapx-panel" -panel-endpoint-field https)"
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
      if ! validate_certificate_paths "$cert" "$key"; then
        "$systemctl_cmd" start "$service_name" || true
        return 1
      fi
      tls_args=( -panel-cert-file="$cert" -panel-key-file="$key" )
      PANEL_HTTPS=1
      ;;
    3)
      tls_args=( -disable-panel-https )
      PANEL_HTTPS=0
      ;;
  esac
  if [[ -z "${TAPX_PUBLIC_HOST:-}" && -n "$cert" ]]; then
    TAPX_PUBLIC_HOST="$(certificate_public_host "$cert")"
  fi

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
  wait_for_service
  show_settings
}

reset_credentials() {
  need_root
  load_env
  local confirm="" username password hash
  read_value confirm "$(text 'Generate a new random administrator username and password? [y/N]: ' '随机重置面板用户名和密码？[y/N]：')"
  is_yes "$confirm" || return
  username="tapx_$(random_token 6)"
  password="$(random_token 21)"
  hash="$(hash_password "$prefix/bin/tapx-panel" "$password")"
  TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" \
  TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" \
  "$prefix/bin/tapx-panel" \
    -listen="${TAPX_PANEL_LISTEN}" \
    -base-path="${TAPX_PANEL_BASE_PATH}" \
    -init-admin \
    -admin-username="$username" \
    -admin-password-hash="$hash"
  "$systemctl_cmd" restart "$service_name"
  wait_for_service
  printf '\n%b%s%b\n' "$green" "$(text 'Administrator credentials were reset.' '面板用户名和密码已重置。')" "$plain"
  printf '%s %s\n%s %s\n' "$(text 'Username:' '用户名：')" "$username" "$(text 'Password:' '密码：')" "$password"
  printf '%b%s%b\n' "$yellow" "$(text 'The generated credentials are shown only once.' '随机生成的用户名和密码只显示这一次。')" "$plain"
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

backup_database() {
  need_root
  load_env
  local default_path destination
  default_path="${PWD}/tapx-backup-$(date +%Y%m%d-%H%M%S).db"
  read_value destination "$(text "Backup file [$default_path]: " "备份文件 [$default_path]：")"
  destination="${destination:-$default_path}"
  install -d -m 0700 "$(dirname "$destination")"
  umask 077
  TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" \
    TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" \
    "$prefix/bin/tapx-panel" -export-backup "$destination" >/dev/null
  chmod 0600 "$destination"
  printf '%s %s\n' "$(text 'Backup created:' '备份已创建：')" "$destination"
}

restore_database() {
  need_root
  load_env
  local source confirm was_active=0
  read_value source "$(text 'TapX .db backup file: ' 'TapX .db 备份文件：')"
  if [[ -z "$source" || ! -r "$source" ]]; then
    printf '%s\n' "$(text 'The backup file cannot be read.' '无法读取备份文件。')" >&2
    return 1
  fi
  read_value confirm "$(text 'Replace the current TapX database? [y/N]: ' '替换当前 TapX 数据库？[y/N]：')"
  is_yes "$confirm" || return
  if "$systemctl_cmd" is-active --quiet "$service_name"; then
    was_active=1
    "$systemctl_cmd" stop "$service_name"
  fi
  if ! TAPX_DB_DRIVER="${TAPX_DB_DRIVER:-sqlite}" \
      TAPX_DB_SOURCE="${TAPX_DB_SOURCE:-$db_path_default}" \
      "$prefix/bin/tapx-panel" -restore-backup "$source" >/dev/null; then
    ((was_active)) && "$systemctl_cmd" start "$service_name" || true
    printf '%s\n' "$(text 'Database restore failed; the current database was preserved.' '数据库恢复失败，当前数据库已保留。')" >&2
    return 1
  fi
  ((was_active)) && "$systemctl_cmd" start "$service_name" || true
  printf '%s\n' "$(text 'Database restored.' '数据库恢复完成。')"
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

update_tapx() {
  need_root
  local script
  script="$(mktemp /tmp/tapx-install.XXXXXX.sh)"
  download_file "https://raw.githubusercontent.com/${repo}/main/scripts/install/install.sh" "$script"
  chmod 0700 "$script"
  TAPX_BOOTSTRAP_FORCE=1 TAPX_LANG="$TAPX_LANG" bash "$script" upgrade
  rm -f "$script"
}

update_management_script() {
  need_root
  local target="$prefix/bin/tapx" candidate script_url
  [[ -x "$target" ]] || {
    printf '%s\n' "$(text 'TapX management script is not installed.' 'TapX 管理脚本尚未安装。')" >&2
    return 1
  }
  candidate="$(mktemp "${target}.update.XXXXXX")"
  script_url="${TAPX_SCRIPT_URL:-https://raw.githubusercontent.com/${repo}/main/scripts/install/linux-install.sh}"
  if ! download_file "$script_url" "$candidate"; then
    rm -f "$candidate"
    return 1
  fi
  if ! bash -n "$candidate" || \
     ! grep -Fq 'main "$@"' "$candidate" || \
     ! grep -Fq 'TapX' "$candidate"; then
    rm -f "$candidate"
    printf '%s\n' "$(text 'Downloaded management script is invalid.' '下载的管理脚本无效。')" >&2
    return 1
  fi
  chmod 0755 "$candidate"
  mv -f "$candidate" "$target"
  printf '%s\n' "$(text 'TapX management script updated.' 'TapX 管理脚本已升级。')"
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
    printf '  7. %s\n' "$(text 'Reset random administrator credentials' '随机重置面板用户名和密码')"
    printf '  8. %s\n' "$(text 'Change database' '更换数据库')"
    printf '  9. %s\n' "$(text 'Backup database' '备份数据库')"
    printf ' 10. %s\n' "$(text 'Restore database' '恢复数据库')"
    printf ' 11. %s\n' "$(text 'Logs' '查看日志')"
    printf ' 12. %s\n' "$(text 'Enable autostart' '启用开机启动')"
    printf ' 13. %s\n' "$(text 'Disable autostart' '关闭开机启动')"
    printf ' 14. %s\n' "$(text 'Update or reinstall TapX' '更新或重新安装 TapX')"
    printf ' 15. %s\n' "$(text 'Update management script' '升级管理脚本')"
    printf ' 16. %s\n' "$(text 'Language' '语言设置')"
    printf ' 17. %s\n' "$(text 'Check installation' '安装自检')"
    printf ' 18. %s\n' "$(text 'Uninstall' '卸载')"
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
      9) backup_database ;;
      10) restore_database ;;
      11) show_logs ;;
      12) need_root; "$systemctl_cmd" enable "$service_name" ;;
      13) need_root; "$systemctl_cmd" disable "$service_name" ;;
      14) update_tapx; return ;;
      15) update_management_script ;;
      16) set_language ;;
      17) check_installation ;;
      18) uninstall_tapx; return ;;
      0) return ;;
      *) printf '%s\n' "$(text 'Invalid option.' '无效选项。')" ;;
    esac
  done
}

usage() {
  printf '%s\n' "$(text \
    'tapx [menu|status|start|stop|restart|settings|set-panel|reset-password|set-database|backup|restore|logs|enable|disable|update|update-script|language|check|uninstall]' \
    'tapx [菜单|状态|启动|停止|重启|设置|面板设置|重置密码|数据库|备份|恢复|日志|启用自启|禁用自启|更新|更新脚本|语言|自检|卸载]')"
}

main() {
  load_env
  choose_language
  local command="${1:-}"
  case "$command" in
    "") if is_installed; then show_menu; else install_wizard; fi ;;
    install|reinstall) install_wizard ;;
    upgrade) upgrade_bundle ;;
    menu) show_menu ;;
    status) show_status ;;
    start) need_root; "$systemctl_cmd" start "$service_name" ;;
    stop) need_root; "$systemctl_cmd" stop "$service_name" ;;
    restart) need_root; "$systemctl_cmd" restart "$service_name" ;;
    settings) show_settings ;;
    set-panel) modify_endpoint ;;
    set-auth|reset-password) reset_credentials ;;
    set-database) change_database ;;
    backup) backup_database ;;
    restore) restore_database ;;
    logs) show_logs ;;
    enable) need_root; "$systemctl_cmd" enable "$service_name" ;;
    disable) need_root; "$systemctl_cmd" disable "$service_name" ;;
    update) update_tapx ;;
    update-script) update_management_script ;;
    language) set_language ;;
    check) check_installation ;;
    uninstall) uninstall_tapx ;;
    help|-h|--help) usage ;;
    *) usage >&2; return 2 ;;
  esac
}

main "$@"
