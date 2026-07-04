#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/netns-guard"
CORE_BIN="${TAPX_CORE_BIN:-${ROOT}/build/linux-amd64/tapx-core}"
LOCAL_CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-it-guard-a"
NS_B="tapx-it-guard-b"
VETH_A="tapxvgua"
VETH_B="tapxvgub"
TUN_A="tapxgta0"
TUN_B="tapxgtb0"
TAP_A="tapxgpa0"
TAP_B="tapxgpb0"
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
  echo "address guard integration failed" >&2
  for log in "${BUILD_DIR}/tapx-a.log" "${BUILD_DIR}/tapx-b.log"; do
    if [[ -f "$log" ]]; then
      echo "== ${log} ==" >&2
      sed -n '1,180p' "$log" >&2
    fi
  done
  echo "== namespace state ==" >&2
  ip -n "$NS_A" addr show >&2 || true
  ip -n "$NS_B" addr show >&2 || true
  ip -n "$NS_A" neigh show >&2 || true
  ip -n "$NS_B" neigh show >&2 || true
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

expect_ping_ok() {
  local ns="$1"
  local src="$2"
  local dst="$3"
  ip netns exec "$ns" ping -I "$src" -c 2 -W 1 "$dst" >/dev/null || fail_with_logs
}

expect_ping_blocked() {
  local ns="$1"
  local src="$2"
  local dst="$3"
  if ip netns exec "$ns" ping -I "$src" -c 2 -W 1 "$dst" >/dev/null 2>&1; then
    echo "unexpected successful ping from ${src} to ${dst}" >&2
    fail_with_logs
  fi
}

if [[ "$(id -u)" != "0" ]]; then
  echo "SKIP: address guard netns integration requires root or CAP_NET_ADMIN"
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
if [[ ! -x "$CORE_BIN" ]]; then
  CORE_BIN="$LOCAL_CORE_BIN"
  (cd "$ROOT" && GOTOOLCHAIN=local go build -o "$CORE_BIN" ./cmd/tapx-core)
fi

cat >"${BUILD_DIR}/tapx-a.json" <<JSON
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "${TUN_A}", "MTU": 1500},
    {"ID": "tap-a", "Enabled": true, "Type": "tap", "IfName": "${TAP_A}", "MTU": 1500}
  ],
  "Addresses": [
    {"ID": "tun-guard-a", "Enabled": true, "DeviceID": "tun-a", "IPv4CIDRs": ["10.90.0.1/32"]},
    {"ID": "tap-guard-a", "Enabled": true, "DeviceID": "tap-a", "MACs": ["02:00:00:00:ca:01"], "IPv4CIDRs": ["10.91.0.1/32"]}
  ],
  "Routes": [
    {"ID": "tun-route-a", "Enabled": true, "DeviceID": "tun-a", "AddressID": "tun-guard-a"},
    {"ID": "tap-route-a", "Enabled": true, "DeviceID": "tap-a", "AddressID": "tap-guard-a"}
  ],
  "Listeners": [
    {
      "ID": "tun-udp-a",
      "Enabled": true,
      "BindHost": "172.31.252.1",
      "BindPort": 44200,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.252.2:44200"},
      "Binding": {"RouteID": "tun-route-a"}
    },
    {
      "ID": "tap-udp-a",
      "Enabled": true,
      "BindHost": "172.31.252.1",
      "BindPort": 44201,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.252.2:44201"},
      "Binding": {"RouteID": "tap-route-a"}
    }
  ]
}
JSON

cat >"${BUILD_DIR}/tapx-b.json" <<JSON
{
  "Devices": [
    {"ID": "tun-b", "Enabled": true, "Type": "tun", "IfName": "${TUN_B}", "MTU": 1500},
    {"ID": "tap-b", "Enabled": true, "Type": "tap", "IfName": "${TAP_B}", "MTU": 1500}
  ],
  "Addresses": [
    {"ID": "tun-guard-b", "Enabled": true, "DeviceID": "tun-b", "IPv4CIDRs": ["10.90.0.2/32"]},
    {"ID": "tap-guard-b", "Enabled": true, "DeviceID": "tap-b", "MACs": ["02:00:00:00:cb:01"], "IPv4CIDRs": ["10.91.0.2/32"]}
  ],
  "Routes": [
    {"ID": "tun-route-b", "Enabled": true, "DeviceID": "tun-b", "AddressID": "tun-guard-b"},
    {"ID": "tap-route-b", "Enabled": true, "DeviceID": "tap-b", "AddressID": "tap-guard-b"}
  ],
  "Listeners": [
    {
      "ID": "tun-udp-b",
      "Enabled": true,
      "BindHost": "172.31.252.2",
      "BindPort": 44200,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.252.1:44200"},
      "Binding": {"RouteID": "tun-route-b"}
    },
    {
      "ID": "tap-udp-b",
      "Enabled": true,
      "BindHost": "172.31.252.2",
      "BindPort": 44201,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.252.1:44201"},
      "Binding": {"RouteID": "tap-route-b"}
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
ip -n "$NS_A" addr add 172.31.252.1/30 dev "$VETH_A"
ip -n "$NS_B" addr add 172.31.252.2/30 dev "$VETH_B"
ip -n "$NS_A" link set "$VETH_A" up
ip -n "$NS_B" link set "$VETH_B" up

echo "start guarded tapx-core peers"
ip netns exec "$NS_A" "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" >"${BUILD_DIR}/tapx-a.log" 2>&1 &
PID_A="$!"
ip netns exec "$NS_B" "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" >"${BUILD_DIR}/tapx-b.log" 2>&1 &
PID_B="$!"

wait_for_log "${BUILD_DIR}/tapx-a.log" "runtime started" || fail_with_logs
wait_for_log "${BUILD_DIR}/tapx-b.log" "runtime started" || fail_with_logs
wait_for_link "$NS_A" "$TUN_A" || fail_with_logs
wait_for_link "$NS_B" "$TUN_B" || fail_with_logs
wait_for_link "$NS_A" "$TAP_A" || fail_with_logs
wait_for_link "$NS_B" "$TAP_B" || fail_with_logs

echo "configure guarded interfaces"
ip -n "$NS_A" addr add 10.90.0.1/30 dev "$TUN_A"
ip -n "$NS_B" addr add 10.90.0.2/30 dev "$TUN_B"
ip -n "$NS_A" addr add 10.90.0.99/32 dev "$TUN_A"
ip -n "$NS_B" addr add 10.90.0.98/32 dev "$TUN_B"
ip -n "$NS_A" link set "$TUN_A" up
ip -n "$NS_B" link set "$TUN_B" up

ip -n "$NS_A" link set "$TAP_A" address 02:00:00:00:ca:01
ip -n "$NS_B" link set "$TAP_B" address 02:00:00:00:cb:01
ip -n "$NS_A" addr add 10.91.0.1/30 dev "$TAP_A"
ip -n "$NS_B" addr add 10.91.0.2/30 dev "$TAP_B"
ip -n "$NS_A" addr add 10.91.0.99/32 dev "$TAP_A"
ip -n "$NS_B" addr add 10.91.0.98/32 dev "$TAP_B"
ip -n "$NS_A" link set "$TAP_A" up
ip -n "$NS_B" link set "$TAP_B" up

echo "verify underlay"
ip netns exec "$NS_A" ping -c 1 -W 1 172.31.252.2 >/dev/null || fail_with_logs

echo "verify allowed guarded TUN/TAP traffic"
expect_ping_ok "$NS_A" "10.90.0.1" "10.90.0.2"
expect_ping_ok "$NS_B" "10.90.0.2" "10.90.0.1"
expect_ping_ok "$NS_A" "10.91.0.1" "10.91.0.2"
expect_ping_ok "$NS_B" "10.91.0.2" "10.91.0.1"

echo "verify unauthorized guarded TUN/TAP traffic is dropped"
expect_ping_blocked "$NS_A" "10.90.0.99" "10.90.0.2"
expect_ping_blocked "$NS_B" "10.90.0.98" "10.90.0.1"
expect_ping_blocked "$NS_A" "10.91.0.99" "10.91.0.2"
expect_ping_blocked "$NS_B" "10.91.0.98" "10.91.0.1"

echo "address guard netns integration: ok"
