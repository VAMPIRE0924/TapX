#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/netns"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-it-a"
NS_B="tapx-it-b"
VETH_A="tapxv-a"
VETH_B="tapxv-b"
TUN_A="tapxita0"
TUN_B="tapxitb0"
PID_A=""
PID_B=""
START_DELAY_SECONDS="${START_DELAY_SECONDS:-0}"
UNDERLAY_MTU="${UNDERLAY_MTU:-1280}"
DYNAMIC_PATH_SHRINK_MTU="${DYNAMIC_PATH_SHRINK_MTU:-0}"
TRACE_SYSCALLS="${TRACE_SYSCALLS:-0}"

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

wait_for_ping() {
  local ns="$1"
  shift
  for _ in $(seq 1 30); do
    if ip netns exec "$ns" ping -c 1 -W 1 "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

show_interface_evidence() {
  local ns="$1"
  local name="$2"
  echo "== ${ns}/${name}: ip -d addr show dev ${name} =="
  ip -n "$ns" -d addr show dev "$name"
}

ip_snmp_value() {
  local ns="$1"
  local field="$2"
  ip netns exec "$ns" awk -v key="$field" '
    $1 == "Ip:" && !seen { for (i = 2; i <= NF; i++) cols[$i] = i; seen = 1; next }
    $1 == "Ip:" && seen { if (cols[key] > 0) print $cols[key]; exit }
  ' /proc/net/snmp
}

if [[ "$(id -u)" != "0" ]]; then
  echo "SKIP: raw UDP/TUN netns integration requires root or CAP_NET_ADMIN"
  exit 0
fi

need go
need ip
need ping
need grep
need sed
need awk

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}/tapx-a.log" "${BUILD_DIR}/tapx-b.log" "${BUILD_DIR}/tapx-a.json" "${BUILD_DIR}/tapx-b.json"

echo "build tapx-core"
(cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -o "$CORE_BIN" ./cmd/tapx-core)

cat >"${BUILD_DIR}/tapx-a.json" <<JSON
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "${TUN_A}", "MTU": 1500, "LinkAutoOptimize": true}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Listeners": [
    {
      "ID": "udp-a",
      "Enabled": true,
      "BindHost": "172.31.250.1",
      "BindPort": 44000,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.250.2:44000"},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
JSON

cat >"${BUILD_DIR}/tapx-b.json" <<JSON
{
  "Devices": [
    {"ID": "tun-b", "Enabled": true, "Type": "tun", "IfName": "${TUN_B}", "MTU": 1500, "LinkAutoOptimize": true}
  ],
  "Routes": [
    {"ID": "route-b", "Enabled": true, "DeviceID": "tun-b"}
  ],
  "Listeners": [
    {
      "ID": "udp-b",
      "Enabled": true,
      "BindHost": "172.31.250.2",
      "BindPort": 44000,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "172.31.250.1:44000"},
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
ip -n "$NS_A" addr add 172.31.250.1/30 dev "$VETH_A"
ip -n "$NS_B" addr add 172.31.250.2/30 dev "$VETH_B"
ip -n "$NS_A" link set "$VETH_A" mtu "$UNDERLAY_MTU"
ip -n "$NS_B" link set "$VETH_B" mtu "$UNDERLAY_MTU"
ip -n "$NS_A" link set "$VETH_A" up
ip -n "$NS_B" link set "$VETH_B" up

echo "start tapx-core peers"
if (( TRACE_SYSCALLS > 0 )); then
  ip netns exec "$NS_A" strace -ff -tt -o "${BUILD_DIR}/tapx-a.strace" -e trace=sendto,recvmsg,getsockopt "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" >"${BUILD_DIR}/tapx-a.log" 2>&1 &
else
  ip netns exec "$NS_A" "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" >"${BUILD_DIR}/tapx-a.log" 2>&1 &
fi
PID_A="$!"
sleep "$START_DELAY_SECONDS"
if (( TRACE_SYSCALLS > 0 )); then
  ip netns exec "$NS_B" strace -ff -tt -o "${BUILD_DIR}/tapx-b.strace" -e trace=sendto,recvmsg,getsockopt "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" >"${BUILD_DIR}/tapx-b.log" 2>&1 &
else
  ip netns exec "$NS_B" "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" >"${BUILD_DIR}/tapx-b.log" 2>&1 &
fi
PID_B="$!"

wait_for_log "${BUILD_DIR}/tapx-a.log" "runtime started" || fail_with_logs
wait_for_log "${BUILD_DIR}/tapx-b.log" "runtime started" || fail_with_logs
wait_for_link "$NS_A" "$TUN_A" || fail_with_logs
wait_for_link "$NS_B" "$TUN_B" || fail_with_logs
show_interface_evidence "$NS_A" "$TUN_A"
show_interface_evidence "$NS_B" "$TUN_B"

echo "configure tunnel interfaces"
ip -n "$NS_A" addr add 10.88.0.1/30 dev "$TUN_A"
ip -n "$NS_B" addr add 10.88.0.2/30 dev "$TUN_B"
ip -n "$NS_A" addr add fd88::1/126 dev "$TUN_A" nodad
ip -n "$NS_B" addr add fd88::2/126 dev "$TUN_B" nodad
ip -n "$NS_A" link set "$TUN_A" up
ip -n "$NS_B" link set "$TUN_B" up

echo "verify underlay"
ip netns exec "$NS_A" ping -c 1 -W 1 172.31.250.2 >/dev/null || fail_with_logs
frag_a_before="$(ip_snmp_value "$NS_A" FragCreates)"
frag_b_before="$(ip_snmp_value "$NS_B" FragCreates)"
reasm_a_before="$(ip_snmp_value "$NS_A" ReasmReqds)"
reasm_b_before="$(ip_snmp_value "$NS_B" ReasmReqds)"

echo "verify raw UDP/TUN tunnel"
wait_for_ping "$NS_A" 10.88.0.2 || fail_with_logs
wait_for_ping "$NS_B" 10.88.0.1 || fail_with_logs
wait_for_ping "$NS_A" -6 fd88::2 || fail_with_logs
wait_for_ping "$NS_B" -6 fd88::1 || fail_with_logs

if (( DYNAMIC_PATH_SHRINK_MTU > 0 )); then
  if (( DYNAMIC_PATH_SHRINK_MTU >= UNDERLAY_MTU )); then
    echo "DYNAMIC_PATH_SHRINK_MTU must be smaller than UNDERLAY_MTU" >&2
    exit 1
  fi
  echo "verify the active tunnel before shrinking the underlay from ${UNDERLAY_MTU} to ${DYNAMIC_PATH_SHRINK_MTU}"
  wait_for_ping "$NS_A" -M do -s 1400 10.88.0.2 || fail_with_logs
  wait_for_ping "$NS_B" -M do -s 1400 10.88.0.1 || fail_with_logs
  ip -n "$NS_A" link set "$VETH_A" mtu "$DYNAMIC_PATH_SHRINK_MTU"
  ip -n "$NS_B" link set "$VETH_B" mtu "$DYNAMIC_PATH_SHRINK_MTU"
fi

echo "verify large inner packets across the smaller UDP underlay without outer fragmentation"
wait_for_ping "$NS_A" -M do -s 1400 10.88.0.2 || fail_with_logs
wait_for_ping "$NS_B" -M do -s 1400 10.88.0.1 || fail_with_logs
wait_for_ping "$NS_A" -6 -M do -s 1352 fd88::2 || fail_with_logs
wait_for_ping "$NS_B" -6 -M do -s 1352 fd88::1 || fail_with_logs

[[ "$(ip_snmp_value "$NS_A" FragCreates)" == "$frag_a_before" ]] || fail_with_logs
[[ "$(ip_snmp_value "$NS_B" FragCreates)" == "$frag_b_before" ]] || fail_with_logs
[[ "$(ip_snmp_value "$NS_A" ReasmReqds)" == "$reasm_a_before" ]] || fail_with_logs
[[ "$(ip_snmp_value "$NS_B" ReasmReqds)" == "$reasm_b_before" ]] || fail_with_logs

echo "raw UDP/TUN netns integration: ok"
