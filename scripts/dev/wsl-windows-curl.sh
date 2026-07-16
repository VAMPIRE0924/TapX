#!/usr/bin/env bash
set -euo pipefail

windows_curl="${TAPX_WINDOWS_CURL:-/mnt/c/Windows/System32/curl.exe}"
if [[ ! -x "$windows_curl" ]]; then
  echo "Windows curl.exe is unavailable at $windows_curl" >&2
  exit 127
fi

args=()
while (($#)); do
  case "$1" in
    -o|--output)
      (($# >= 2)) || { echo "missing curl output path" >&2; exit 2; }
      args+=("$1")
      if [[ "$2" == /* ]]; then
        args+=("$(wslpath -w "$2")")
      else
        args+=("$2")
      fi
      shift 2
      ;;
    --output=/*)
      args+=("--output=$(wslpath -w "${1#--output=}")")
      shift
      ;;
    *)
      args+=("$1")
      shift
      ;;
  esac
done

exec "$windows_curl" "${args[@]}"
