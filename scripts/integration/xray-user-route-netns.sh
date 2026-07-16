#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/xray-user-route"
CORE_BIN="${BUILD_DIR}/tapx-core"
XRAY_BIN="${BUILD_DIR}/xray"
RUNTIME="${1:-embedded}"
DEVICE_CASE="${2:-separate}"
ROUTE_DEVICE_ID="tun-route"
[[ "$DEVICE_CASE" != "same" ]] || ROUTE_DEVICE_ID="tun-default"
NS_A="tapx-xroute-a"
NS_B="tapx-xroute-b"
VETH_A="txroute-a"
VETH_B="txroute-b"
TUN_ROUTE="txroute-tun-a"
TUN_DEFAULT="txroute-def-a"
TUN_CLIENT="txroute-tun-b"
PID_A=""
PID_B=""

cleanup() {
  set +e
  [[ -z "$PID_A" ]] || { kill "$PID_A" >/dev/null 2>&1; wait "$PID_A" >/dev/null 2>&1; }
  [[ -z "$PID_B" ]] || { kill "$PID_B" >/dev/null 2>&1; wait "$PID_B" >/dev/null 2>&1; }
  ip netns delete "$NS_A" >/dev/null 2>&1
  ip netns delete "$NS_B" >/dev/null 2>&1
}

fail_with_logs() {
  echo "${RUNTIME} Xray authenticated-user route integration failed" >&2
  sed -n '1,220p' "${BUILD_DIR}/tapx-a.log" >&2 || true
  sed -n '1,220p' "${BUILD_DIR}/tapx-b.log" >&2 || true
  exit 1
}

wait_for() {
  local command="$1"
  for _ in $(seq 1 200); do
    eval "$command" && return 0
    sleep 0.05
  done
  return 1
}

write_client_config() {
  local uuid="$1"
  cat >"${BUILD_DIR}/tapx-b.json" <<JSON
{
  "Devices": [{"ID":"tun-b","Enabled":true,"Type":"tun","IfName":"${TUN_CLIENT}","MTU":1400}],
  "XrayProfiles": [{
    "ID":"xray-client","Enabled":true,"Runtime":"${RUNTIME}",
    "OutboundProtocol":"vless",
    "OutboundSettingsJSON":"{\"vnext\":[{\"address\":\"198.18.70.1\",\"port\":44431,\"users\":[{\"id\":\"${uuid}\",\"encryption\":\"none\"}]}]}",
    "Network":"tcp","Security":"none","StreamSettingsJSON":"{}"
  }],
  "Connectors": [{
    "ID":"xray-b","Enabled":true,"Remote":"tapx.frame.local","Port":1,
    "Transport":"xray","XrayProfileID":"xray-client","RawTCP":{"LengthMode":"uint16"},
    "Binding":{"DeviceID":"tun-b"}
  }],
  "Settings": [{"ID":"global","Enabled":true,"ExternalXrayPath":"${XRAY_BIN}","DataDir":"${BUILD_DIR}/data-b","LogLevel":"warn"}]
}
JSON
}

start_client() {
  : >"${BUILD_DIR}/tapx-b.log"
  ip netns exec "$NS_B" "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" >"${BUILD_DIR}/tapx-b.log" 2>&1 & PID_B="$!"
  wait_for "grep -q 'runtime started' '${BUILD_DIR}/tapx-b.log' 2>/dev/null" || fail_with_logs
  wait_for "ip -n '$NS_B' link show '$TUN_CLIENT' >/dev/null 2>&1" || fail_with_logs
  ip -n "$NS_B" addr add 10.97.0.2/30 dev "$TUN_CLIENT"
  ip -n "$NS_B" addr add 10.97.0.99/32 dev "$TUN_CLIENT"
  ip -n "$NS_B" link set "$TUN_CLIENT" up
}

stop_client() {
  kill "$PID_B" >/dev/null 2>&1 || true
  wait "$PID_B" >/dev/null 2>&1 || true
  PID_B=""
  wait_for "! ip -n '$NS_B' link show '$TUN_CLIENT' >/dev/null 2>&1" || fail_with_logs
}

[[ "$(id -u)" == "0" ]] || { echo "SKIP: test requires root or CAP_NET_ADMIN"; exit 0; }
[[ "$RUNTIME" == "embedded" || "$RUNTIME" == "external" ]] || { echo "usage: $0 [embedded|external]" >&2; exit 2; }
[[ "$DEVICE_CASE" == "separate" || "$DEVICE_CASE" == "same" ]] || { echo "usage: $0 [embedded|external] [separate|same]" >&2; exit 2; }
for tool in go ip ping grep sed; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing required command: $tool" >&2; exit 1; }
done

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}"/{tapx-a,tapx-b}.{json,log}
(cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -o "$CORE_BIN" ./cmd/tapx-core)
if [[ "$RUNTIME" == "external" ]]; then
  cp "${ROOT}/build/lab/xray-linux-amd64" "$XRAY_BIN"
  chmod 0755 "$XRAY_BIN"
fi

cat >"${BUILD_DIR}/tapx-a.json" <<JSON
{
  "Devices": [
    {"ID":"tun-route","Enabled":true,"Type":"tun","IfName":"${TUN_ROUTE}","MTU":1400},
    {"ID":"tun-default","Enabled":true,"Type":"tun","IfName":"${TUN_DEFAULT}","MTU":1400}
  ],
  "XrayProfiles": [{
    "ID":"xray-server","Enabled":true,"Runtime":"${RUNTIME}",
    "InboundProtocol":"vless","InboundSettingsJSON":"{\"decryption\":\"none\"}",
    "Network":"tcp","Security":"none","StreamSettingsJSON":"{}"
  }],
  "Listeners": [{
    "ID":"xray-a","Enabled":true,"BindHost":"198.18.70.1","BindPort":44431,
    "Transport":"xray","XrayProfileID":"xray-server","RawTCP":{"LengthMode":"uint16"},
    "Binding":{"DeviceID":"tun-default"}
  }],
  "Clients": [
    {"ID":"allow-user","Enabled":true,"Email":"allow@tapx.test","UUID":"11111111-1111-4111-8111-111111111111","ListenerID":"xray-a","AddressID":"allow-address","AllowedDeviceIDs":["${ROUTE_DEVICE_ID}"]},
    {"ID":"drop-user","Enabled":true,"Email":"drop@tapx.test","UUID":"22222222-2222-4222-8222-222222222222","ListenerID":"xray-a"}
  ],
  "Addresses": [
    {"ID":"allow-address","Enabled":true,"DeviceID":"${ROUTE_DEVICE_ID}","ClientID":"allow-user","IPv4CIDRs":["10.97.0.2/32"]}
  ],
  "Routes": [
    {"ID":"allow-route","Enabled":true,"Priority":10,"Action":"bind-device","ListenerID":"xray-a","ClientID":"allow-user","DeviceID":"${ROUTE_DEVICE_ID}","AddressID":"allow-address"},
    {"ID":"drop-route","Enabled":true,"Priority":20,"Action":"drop","ListenerID":"xray-a","ClientID":"drop-user"}
  ],
  "Settings": [{"ID":"global","Enabled":true,"ExternalXrayPath":"${XRAY_BIN}","DataDir":"${BUILD_DIR}/data-a","LogLevel":"warn"}]
}
JSON

ip netns add "$NS_A"
ip netns add "$NS_B"
ip link add "$VETH_A" type veth peer name "$VETH_B"
ip link set "$VETH_A" netns "$NS_A"
ip link set "$VETH_B" netns "$NS_B"
ip -n "$NS_A" link set lo up
ip -n "$NS_B" link set lo up
ip -n "$NS_A" addr add 198.18.70.1/30 dev "$VETH_A"
ip -n "$NS_B" addr add 198.18.70.2/30 dev "$VETH_B"
ip -n "$NS_A" link set "$VETH_A" up
ip -n "$NS_B" link set "$VETH_B" up

ip netns exec "$NS_A" "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" >"${BUILD_DIR}/tapx-a.log" 2>&1 & PID_A="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/tapx-a.log' 2>/dev/null" || fail_with_logs
wait_for "ip -n '$NS_A' link show '$TUN_DEFAULT' >/dev/null 2>&1" || fail_with_logs
if [[ "$DEVICE_CASE" == "separate" ]]; then
  wait_for "ip -n '$NS_A' link show '$TUN_ROUTE' >/dev/null 2>&1" || fail_with_logs
  ip -n "$NS_A" addr add 10.97.0.1/30 dev "$TUN_ROUTE"
  ip -n "$NS_A" route add 10.97.0.99/32 dev "$TUN_ROUTE"
  ip -n "$NS_A" link set "$TUN_ROUTE" up
else
  ip -n "$NS_A" addr add 10.97.0.1/30 dev "$TUN_DEFAULT"
  ip -n "$NS_A" route add 10.97.0.99/32 dev "$TUN_DEFAULT"
fi
ip -n "$NS_A" addr add 10.98.0.1/30 dev "$TUN_DEFAULT"
ip -n "$NS_A" link set "$TUN_DEFAULT" up

write_client_config "11111111-1111-4111-8111-111111111111"
start_client
wait_for "ip netns exec '$NS_B' ping -c 1 -W 1 10.97.0.1 >/dev/null 2>&1" || fail_with_logs
ip netns exec "$NS_A" ping -c 1 -W 1 10.97.0.2 >/dev/null || fail_with_logs
if ip netns exec "$NS_B" ping -I 10.97.0.99 -c 2 -W 1 10.97.0.1 >/dev/null 2>&1; then
  echo "allow-user spoofed source bypassed its address limit" >&2
  fail_with_logs
fi
if ip netns exec "$NS_A" ping -c 2 -W 1 10.97.0.99 >/dev/null 2>&1; then
  echo "allow-user received traffic outside its address limit" >&2
  fail_with_logs
fi
if [[ "$DEVICE_CASE" == "separate" ]] && ip netns exec "$NS_B" ping -c 1 -W 1 10.98.0.1 >/dev/null 2>&1; then
  echo "allow-user incorrectly reached the fallback device" >&2
  fail_with_logs
fi
stop_client

write_client_config "22222222-2222-4222-8222-222222222222"
start_client
if ip netns exec "$NS_B" ping -c 2 -W 1 10.97.0.1 >/dev/null 2>&1; then
  echo "drop-user unexpectedly reached the routed device" >&2
  fail_with_logs
fi
if ip netns exec "$NS_B" ping -c 2 -W 1 10.98.0.1 >/dev/null 2>&1; then
  echo "drop-user unexpectedly reached the fallback device" >&2
  fail_with_logs
fi

echo "${RUNTIME} Xray authenticated-user bind/drop routing (${DEVICE_CASE} devices): ok"
