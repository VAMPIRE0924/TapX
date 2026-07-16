#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/xray-mkcp-v6"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-xkcp-v6-a"
NS_B="tapx-xkcp-v6-b"
VETH_A="txkcpv6-a"
VETH_B="txkcpv6-b"
TUN_A="txkcptun-a"
TUN_B="txkcptun-b"
PID_A=""
PID_B=""
CORE_ARGS="${CORE_ARGS:-}"

cleanup() {
  set +e
  [[ -z "$PID_A" ]] || { kill "$PID_A" >/dev/null 2>&1; wait "$PID_A" >/dev/null 2>&1; }
  [[ -z "$PID_B" ]] || { kill "$PID_B" >/dev/null 2>&1; wait "$PID_B" >/dev/null 2>&1; }
  ip netns delete "$NS_A" >/dev/null 2>&1
  ip netns delete "$NS_B" >/dev/null 2>&1
}

fail_with_logs() {
  echo "embedded Xray mKCP/IPv6-underlay integration failed" >&2
  sed -n '1,180p' "${BUILD_DIR}/tapx-a.log" >&2 || true
  sed -n '1,180p' "${BUILD_DIR}/tapx-b.log" >&2 || true
  exit 1
}

wait_for() {
  local command="$1"
  for _ in $(seq 1 160); do
    eval "$command" && return 0
    sleep 0.05
  done
  return 1
}

snmp6_value() {
  local ns="$1" key="$2"
  ip netns exec "$ns" awk -v key="$key" '$1 == key { print $2; exit }' /proc/net/snmp6
}

[[ "$(id -u)" == "0" ]] || { echo "SKIP: test requires root or CAP_NET_ADMIN"; exit 0; }
for tool in go ip ping grep sed awk; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing required command: $tool" >&2; exit 1; }
done

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}"/{tapx-a,tapx-b}.{json,log}
(cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -o "$CORE_BIN" ./cmd/tapx-core)

cat >"${BUILD_DIR}/tapx-a.json" <<JSON
{
  "Devices": [{"ID":"tun-a","Enabled":true,"Type":"tun","IfName":"${TUN_A}","MTU":1400,"LinkAutoOptimize":true}],
  "XrayProfiles": [{
    "ID":"xray-server","Enabled":true,"Runtime":"embedded",
    "InboundProtocol":"vless",
    "InboundSettingsJSON":"{\"clients\":[{\"id\":\"22222222-2222-4222-8222-222222222222\",\"level\":0}],\"decryption\":\"none\"}",
    "Network":"mkcp","Security":"none","StreamSettingsJSON":"{}",
    "AdvancedJSON":"{\"outbounds\":[{\"tag\":\"direct\",\"protocol\":\"freedom\"}]}"
  }],
  "Listeners": [{
    "ID":"xray-a","Enabled":true,"BindHost":"fd31:251::1","BindPort":44101,
    "Transport":"xray","XrayProfileID":"xray-server","RawTCP":{"LengthMode":"uint16"},
    "Binding":{"DeviceID":"tun-a"}
  }]
}
JSON

cat >"${BUILD_DIR}/tapx-b.json" <<JSON
{
  "Devices": [{"ID":"tun-b","Enabled":true,"Type":"tun","IfName":"${TUN_B}","MTU":1400,"LinkAutoOptimize":true}],
  "XrayProfiles": [{
    "ID":"xray-client","Enabled":true,"Runtime":"embedded",
    "OutboundProtocol":"vless",
    "OutboundSettingsJSON":"{\"vnext\":[{\"address\":\"fd31:251::1\",\"port\":44101,\"users\":[{\"id\":\"22222222-2222-4222-8222-222222222222\",\"encryption\":\"none\"}]}]}",
    "Network":"mkcp","Security":"none","StreamSettingsJSON":"{}"
  }],
  "Connectors": [{
    "ID":"xray-b","Enabled":true,"Remote":"tapx.frame.local","Port":1,
    "Transport":"xray","XrayProfileID":"xray-client","RawTCP":{"LengthMode":"uint16"},
    "Binding":{"DeviceID":"tun-b"}
  }]
}
JSON

ip netns add "$NS_A"
ip netns add "$NS_B"
ip link add "$VETH_A" type veth peer name "$VETH_B"
ip link set "$VETH_A" netns "$NS_A"
ip link set "$VETH_B" netns "$NS_B"
ip -n "$NS_A" link set lo up
ip -n "$NS_B" link set lo up
ip -n "$NS_A" addr add fd31:251::1/64 dev "$VETH_A" nodad
ip -n "$NS_B" addr add fd31:251::2/64 dev "$VETH_B" nodad
ip -n "$NS_A" link set "$VETH_A" mtu 1280 up
ip -n "$NS_B" link set "$VETH_B" mtu 1280 up

ip netns exec "$NS_A" "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" $CORE_ARGS >"${BUILD_DIR}/tapx-a.log" 2>&1 & PID_A="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/tapx-a.log' 2>/dev/null" || fail_with_logs
ip netns exec "$NS_B" "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" $CORE_ARGS >"${BUILD_DIR}/tapx-b.log" 2>&1 & PID_B="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/tapx-b.log' 2>/dev/null" || fail_with_logs
wait_for "ip -n '$NS_A' link show '$TUN_A' >/dev/null 2>&1" || fail_with_logs
wait_for "ip -n '$NS_B' link show '$TUN_B' >/dev/null 2>&1" || fail_with_logs

ip -n "$NS_A" addr add 10.89.1.1/30 dev "$TUN_A"
ip -n "$NS_B" addr add 10.89.1.2/30 dev "$TUN_B"
ip -n "$NS_A" link set "$TUN_A" up
ip -n "$NS_B" link set "$TUN_B" up
ip netns exec "$NS_A" ping -6 -c 1 -W 1 fd31:251::2 >/dev/null || fail_with_logs

frag_a_before="$(snmp6_value "$NS_A" Ip6FragCreates)"
frag_b_before="$(snmp6_value "$NS_B" Ip6FragCreates)"
reasm_a_before="$(snmp6_value "$NS_A" Ip6ReasmReqds)"
reasm_b_before="$(snmp6_value "$NS_B" Ip6ReasmReqds)"

wait_for "ip netns exec '$NS_A' ping -c 1 -W 1 10.89.1.2 >/dev/null 2>&1" || fail_with_logs
wait_for "ip netns exec '$NS_B' ping -c 1 -W 1 10.89.1.1 >/dev/null 2>&1" || fail_with_logs
wait_for "ip netns exec '$NS_A' ping -M do -c 2 -W 2 -s 1372 10.89.1.2 >/dev/null 2>&1" || fail_with_logs
wait_for "ip netns exec '$NS_B' ping -M do -c 2 -W 2 -s 1372 10.89.1.1 >/dev/null 2>&1" || fail_with_logs

[[ "$(snmp6_value "$NS_A" Ip6FragCreates)" == "$frag_a_before" ]] || fail_with_logs
[[ "$(snmp6_value "$NS_B" Ip6FragCreates)" == "$frag_b_before" ]] || fail_with_logs
[[ "$(snmp6_value "$NS_A" Ip6ReasmReqds)" == "$reasm_a_before" ]] || fail_with_logs
[[ "$(snmp6_value "$NS_B" Ip6ReasmReqds)" == "$reasm_b_before" ]] || fail_with_logs

echo "embedded Xray mKCP/TUN over 1280-byte IPv6 underlay without IP fragmentation: ok"
