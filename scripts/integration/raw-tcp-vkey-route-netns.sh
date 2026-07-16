#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SECURITY="${SECURITY:-none}"
[[ "$SECURITY" == "none" || "$SECURITY" == "tls" ]] || { echo "SECURITY must be none or tls" >&2; exit 1; }
BUILD_DIR="${ROOT}/build/integration/raw-tcp-vkey-route-${SECURITY}"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-tvroute-a"
NS_B="tapx-tvroute-b"
NS_C="tapx-tvroute-c"
BRIDGE="tapx-tvroute-br"
PID_A=""
PID_B=""
PID_C=""
TLS_LISTENER_JSON=""
TLS_CONNECTOR_JSON=""

cleanup() {
  set +e
  for pid in "$PID_A" "$PID_B" "$PID_C"; do
    [[ -z "$pid" ]] || { kill "$pid" >/dev/null 2>&1; wait "$pid" >/dev/null 2>&1; }
  done
  ip netns delete "$NS_A" >/dev/null 2>&1
  ip netns delete "$NS_B" >/dev/null 2>&1
  ip netns delete "$NS_C" >/dev/null 2>&1
  ip link delete "$BRIDGE" >/dev/null 2>&1
}

fail() {
  echo "raw TCP vKey route integration failed" >&2
  for log in "${BUILD_DIR}"/*.log; do
    [[ ! -f "$log" ]] || { echo "== $log ==" >&2; sed -n '1,240p' "$log" >&2; }
  done
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

write_connector() {
  local path="$1" device_id="$2" ifname="$3" key_id="$4" key="$5"
  cat >"$path" <<JSON
{
  "Devices":[{"ID":"${device_id}","Enabled":true,"Type":"tun","IfName":"${ifname}","MTU":1400}],
  "VKeys":[{"ID":"${key_id}","Enabled":true,"Value":"${key}"}],
  "Routes":[{"ID":"route-${device_id}","Enabled":true,"Action":"bind-device","VKeyID":"${key_id}","DeviceID":"${device_id}"}],
  "Connectors":[{
    "ID":"connector-${device_id}","Enabled":true,"Remote":"198.18.81.1","Port":44511,"Transport":"tcp",
    "RawTCP":{"LengthMode":"uint16","NoDelay":true,"ConnectTimeout":3${TLS_CONNECTOR_JSON}},"Binding":{"RouteID":"route-${device_id}"}
  }]
}
JSON
}

start_connector_c() {
  local key="$1"
  write_connector "${BUILD_DIR}/c.json" "tun-c" "tvroute-tun-c" "key-c" "$key"
  : >"${BUILD_DIR}/c.log"
  ip netns exec "$NS_C" "$CORE_BIN" -debug-counters -config "${BUILD_DIR}/c.json" >"${BUILD_DIR}/c.log" 2>&1 & PID_C="$!"
  wait_for "grep -q 'runtime started' '${BUILD_DIR}/c.log' 2>/dev/null" || fail
  wait_for "ip -n '$NS_C' link show tvroute-tun-c >/dev/null 2>&1" || fail
  ip -n "$NS_C" addr add 10.101.2.2/30 dev tvroute-tun-c
  ip -n "$NS_C" link set tvroute-tun-c up
}

stop_connector_c() {
  kill "$PID_C" >/dev/null 2>&1 || true
  wait "$PID_C" >/dev/null 2>&1 || true
  PID_C=""
  wait_for "! ip -n '$NS_C' link show tvroute-tun-c >/dev/null 2>&1" || fail
}

[[ "$(id -u)" == "0" ]] || { echo "SKIP: test requires root or CAP_NET_ADMIN"; exit 0; }
for tool in go ip ping grep sed; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing required command: $tool" >&2; exit 1; }
done
if [[ "$SECURITY" == "tls" ]]; then
  command -v openssl >/dev/null 2>&1 || { echo "missing required command: openssl" >&2; exit 1; }
fi

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}"/*.{json,log}
if [[ "${SKIP_BUILD:-0}" != "1" || ! -x "$CORE_BIN" ]]; then
  (cd "$ROOT" && go build -o "$CORE_BIN" ./cmd/tapx-core)
fi

if [[ "$SECURITY" == "tls" ]]; then
  mkdir -p "${BUILD_DIR}/tls"
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "${BUILD_DIR}/tls/server.key" -out "${BUILD_DIR}/tls/server.crt" \
    -days 1 -subj "/CN=tapx.local" \
    -addext "subjectAltName = DNS:tapx.local,IP:198.18.81.1" >/dev/null 2>&1
  TLS_LISTENER_JSON=",\"TLS\":{\"Enabled\":true,\"CertFile\":\"${BUILD_DIR}/tls/server.crt\",\"KeyFile\":\"${BUILD_DIR}/tls/server.key\",\"MinVersion\":\"1.2\"}"
  TLS_CONNECTOR_JSON=",\"TLS\":{\"Enabled\":true,\"CAFile\":\"${BUILD_DIR}/tls/server.crt\",\"ServerName\":\"tapx.local\",\"MinVersion\":\"1.2\"}"
fi

cat >"${BUILD_DIR}/a.json" <<JSON
{
  "Devices":[
    {"ID":"tun-alpha","Enabled":true,"Type":"tun","IfName":"tvroute-alpha","MTU":1400},
    {"ID":"tun-bravo","Enabled":true,"Type":"tun","IfName":"tvroute-bravo","MTU":1400}
  ],
  "VKeys":[
    {"ID":"key-alpha","Enabled":true,"Value":"alpha"},
    {"ID":"key-bravo","Enabled":true,"Value":"bravo"},
    {"ID":"key-blocked","Enabled":true,"Value":"blocked"}
  ],
  "Addresses":[
    {"ID":"alpha-address","Enabled":true,"DeviceID":"tun-alpha","IPv4CIDRs":["10.101.1.2/32"]},
    {"ID":"bravo-address","Enabled":true,"DeviceID":"tun-bravo","IPv4CIDRs":["10.101.2.2/32"]}
  ],
  "Routes":[
    {"ID":"alpha-route","Enabled":true,"Priority":10,"Action":"bind-device","ListenerID":"raw-in","VKeyID":"key-alpha","DeviceID":"tun-alpha","AddressID":"alpha-address"},
    {"ID":"bravo-route","Enabled":true,"Priority":20,"Action":"bind-device","ListenerID":"raw-in","VKeyID":"key-bravo","DeviceID":"tun-bravo","AddressID":"bravo-address"},
    {"ID":"blocked-route","Enabled":true,"Priority":30,"Action":"drop","ListenerID":"raw-in","VKeyID":"key-blocked"}
  ],
  "Listeners":[{
    "ID":"raw-in","Enabled":true,"BindHost":"198.18.81.1","BindPort":44511,"Transport":"tcp",
    "RawTCP":{"LengthMode":"uint16","NoDelay":true,"ConnectTimeout":3${TLS_LISTENER_JSON}}
  }]
}
JSON

write_connector "${BUILD_DIR}/b.json" "tun-b" "tvroute-tun-b" "key-b" "alpha"

ip link add "$BRIDGE" type bridge
ip link set "$BRIDGE" up
for suffix in a b c; do
  ns_var="NS_${suffix^^}"
  ns="${!ns_var}"
  ip netns add "$ns"
  ip link add "tvroute-${suffix}-host" type veth peer name "tvroute-${suffix}"
  ip link set "tvroute-${suffix}" netns "$ns"
  ip link set "tvroute-${suffix}-host" master "$BRIDGE"
  ip link set "tvroute-${suffix}-host" up
  ip -n "$ns" link set lo up
  ip -n "$ns" link set "tvroute-${suffix}" up
done
ip -n "$NS_A" addr add 198.18.81.1/24 dev tvroute-a
ip -n "$NS_B" addr add 198.18.81.2/24 dev tvroute-b
ip -n "$NS_C" addr add 198.18.81.3/24 dev tvroute-c

ip netns exec "$NS_A" "$CORE_BIN" -debug-counters -config "${BUILD_DIR}/a.json" >"${BUILD_DIR}/a.log" 2>&1 & PID_A="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/a.log' 2>/dev/null" || fail
ip netns exec "$NS_B" "$CORE_BIN" -debug-counters -config "${BUILD_DIR}/b.json" >"${BUILD_DIR}/b.log" 2>&1 & PID_B="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/b.log' 2>/dev/null" || fail
for link in tvroute-alpha tvroute-bravo; do wait_for "ip -n '$NS_A' link show '$link' >/dev/null 2>&1" || fail; done
wait_for "ip -n '$NS_B' link show tvroute-tun-b >/dev/null 2>&1" || fail

ip -n "$NS_A" addr add 10.101.1.1/30 dev tvroute-alpha
ip -n "$NS_A" route add 10.101.1.99/32 dev tvroute-alpha
ip -n "$NS_A" link set tvroute-alpha up
ip -n "$NS_A" addr add 10.101.2.1/30 dev tvroute-bravo
ip -n "$NS_A" link set tvroute-bravo up
ip -n "$NS_B" addr add 10.101.1.2/30 dev tvroute-tun-b
ip -n "$NS_B" addr add 10.101.1.99/32 dev tvroute-tun-b
ip -n "$NS_B" link set tvroute-tun-b up

wait_for "ip netns exec '$NS_B' ping -c 1 -W 1 10.101.1.1 >/dev/null 2>&1" || fail
ip netns exec "$NS_A" ping -c 1 -W 1 10.101.1.2 >/dev/null || fail
if ip netns exec "$NS_B" ping -I 10.101.1.99 -c 2 -W 1 10.101.1.1 >/dev/null 2>&1; then
  echo "alpha vKey bypassed remote source address limit" >&2; fail
fi
if ip netns exec "$NS_A" ping -I 10.101.1.1 -c 2 -W 1 10.101.1.99 >/dev/null 2>&1; then
  echo "alpha vKey received traffic outside remote destination limit" >&2; fail
fi
if ip netns exec "$NS_B" ping -c 2 -W 1 10.101.2.1 >/dev/null 2>&1; then
  echo "alpha vKey reached bravo device" >&2; fail
fi

start_connector_c "bravo"
wait_for "ip netns exec '$NS_C' ping -c 1 -W 1 10.101.2.1 >/dev/null 2>&1" || fail
ip netns exec "$NS_A" ping -c 1 -W 1 10.101.2.2 >/dev/null || fail
if ip netns exec "$NS_C" ping -c 2 -W 1 10.101.1.1 >/dev/null 2>&1; then
  echo "bravo vKey reached alpha device" >&2; fail
fi
stop_connector_c

# Reusing the same vKey after disconnect proves the listener released its C
# worker session and can classify a replacement connection.
start_connector_c "bravo"
wait_for "ip netns exec '$NS_C' ping -c 1 -W 1 10.101.2.1 >/dev/null 2>&1" || fail
stop_connector_c

start_connector_c "blocked"
if ip netns exec "$NS_C" ping -c 2 -W 1 10.101.2.1 >/dev/null 2>&1; then
  echo "explicitly blocked vKey reached a device" >&2; fail
fi
stop_connector_c

start_connector_c "unknown"
if ip netns exec "$NS_C" ping -c 2 -W 1 10.101.2.1 >/dev/null 2>&1; then
  echo "unknown vKey reached a device" >&2; fail
fi

echo "raw TCP/${SECURITY} vKey multi-route, reconnect, address-limit, drop and unknown-key dispatch: ok"
