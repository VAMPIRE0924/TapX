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
    echo -e "${red}TapX 安装/管理脚本必须使用 root 权限运行。${plain}" >&2
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

has_tty() {
  [[ -r /dev/tty ]] && { : </dev/tty; } 2>/dev/null
}

is_yes() {
  [[ "${1:-}" =~ ^([Yy]|[Yy][Ee][Ss]|是|确认)$ ]]
}

read_prompt() {
  local __var="$1" __prompt="$2" __secret="${3:-0}" __value=""
  if [[ "$__secret" == "1" ]]; then
    if has_tty; then
      read -r -s -p "$__prompt" __value </dev/tty
    else
      read -r -s -p "$__prompt" __value
    fi
    echo
  else
    if has_tty; then
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
  read_prompt value "$prompt [留空=随机生成]: " 1
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
Description=TapX 面板和运行时管理服务
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
      echo -e "${red}缺少 $build_dir/$bin。${plain}" >&2
      echo "请先构建：make build-linux-amd64" >&2
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

  echo -e "${green}TapX Linux 安装向导${plain}"
  echo
  local host port listen base_path username password db_path enable start
  prompt_default host "面板监听地址" "${TAPX_PANEL_HOST:-127.0.0.1}"
  prompt_default port "面板监听端口" "${TAPX_PANEL_PORT:-8080}"
  listen="${host}:${port}"
  prompt_default base_path "面板访问路径" "${TAPX_PANEL_BASE_PATH:-/tapx-$(random_token 9)}"
  base_path="$(normalize_base_path "$base_path")"
  prompt_default username "管理员用户名" "${TAPX_ADMIN_USERNAME:-admin}"
  prompt_secret_default password "管理员密码" "${TAPX_ADMIN_PASSWORD:-$(random_token 18)}"
  prompt_default db_path "SQLite 数据库路径" "$db_path_default"
  prompt_default enable "启用 systemd 开机自启" "y"
  prompt_default start "现在启动面板" "y"

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
    is_yes "$enable" && systemctl enable "$service_name"
    is_yes "$start" && systemctl restart "$service_name"
  fi

  echo
  echo -e "${green}TapX 已安装完成。${plain}"
  show_settings
  echo -e "${yellow}管理员密码仅保存一次：$(result_file)，权限为 600。${plain}"
}

is_installed() {
  [[ -x "$prefix/bin/tapx-panel" && -x "$prefix/bin/tapx-core" && -f "$(env_file)" ]]
}

show_status() {
  echo -e "${blue}面板服务：${plain}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl status "$service_name" --no-pager || true
  else
    pgrep -a tapx-panel || true
  fi
  echo
  echo -e "${blue}程序版本：${plain}"
  "$prefix/bin/tapx-panel" -version 2>/dev/null || true
  "$prefix/bin/tapx-core" -version 2>/dev/null || true
}

show_settings() {
  load_env
  echo -e "${blue}面板参数：${plain}"
  echo "  监听地址：      ${TAPX_PANEL_LISTEN:-127.0.0.1:8080}"
  echo "  访问路径：      ${TAPX_PANEL_BASE_PATH:-/}"
  echo "  数据库：        ${TAPX_DB_PATH:-$db_path_default}"
  echo "  面板地址：      $(panel_url_host "${TAPX_PANEL_LISTEN:-127.0.0.1:8080}")"
  echo "  环境文件：      $(env_file)"
  echo "  安装结果文件：  $(result_file)"
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

  echo -e "${green}修改 TapX 面板参数${plain}"
  prompt_default host "面板监听地址" "$current_host"
  prompt_default port "面板监听端口" "$current_port"
  listen="${host}:${port}"
  prompt_default base_path "面板访问路径" "$current_base"
  base_path="$(normalize_base_path "$base_path")"
  prompt_default db_path "SQLite 数据库路径" "$current_db"
  prompt_default change_auth "是否修改管理员用户名/密码" "n"

  write_env_file "$listen" "$base_path" "$db_path"
  if is_yes "$change_auth"; then
    prompt_default username "管理员用户名" "${TAPX_ADMIN_USERNAME:-admin}"
    prompt_secret_default password "管理员密码" "$(random_token 18)"
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
    rm -f "$cookie"
    echo -e "${red}API 调用失败：${plain} $result" >&2
    return 1
  }
  rm -f "$cookie"
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
  read_prompt confirm "确认卸载 TapX 面板、二进制文件和 systemd 服务？[y/N]: "
  is_yes "$confirm" || return 0
  systemctl_if_available stop "$service_name" >/dev/null 2>&1 || true
  systemctl_if_available disable "$service_name" >/dev/null 2>&1 || true
  rm -f "$unit_dir/$service_name" "$prefix/bin/tapx" "$prefix/bin/tapx-core" "$prefix/bin/tapx-panel"
  systemctl_if_available daemon-reload >/dev/null 2>&1 || true
  echo -e "${yellow}已保留数据和配置目录：${plain} $sysconfdir /var/lib/tapx /var/log/tapx"
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
    echo -e "${green}TapX 管理菜单${plain}"
    echo "  1. 安装 / 重新安装"
    echo "  2. 查看面板状态"
    echo "  3. 启动面板"
    echo "  4. 停止面板"
    echo "  5. 重启面板"
    echo "  6. 查看 Core 运行状态"
    echo "  7. 应用 Core 运行配置"
    echo "  8. 停止 Core 运行时"
    echo "  9. 查看面板参数"
    echo " 10. 修改面板参数"
    echo " 11. 查看日志"
    echo " 12. 启用开机自启"
    echo " 13. 禁用开机自启"
    echo " 14. 卸载"
    echo "  0. 退出"
    read_prompt choice "请选择: "
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
      *) echo -e "${red}无效选项。${plain}" ;;
    esac
  done
}

usage() {
  cat <<EOF
用法：tapx [command]

命令：
  install       运行安装向导
  menu          显示管理菜单
  status        查看面板状态和程序版本
  start         启动面板服务
  stop          停止面板服务
  restart       重启面板服务
  core-state    通过面板 API 查看 TapX 运行状态
  core-apply    通过面板 API 应用 TapX 运行配置
  core-stop     通过面板 API 停止 TapX 运行时
  settings      查看面板参数
  set-panel     修改面板参数
  logs          查看面板日志
  enable        启用开机自启
  disable       禁用开机自启
  uninstall     卸载程序和服务，保留数据
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
    *) echo "未知命令：$cmd" >&2; usage >&2; exit 2 ;;
  esac
}

main "$@"
