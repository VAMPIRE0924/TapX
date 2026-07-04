#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/netns"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS="tapx-it-apply"
TUN="tapxap0"
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
    kill "$PID" >/dev/null 2>&1
    wait "$PID" >/dev/null 2>&1
  fi
  ip netns delete "$NS" >/dev/null 2>&1
}

fail_with_logs() {
  echo "device apply integration failed" >&2
  if [[ -f "${BUILD_DIR}/tapx-apply.log" ]]; then
    echo "== ${BUILD_DIR}/tapx-apply.log ==" >&2
    sed -n '1,160p' "${BUILD_DIR}/tapx-apply.log" >&2
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

if [[ "$(id -u)" != "0" ]]; then
  echo "SKIP: device apply netns integration requires root or CAP_NET_ADMIN"
  exit 0
fi

need go
need ip
need grep
need sed

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}/tapx-apply.log" "${BUILD_DIR}/tapx-apply.json"

echo "build tapx-core"
(cd "$ROOT" && GOTOOLCHAIN=local go build -o "$CORE_BIN" ./cmd/tapx-core)

cat >"${BUILD_DIR}/tapx-apply.json" <<JSON
{
  "Devices": [
    {
      "ID": "tun-a",
      "Enabled": true,
      "Type": "tun",
      "IfName": "${TUN}",
      "MTU": 1400,
      "IPv4CIDR": "10.99.0.1/30",
      "Routes": [
        {"Enabled": true, "Destination": "10.250.0.0/24", "Metric": 20}
      ]
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
      "BindPort": 45100,
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
ip netns exec "$NS" "$CORE_BIN" -config "${BUILD_DIR}/tapx-apply.json" >"${BUILD_DIR}/tapx-apply.log" 2>&1 &
PID="$!"

wait_for_log "${BUILD_DIR}/tapx-apply.log" "runtime started" || fail_with_logs

echo "verify device config"
ip -n "$NS" link show dev "$TUN" | grep -q "mtu 1400" || fail_with_logs
ip -n "$NS" addr show dev "$TUN" | grep -q "10.99.0.1/30" || fail_with_logs
ip -n "$NS" link show dev "$TUN" | grep -q "UP" || fail_with_logs
ip -n "$NS" route show 10.250.0.0/24 | grep -q "dev ${TUN}" || fail_with_logs
ip -n "$NS" route show 10.250.0.0/24 | grep -q "metric 20" || fail_with_logs

echo "device apply netns integration: ok"
