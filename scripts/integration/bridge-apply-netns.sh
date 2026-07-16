#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/netns"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS="tapx-it-bridge"
TAP="tapxbr0"
BRIDGE="brtapx0"
MEMBER="tapxmem0"
PEER="tapxmem1"
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
  echo "bridge apply integration failed" >&2
  if [[ -f "${BUILD_DIR}/tapx-bridge.log" ]]; then
    echo "== ${BUILD_DIR}/tapx-bridge.log ==" >&2
    sed -n '1,160p' "${BUILD_DIR}/tapx-bridge.log" >&2
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
  echo "SKIP: bridge apply netns integration requires root or CAP_NET_ADMIN"
  exit 0
fi

need go
need ip
need grep
need sed

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}/tapx-bridge.log" "${BUILD_DIR}/tapx-bridge.json"

echo "build tapx-core"
(cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -o "$CORE_BIN" ./cmd/tapx-core)

cat >"${BUILD_DIR}/tapx-bridge.json" <<JSON
{
  "Devices": [
    {
      "ID": "tap-a",
      "Enabled": true,
      "Type": "tap",
      "IfName": "${TAP}",
      "MTU": 1400,
      "Bridge": {
        "Enabled": true,
        "Name": "${BRIDGE}",
        "IfName": "${MEMBER}",
        "MTU": 1400
      }
    }
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tap-a"}
  ],
  "Listeners": [
    {
      "ID": "udp-a",
      "Enabled": true,
      "BindHost": "127.0.0.1",
      "BindPort": 45101,
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
ip -n "$NS" link add "$MEMBER" type veth peer name "$PEER"
ip -n "$NS" link set "$PEER" up

echo "start tapx-core"
ip netns exec "$NS" "$CORE_BIN" -config "${BUILD_DIR}/tapx-bridge.json" >"${BUILD_DIR}/tapx-bridge.log" 2>&1 &
PID="$!"

wait_for_log "${BUILD_DIR}/tapx-bridge.log" "runtime started" || fail_with_logs

echo "verify bridge config"
ip -n "$NS" link show dev "$BRIDGE" | grep -q "mtu 1400" || fail_with_logs
ip -n "$NS" link show dev "$TAP" | grep -q "master ${BRIDGE}" || fail_with_logs
ip -n "$NS" link show dev "$MEMBER" | grep -q "master ${BRIDGE}" || fail_with_logs
ip -n "$NS" link show dev "$BRIDGE" | grep -q "UP" || fail_with_logs

echo "bridge apply netns integration: ok"
