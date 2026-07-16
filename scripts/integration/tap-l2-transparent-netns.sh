#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TRANSPORT="${TRANSPORT:-udp}"
[[ "$TRANSPORT" == "udp" || "$TRANSPORT" == "udp-dtls" || "$TRANSPORT" == "tcp" || "$TRANSPORT" == "tcp-tls" || "$TRANSPORT" == "xray" || "$TRANSPORT" == "xray-external" ]] || { echo "TRANSPORT must be udp, udp-dtls, tcp, tcp-tls, xray or xray-external" >&2; exit 1; }
DEVICE_MTU="${DEVICE_MTU:-1500}"
[[ "$DEVICE_MTU" =~ ^[0-9]+$ ]] && (( DEVICE_MTU >= 576 && DEVICE_MTU <= 65513 )) || { echo "DEVICE_MTU must be between 576 and 65513" >&2; exit 1; }
HOST_LINK_MTU=$((DEVICE_MTU + 22))
BUILD_DIR="${ROOT}/build/integration/tap-l2-transparent-${TRANSPORT}"
XRAY_BIN="${XRAY_BIN:-${ROOT}/build/lab/xray-linux-amd64}"
XRAY_RUNTIME="embedded"
[[ "$TRANSPORT" == "xray-external" ]] && XRAY_RUNTIME="external"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-l2-a"
NS_B="tapx-l2-b"
HOST_A="tapx-l2-host-a"
HOST_B="tapx-l2-host-b"
TAP_A="tapxl2a0"
TAP_B="tapxl2b0"
UNDERLAY_A="tapxul2a"
UNDERLAY_B="tapxul2b"
LAN_A="tapxlan2a"
LAN_B="tapxlan2b"
HOST_IF_A="eth-l2a"
HOST_IF_B="eth-l2b"
BRIDGE_A="br-tapxl2a"
BRIDGE_B="br-tapxl2b"
MAC_A="02:00:00:00:aa:11"
MAC_B="02:00:00:00:bb:11"
PID_A=""
PID_B=""

cleanup() {
  set +e
  [[ -n "$PID_A" ]] && kill "$PID_A" >/dev/null 2>&1 && wait "$PID_A" >/dev/null 2>&1
  [[ -n "$PID_B" ]] && kill "$PID_B" >/dev/null 2>&1 && wait "$PID_B" >/dev/null 2>&1
  for ns in "$HOST_A" "$HOST_B" "$NS_A" "$NS_B"; do ip netns delete "$ns" >/dev/null 2>&1; done
}

fail() {
  echo "TAP L2 transparent integration failed" >&2
  for log in "$BUILD_DIR/a.log" "$BUILD_DIR/b.log"; do
    [[ -f "$log" ]] && { echo "== $log ==" >&2; sed -n '1,160p' "$log" >&2; }
  done
  exit 1
}

wait_link() {
  local ns="$1" name="$2"
  for _ in $(seq 1 100); do
    ip -n "$ns" link show dev "$name" >/dev/null 2>&1 && return 0
    sleep 0.05
  done
  return 1
}

expect_frame() {
  local receiver_ns="$1" receiver_if="$2" sender_ns="$3" sender_if="$4"
  local destination="$5" source="$6" ether_type="$7" payload_hex="$8" marker="$9"
  local result="$BUILD_DIR/frame-result"
  rm -f "$result"
  ip netns exec "$receiver_ns" python3 "$BUILD_DIR/frame.py" receive "$receiver_if" "$marker" >"$result" &
  local receiver_pid=$!
  sleep 0.12
  ip netns exec "$sender_ns" python3 "$BUILD_DIR/frame.py" send "$sender_if" "$destination" "$source" "$ether_type" "$payload_hex"
  wait "$receiver_pid" || {
    echo "missing frame: sender=${sender_ns}/${sender_if} receiver=${receiver_ns}/${receiver_if} dst=${destination} src=${source} etherType=${ether_type} marker=${marker}" >&2
    cat "$result" >&2 || true
    ip -n "$NS_A" -s link show "$TAP_A" >&2 || true
    ip -n "$NS_B" -s link show "$TAP_B" >&2 || true
    ip -n "$HOST_A" -s link show "$HOST_IF_A" >&2 || true
    ip -n "$HOST_B" -s link show "$HOST_IF_B" >&2 || true
    ip netns exec "$NS_A" bridge fdb show br "$BRIDGE_A" >&2 || true
    ip netns exec "$NS_B" bridge fdb show br "$BRIDGE_B" >&2 || true
    ip netns exec "$NS_A" ss -uanp >&2 || true
    ip netns exec "$NS_B" ss -uanp >&2 || true
    fail
  }
  grep -q '^received ' "$result" || fail
}

[[ "$(id -u)" == "0" ]] || { echo "SKIP: root or CAP_NET_ADMIN required"; exit 0; }
for command in go ip bridge tc python3 grep sed; do command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }; done
if [[ "$TRANSPORT" == "udp-dtls" || "$TRANSPORT" == "tcp-tls" ]]; then
  command -v openssl >/dev/null || { echo "missing openssl" >&2; exit 1; }
fi
if [[ "$TRANSPORT" == "xray-external" && ! -x "$XRAY_BIN" ]]; then
  echo "missing executable external Xray binary: $XRAY_BIN" >&2
  exit 1
fi

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "$BUILD_DIR"/{a.log,b.log,a.json,b.json,frame-result,server.crt,server.key}

(cd "$ROOT" && go build -o "$CORE_BIN" ./cmd/tapx-core)

for ns in "$NS_A" "$NS_B" "$HOST_A" "$HOST_B"; do ip netns add "$ns"; ip -n "$ns" link set lo up; done
ip link add "$UNDERLAY_A" type veth peer name "$UNDERLAY_B"
ip link set "$UNDERLAY_A" netns "$NS_A"
ip link set "$UNDERLAY_B" netns "$NS_B"
ip -n "$NS_A" addr add 172.31.252.1/30 dev "$UNDERLAY_A"
ip -n "$NS_B" addr add 172.31.252.2/30 dev "$UNDERLAY_B"
ip -n "$NS_A" link set "$UNDERLAY_A" mtu 1280 up
ip -n "$NS_B" link set "$UNDERLAY_B" mtu 1280 up

ip link add "$LAN_A" type veth peer name "$HOST_IF_A"
ip link set "$LAN_A" netns "$NS_A"
ip link set "$HOST_IF_A" netns "$HOST_A"
ip link add "$LAN_B" type veth peer name "$HOST_IF_B"
ip link set "$LAN_B" netns "$NS_B"
ip link set "$HOST_IF_B" netns "$HOST_B"
ip -n "$NS_A" link set "$LAN_A" mtu "$HOST_LINK_MTU"
ip -n "$NS_B" link set "$LAN_B" mtu "$HOST_LINK_MTU"
ip -n "$HOST_A" link set "$HOST_IF_A" mtu "$HOST_LINK_MTU" address "$MAC_A" up
ip -n "$HOST_B" link set "$HOST_IF_B" mtu "$HOST_LINK_MTU" address "$MAC_B" up

if [[ "$TRANSPORT" == "udp" ]]; then
cat >"$BUILD_DIR/a.json" <<JSON
{
  "Devices": [{"ID":"tap-a","Enabled":true,"Type":"tap","IfName":"$TAP_A","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_A","IfName":"$LAN_A","MTU":$DEVICE_MTU}}],
  "Routes": [{"ID":"route-a","Enabled":true,"DeviceID":"tap-a"}],
  "Listeners": [{"ID":"udp-a","Enabled":true,"BindHost":"172.31.252.1","BindPort":44200,"Transport":"udp","RawUDP":{"PeerMode":"fixed","FixedPeer":"172.31.252.2:44200"},"Binding":{"RouteID":"route-a"}}]
}
JSON
cat >"$BUILD_DIR/b.json" <<JSON
{
  "Devices": [{"ID":"tap-b","Enabled":true,"Type":"tap","IfName":"$TAP_B","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_B","IfName":"$LAN_B","MTU":$DEVICE_MTU}}],
  "Routes": [{"ID":"route-b","Enabled":true,"DeviceID":"tap-b"}],
  "Listeners": [{"ID":"udp-b","Enabled":true,"BindHost":"172.31.252.2","BindPort":44200,"Transport":"udp","RawUDP":{"PeerMode":"fixed","FixedPeer":"172.31.252.1:44200"},"Binding":{"RouteID":"route-b"}}]
}
JSON
elif [[ "$TRANSPORT" == "udp-dtls" ]]; then
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$BUILD_DIR/server.key" -out "$BUILD_DIR/server.crt" -days 1 \
  -subj "/CN=tapx.local" -addext "subjectAltName = DNS:tapx.local,IP:172.31.252.1" >/dev/null 2>&1
cat >"$BUILD_DIR/a.json" <<JSON
{
  "Devices": [{"ID":"tap-a","Enabled":true,"Type":"tap","IfName":"$TAP_A","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_A","IfName":"$LAN_A","MTU":$DEVICE_MTU}}],
  "VKeys": [{"ID":"vk-a","Enabled":true,"Value":"tapx-l2-dtls-vkey"}],
  "Routes": [{"ID":"route-a","Enabled":true,"DeviceID":"tap-a","VKeyID":"vk-a"}],
  "Listeners": [{"ID":"udp-a","Enabled":true,"BindHost":"172.31.252.1","BindPort":44200,"Transport":"udp","RawUDP":{"PeerMode":"learn","DTLS":{"Enabled":true,"CertFile":"$BUILD_DIR/server.crt","KeyFile":"$BUILD_DIR/server.key","ALPN":["tapx"],"ReplayWindow":64}},"Binding":{"RouteID":"route-a"}}]
}
JSON
cat >"$BUILD_DIR/b.json" <<JSON
{
  "Devices": [{"ID":"tap-b","Enabled":true,"Type":"tap","IfName":"$TAP_B","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_B","IfName":"$LAN_B","MTU":$DEVICE_MTU}}],
  "VKeys": [{"ID":"vk-b","Enabled":true,"Value":"tapx-l2-dtls-vkey"}],
  "Routes": [{"ID":"route-b","Enabled":true,"DeviceID":"tap-b","VKeyID":"vk-b"}],
  "Connectors": [{"ID":"udp-b","Enabled":true,"Remote":"172.31.252.1","Port":44200,"Transport":"udp","RawUDP":{"PeerMode":"fixed","FixedPeer":"172.31.252.1:44200","DTLS":{"Enabled":true,"CAFile":"$BUILD_DIR/server.crt","ServerName":"tapx.local","ALPN":["tapx"],"ReplayWindow":64}},"Binding":{"RouteID":"route-b"}}]
}
JSON
elif [[ "$TRANSPORT" == "tcp" || "$TRANSPORT" == "tcp-tls" ]]; then
server_tls=""
client_tls=""
if [[ "$TRANSPORT" == "tcp-tls" ]]; then
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$BUILD_DIR/server.key" -out "$BUILD_DIR/server.crt" -days 1 \
    -subj "/CN=tapx.local" -addext "subjectAltName = DNS:tapx.local,IP:172.31.252.1" >/dev/null 2>&1
  server_tls=',"TLS":{"Enabled":true,"CertFile":"'"$BUILD_DIR"'/server.crt","KeyFile":"'"$BUILD_DIR"'/server.key","ALPN":["tapx"],"MinVersion":"1.2"}'
  client_tls=',"TLS":{"Enabled":true,"CAFile":"'"$BUILD_DIR"'/server.crt","ServerName":"tapx.local","ALPN":["tapx"],"MinVersion":"1.2"}'
fi
cat >"$BUILD_DIR/a.json" <<JSON
{
  "Devices": [{"ID":"tap-a","Enabled":true,"Type":"tap","IfName":"$TAP_A","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_A","IfName":"$LAN_A","MTU":$DEVICE_MTU}}],
  "VKeys": [{"ID":"vk-a","Enabled":true,"Value":"tapx-l2-tcp-vkey"}],
  "Routes": [{"ID":"route-a","Enabled":true,"DeviceID":"tap-a","VKeyID":"vk-a"}],
  "Listeners": [{"ID":"tcp-a","Enabled":true,"BindHost":"172.31.252.1","BindPort":44200,"Transport":"tcp","RawTCP":{"LengthMode":"uint16","NoDelay":true$server_tls},"Binding":{"RouteID":"route-a"}}]
}
JSON
cat >"$BUILD_DIR/b.json" <<JSON
{
  "Devices": [{"ID":"tap-b","Enabled":true,"Type":"tap","IfName":"$TAP_B","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_B","IfName":"$LAN_B","MTU":$DEVICE_MTU}}],
  "VKeys": [{"ID":"vk-b","Enabled":true,"Value":"tapx-l2-tcp-vkey"}],
  "Routes": [{"ID":"route-b","Enabled":true,"DeviceID":"tap-b","VKeyID":"vk-b"}],
  "Connectors": [{"ID":"tcp-b","Enabled":true,"Remote":"172.31.252.1","Port":44200,"Transport":"tcp","RawTCP":{"LengthMode":"uint16","NoDelay":true,"ConnectTimeout":5,"ReconnectSecond":1$client_tls},"Binding":{"RouteID":"route-b"}}]
}
JSON
else
cat >"$BUILD_DIR/a.json" <<JSON
{
  "Devices": [{"ID":"tap-a","Enabled":true,"Type":"tap","IfName":"$TAP_A","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_A","IfName":"$LAN_A","MTU":$DEVICE_MTU}}],
  "Routes": [{"ID":"route-a","Enabled":true,"DeviceID":"tap-a"}],
  "XrayProfiles": [{"ID":"xray-a","Enabled":true,"Runtime":"$XRAY_RUNTIME","InboundProtocol":"vless","InboundSettingsJSON":"{\"clients\":[{\"id\":\"d0e445dc-2368-4d5d-8ccd-e324b0a3a8cd\"}],\"decryption\":\"none\"}","Network":"tcp","Security":"none","AdvancedJSON":"{\"outbounds\":[{\"tag\":\"direct\",\"protocol\":\"freedom\"}]}"}],
  "Listeners": [{"ID":"xray-a","Enabled":true,"BindHost":"172.31.252.1","BindPort":44200,"Transport":"xray","XrayProfileID":"xray-a","RawTCP":{"LengthMode":"uint16"},"Binding":{"RouteID":"route-a"}}],
  "Settings": [{"ID":"global","Enabled":true,"ExternalXrayPath":"$XRAY_BIN","DataDir":"$BUILD_DIR/xray-a"}]
}
JSON
cat >"$BUILD_DIR/b.json" <<JSON
{
  "Devices": [{"ID":"tap-b","Enabled":true,"Type":"tap","IfName":"$TAP_B","MTU":$DEVICE_MTU,"LinkAutoOptimize":true,"Bridge":{"Enabled":true,"Name":"$BRIDGE_B","IfName":"$LAN_B","MTU":$DEVICE_MTU}}],
  "Routes": [{"ID":"route-b","Enabled":true,"DeviceID":"tap-b"}],
  "XrayProfiles": [{"ID":"xray-b","Enabled":true,"Runtime":"$XRAY_RUNTIME","OutboundProtocol":"vless","OutboundSettingsJSON":"{\"vnext\":[{\"address\":\"172.31.252.1\",\"port\":44200,\"users\":[{\"id\":\"d0e445dc-2368-4d5d-8ccd-e324b0a3a8cd\",\"encryption\":\"none\"}]}]}","Network":"tcp","Security":"none"}],
  "Connectors": [{"ID":"xray-b","Enabled":true,"Remote":"tapx.frame.local","Port":1,"Transport":"xray","XrayProfileID":"xray-b","RawTCP":{"LengthMode":"uint16"},"Binding":{"RouteID":"route-b"}}],
  "Settings": [{"ID":"global","Enabled":true,"ExternalXrayPath":"$XRAY_BIN","DataDir":"$BUILD_DIR/xray-b"}]
}
JSON
fi

cat >"$BUILD_DIR/frame.py" <<'PY'
import socket, struct, sys, time

mode, interface = sys.argv[1], sys.argv[2]
s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
s.bind((interface, 0))
if mode == "send":
    destination = bytes.fromhex(sys.argv[3].replace(":", ""))
    source = bytes.fromhex(sys.argv[4].replace(":", ""))
    ether_type = bytes.fromhex(sys.argv[5])
    payload = bytes.fromhex(sys.argv[6])
    s.send(destination + source + ether_type + payload)
else:
    marker = bytes.fromhex(sys.argv[3])
    s.settimeout(3)
    deadline = time.time() + 3
    observed = []
    while time.time() < deadline:
        try:
            frame = s.recv(65535)
        except TimeoutError:
            break
        if marker in frame:
            print("received", len(frame), frame[:22].hex())
            sys.exit(0)
        observed.append(frame[:96].hex())
        observed = observed[-12:]
    for frame in observed:
        print("observed", frame)
    sys.exit(1)
PY

ip netns exec "$NS_A" "$CORE_BIN" -config "$BUILD_DIR/a.json" >"$BUILD_DIR/a.log" 2>&1 & PID_A=$!
ip netns exec "$NS_B" "$CORE_BIN" -config "$BUILD_DIR/b.json" >"$BUILD_DIR/b.log" 2>&1 & PID_B=$!
wait_link "$NS_A" "$TAP_A" || fail
wait_link "$NS_B" "$TAP_B" || fail
wait_link "$NS_A" "$BRIDGE_A" || fail
wait_link "$NS_B" "$BRIDGE_B" || fail
ip netns exec "$NS_A" ip -d link show dev "$BRIDGE_A" | grep -q 'group_fwd_mask 0xfff8' || fail
ip netns exec "$NS_B" ip -d link show dev "$BRIDGE_B" | grep -q 'group_fwd_mask 0xfff8' || fail

echo "verify PPPoE discovery broadcast and bridge MAC learning"
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" ff:ff:ff:ff:ff:ff "$MAC_A" 8863 1109000000080102000461a1b2c3 61a1b2c3
expect_frame "$HOST_A" "$HOST_IF_A" "$HOST_B" "$HOST_IF_B" "$MAC_A" "$MAC_B" 8863 1107000000080102000462a1b2c3 62a1b2c3
ip netns exec "$NS_A" bridge fdb show br "$BRIDGE_A" | grep -qi "$MAC_B dev $TAP_A" || fail
ip netns exec "$NS_B" bridge fdb show br "$BRIDGE_B" | grep -qi "$MAC_A dev $TAP_B" || fail

echo "verify PPPoE session, multicast, unknown unicast, VLAN and QinQ"
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" "$MAC_B" "$MAC_A" 8864 110000010008002163a1b2c3 63a1b2c3
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" 01:00:5e:00:00:01 "$MAC_A" 8863 1109000000080102000464a1b2c3 64a1b2c3
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" 02:00:00:00:cc:11 "$MAC_A" 8863 1109000000080102000465a1b2c3 65a1b2c3
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" "$MAC_B" "$MAC_A" 8100 006488631109000000080102000466a1b2c3 66a1b2c3
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" "$MAC_B" "$MAC_A" 88a8 000a810000648864110000010008002167a1b2c3 67a1b2c3

echo "verify full $HOST_LINK_MTU-byte QinQ/PPPoE Ethernet frame over 1280-byte underlay"
large_payload=$(python3 - "$DEVICE_MTU" <<'PY'
import sys

device_mtu = int(sys.argv[1])
marker = bytes.fromhex("68a1b2c3")
ppp_payload = marker + bytes(device_mtu - 12)
pppoe_length = 2 + len(ppp_payload)
pppoe = bytes.fromhex("11000001") + pppoe_length.to_bytes(2, "big") + bytes.fromhex("0021") + ppp_payload
print((bytes.fromhex("000a810000648864") + pppoe).hex())
PY
)
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" "$MAC_B" "$MAC_A" 88a8 "$large_payload" 68a1b2c3

echo "verify transparent IEEE 802.1D control protocols in both directions"
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" 01:80:c2:00:00:00 "$MAC_A" 0026 0000000069a1b2c3 69a1b2c3
expect_frame "$HOST_A" "$HOST_IF_A" "$HOST_B" "$HOST_IF_B" 01:80:c2:00:00:02 "$MAC_B" 8809 01016aa1b2c3 6aa1b2c3
expect_frame "$HOST_B" "$HOST_IF_B" "$HOST_A" "$HOST_IF_A" 01:80:c2:00:00:03 "$MAC_A" 888e 6ba1b2c3 6ba1b2c3
expect_frame "$HOST_A" "$HOST_IF_A" "$HOST_B" "$HOST_IF_B" 01:80:c2:00:00:0e "$MAC_B" 88cc 6ca1b2c3 6ca1b2c3

echo "tap l2 transparent netns integration ($TRANSPORT, device MTU $DEVICE_MTU): ok"
