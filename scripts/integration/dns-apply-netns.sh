#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/netns"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS="tapx-it-dns"
TUN="tapxdns0"
DNS_FILE="${BUILD_DIR}/tapx-dns.resolv.conf"
PID=""

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

cleanup() {
  set +e
  if [[ -n "$PID" ]]; then
    kill -TERM "$PID" >/dev/null 2>&1
    wait "$PID" >/dev/null 2>&1
  fi
  ip netns delete "$NS" >/dev/null 2>&1
  rm -f "$DNS_FILE"
}

fail_with_logs() {
  echo "dns apply integration failed" >&2
  if [[ -f "${BUILD_DIR}/tapx-dns.log" ]]; then
    echo "== ${BUILD_DIR}/tapx-dns.log ==" >&2
    sed -n '1,160p' "${BUILD_DIR}/tapx-dns.log" >&2
  fi
  if [[ -f "$DNS_FILE" ]]; then
    echo "== ${DNS_FILE} ==" >&2
    cat "$DNS_FILE" >&2
  fi
  exit 1
}

wait_for_log() {
  local log="$1"
  local text="$2"
  for _ in $(seq 1 100); do
    if [[ -f "$log" ]] && grep -q "$text" "$log"; then
      return 0
    fi
    sleep 0.05
  done
  return 1
}

stop_core() {
  if [[ -n "$PID" ]]; then
    kill -TERM "$PID" >/dev/null 2>&1
    wait "$PID" >/dev/null 2>&1
    PID=""
  fi
}

if [[ "$(id -u)" != "0" ]]; then
  echo "SKIP: dns apply netns integration requires root or CAP_NET_ADMIN"
  exit 0
fi

need go
need ip
need grep
need sed

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
mkdir -p "$BUILD_DIR"
rm -f "${BUILD_DIR}/tapx-dns.log" "${BUILD_DIR}/tapx-dns.json"

echo "build tapx-core"
(cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -o "$CORE_BIN" ./cmd/tapx-core)

cat >"${BUILD_DIR}/tapx-dns.json" <<JSON
{
  "Devices": [
    {
      "ID": "tun-a",
      "Enabled": true,
      "Type": "tun",
      "IfName": "${TUN}",
      "MTU": 1400,
      "IPv4CIDR": "10.97.0.1/30",
      "DNS": {
        "Enabled": true,
        "Nameservers": ["1.1.1.1", "2606:4700:4700::1111"],
        "SearchDomains": ["example.com", "lan"],
        "Options": ["timeout:1", "attempts:2"],
        "OutputPath": "${DNS_FILE}"
      }
    }
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Listeners": [
    {
      "ID": "udp-a",
      "Enabled": true,
      "BindHost": "127.0.0.1",
      "BindPort": 45103,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "any"},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
JSON

echo "create namespace"
ip netns add "$NS"
ip -n "$NS" link set lo up

echo "start tapx-core"
ip netns exec "$NS" "$CORE_BIN" -config "${BUILD_DIR}/tapx-dns.json" >"${BUILD_DIR}/tapx-dns.log" 2>&1 &
PID="$!"

wait_for_log "${BUILD_DIR}/tapx-dns.log" "runtime started" || fail_with_logs

echo "verify dns file"
grep -q "nameserver 1.1.1.1" "$DNS_FILE" || fail_with_logs
grep -q "nameserver 2606:4700:4700::1111" "$DNS_FILE" || fail_with_logs
grep -q "search example.com lan" "$DNS_FILE" || fail_with_logs
grep -q "options timeout:1 attempts:2" "$DNS_FILE" || fail_with_logs

echo "verify dns rollback"
stop_core
if [[ -e "$DNS_FILE" ]]; then
  fail_with_logs
fi

echo "dns apply netns integration: ok"
