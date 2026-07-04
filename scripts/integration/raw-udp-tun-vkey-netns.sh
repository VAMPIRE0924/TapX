#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/netns-vkey"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-it-vkey-a"
NS_B="tapx-it-vkey-b"
VETH_A="tapxvvka"
VETH_B="tapxvvkb"
TUN_A="tapxivka0"
TUN_B="tapxivkb0"
VKEY="tapx-netns-vkey"
PID_A=""
PID_B=""

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

cleanup() {
  set +e
  if [[ -n "$PID_A" ]]; then
    kill "$PID_A" >/dev/null 2>&1
    wait "$PID_A" >/dev/null 2>&1
  fi
  if [[ -n "$PID_B" ]]; then
    kill "$PID_B" >/dev/null 2>&1
    wait "$PID_B" >/dev/null 2>&1
  fi
  ip netns delete "$NS_A" >/dev/null 2>&1
  ip netns delete "$NS_B" >/dev/null 2>&1
}

fail_with_logs() {
  echo "integration test failed" >&2
  for log in "${BUILD_DIR}/tapx-a.log" "${BUILD_DIR}/tapx-b.log"; do
    if [[ -f "$log" ]]; then
      echo "== ${log} ==" >&2
      sed -n '1,160p' "$log" >&2
    fi
  done
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

wait_for_link() {
  local ns="$1"
  local name="$2"
  for _ in $(seq 1 100); do
    if ip -n "$ns" link show dev "$name" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.05
  done
  return 1
}

show_interface_evidence() {
  local ns="$1"
  local name="$2"
  echo "== ${ns}/${name}: ip -d addr show dev ${name} =="
  ip -n "$ns" -d addr show dev "$name"
}

if [[ "$(id -u)" != "0" ]]; then
  echo "SKIP: raw UDP/TUN vKey netns integration requires root or CAP_NET_ADMIN"
  exit 0
fi

need go
need ip
need ping
need grep
need sed

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}/tapx-a.log" "${BUILD_DIR}/tapx-b.log" "${BUILD_DIR}/tapx-a.json" "${BUILD_DIR}/tapx-b.json"

echo "build tapx-core"
(cd "$ROOT" && GOTOOLCHAIN=local go build -o "$CORE_BIN" ./cmd/tapx-core)

cat >"${BUILD_DIR}/tapx-a.json" <<JSON
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "${TUN_A}", "MTU": 1500}
  ],
  "VKeys": [
    {"ID": "vk-a", "Enabled": true, "Value": "${VKEY}"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a", "VKeyID": "vk-a"}
  ],
  "Listeners": [
    {
      "ID": "udp-a",
      "Enabled": true,
      "BindHost": "172.31.253.1",
      "BindPort": 44300,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.253.2:44300"},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
JSON

cat >"${BUILD_DIR}/tapx-b.json" <<JSON
{
  "Devices": [
    {"ID": "tun-b", "Enabled": true, "Type": "tun", "IfName": "${TUN_B}", "MTU": 1500}
  ],
  "VKeys": [
    {"ID": "vk-b", "Enabled": true, "Value": "${VKEY}"}
  ],
  "Routes": [
    {"ID": "route-b", "Enabled": true, "DeviceID": "tun-b", "VKeyID": "vk-b"}
  ],
  "Listeners": [
    {
      "ID": "udp-b",
      "Enabled": true,
      "BindHost": "172.31.253.2",
      "BindPort": 44300,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.253.1:44300"},
      "Binding": {"RouteID": "route-b"}
    }
  ]
}
JSON

echo "create namespaces"
ip netns add "$NS_A"
ip netns add "$NS_B"
ip link add "$VETH_A" type veth peer name "$VETH_B"
ip link set "$VETH_A" netns "$NS_A"
ip link set "$VETH_B" netns "$NS_B"

ip -n "$NS_A" link set lo up
ip -n "$NS_B" link set lo up
ip -n "$NS_A" addr add 172.31.253.1/30 dev "$VETH_A"
ip -n "$NS_B" addr add 172.31.253.2/30 dev "$VETH_B"
ip -n "$NS_A" link set "$VETH_A" up
ip -n "$NS_B" link set "$VETH_B" up

echo "start tapx-core vKey peers"
ip netns exec "$NS_A" "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" >"${BUILD_DIR}/tapx-a.log" 2>&1 &
PID_A="$!"
ip netns exec "$NS_B" "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" >"${BUILD_DIR}/tapx-b.log" 2>&1 &
PID_B="$!"

wait_for_log "${BUILD_DIR}/tapx-a.log" "runtime started" || fail_with_logs
wait_for_log "${BUILD_DIR}/tapx-b.log" "runtime started" || fail_with_logs
wait_for_link "$NS_A" "$TUN_A" || fail_with_logs
wait_for_link "$NS_B" "$TUN_B" || fail_with_logs
show_interface_evidence "$NS_A" "$TUN_A"
show_interface_evidence "$NS_B" "$TUN_B"

echo "configure tunnel interfaces"
ip -n "$NS_A" addr add 10.91.0.1/30 dev "$TUN_A"
ip -n "$NS_B" addr add 10.91.0.2/30 dev "$TUN_B"
ip -n "$NS_A" link set "$TUN_A" up
ip -n "$NS_B" link set "$TUN_B" up

echo "verify underlay"
ip netns exec "$NS_A" ping -c 1 -W 1 172.31.253.2 >/dev/null || fail_with_logs

echo "verify raw UDP/TUN vKey tunnel"
ip netns exec "$NS_A" ping -c 3 -W 1 10.91.0.2 >/dev/null || fail_with_logs
ip netns exec "$NS_B" ping -c 3 -W 1 10.91.0.1 >/dev/null || fail_with_logs

echo "raw UDP/TUN vKey netns integration: ok"
