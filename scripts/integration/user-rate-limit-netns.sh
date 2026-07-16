#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT}/build/integration/rate-limit"
CORE_BIN="${BUILD_DIR}/tapx-core"
NS_A="tapx-rate-a"
NS_B="tapx-rate-b"
PID_A=""
PID_B=""
TRANSPORT="${TRANSPORT:-tcp}"
XRAY_BIN="${XRAY_BIN:-${ROOT}/build/lab/xray-linux-amd64}"

cleanup() {
  set +e
  [[ -n "$PID_A" ]] && kill "$PID_A" >/dev/null 2>&1 && wait "$PID_A" >/dev/null 2>&1
  [[ -n "$PID_B" ]] && kill "$PID_B" >/dev/null 2>&1 && wait "$PID_B" >/dev/null 2>&1
  ip netns delete "$NS_A" >/dev/null 2>&1
  ip netns delete "$NS_B" >/dev/null 2>&1
}

fail() {
  echo "user rate-limit integration failed" >&2
  sed -n '1,160p' "${BUILD_DIR}/tapx-a.log" >&2 || true
  sed -n '1,160p' "${BUILD_DIR}/tapx-b.log" >&2 || true
  exit 1
}

wait_for() {
  local command="$1"
  for _ in $(seq 1 100); do
    if eval "$command"; then return 0; fi
    sleep 0.05
  done
  return 1
}

check_rate() {
  local file="$1"
  local direction="$2"
  local minimum="$3"
  local maximum="$4"
  python3 - "$file" "$direction" "$minimum" "$maximum" <<'PY'
import json, sys
path, direction, minimum, maximum = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4])
data = json.load(open(path))
section = "sum_received"
rate = float(data["end"][section]["bits_per_second"])
print(f"{direction}: {rate / 1_000_000:.3f} Mbps")
if not minimum <= rate <= maximum:
    raise SystemExit(f"{direction} rate {rate} outside [{minimum}, {maximum}]")
PY
}

if [[ "$(id -u)" != "0" ]]; then
  echo "SKIP: user rate-limit integration requires root or CAP_NET_ADMIN"
  exit 0
fi
for command in go ip iperf3 python3; do command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }; done

mkdir -p "$BUILD_DIR"
trap cleanup EXIT
cleanup
rm -f "${BUILD_DIR}"/*.json "${BUILD_DIR}"/*.log
(cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -o "$CORE_BIN" ./cmd/tapx-core)

if [[ "$TRANSPORT" == "tcp" ]]; then
cat >"${BUILD_DIR}/tapx-a.json" <<'JSON'
{
  "Devices": [{"ID":"tun-a","Enabled":true,"Type":"tun","IfName":"tapxrate0","MTU":1400}],
  "Clients": [{
    "ID":"client-a","Enabled":true,"ListenerIDs":["tcp-a"],
    "UploadRateLimit":4000000,"DownloadRateLimit":8000000,
    "Binding":{"DeviceID":"tun-a"}
  }],
  "Listeners": [{
    "ID":"tcp-a","Enabled":true,"BindHost":"172.31.249.1","BindPort":44900,
    "Transport":"tcp","RawTCP":{"LengthMode":"uint16","NoDelay":true},
    "Binding":{"ClientID":"client-a"}
  }]
}
JSON

cat >"${BUILD_DIR}/tapx-b.json" <<'JSON'
{
  "Devices": [{"ID":"tun-b","Enabled":true,"Type":"tun","IfName":"tapxrate1","MTU":1400}],
  "Connectors": [{
    "ID":"tcp-b","Enabled":true,"Remote":"172.31.249.1","Port":44900,
    "Transport":"tcp","RawTCP":{"LengthMode":"uint16","NoDelay":true,"ConnectTimeout":3},
    "Binding":{"DeviceID":"tun-b"}
  }]
}
JSON
elif [[ "$TRANSPORT" == "xray" || "$TRANSPORT" == "xray-external" ]]; then
  runtime="embedded"
  if [[ "$TRANSPORT" == "xray-external" ]]; then
    runtime="external"
    if [[ ! -x "$XRAY_BIN" ]]; then
      mkdir -p "$(dirname "$XRAY_BIN")"
      (cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" CGO_ENABLED=0 go build -o "$XRAY_BIN" github.com/xtls/xray-core/main)
    fi
  fi
  cat >"${BUILD_DIR}/tapx-a.json" <<JSON
{
  "Devices": [{"ID":"tun-a","Enabled":true,"Type":"tun","IfName":"tapxrate0","MTU":1400}],
  "Clients": [{
    "ID":"client-a","Enabled":true,"ListenerIDs":["xray-a"],
    "UUID":"11111111-1111-4111-8111-111111111111",
    "UploadRateLimit":4000000,"DownloadRateLimit":8000000,
    "Binding":{"DeviceID":"tun-a"}
  }],
  "XrayProfiles": [{
    "ID":"xray-server","Enabled":true,"Runtime":"$runtime",
    "InboundProtocol":"vless",
    "InboundSettingsJSON":"{\"clients\":[{\"id\":\"11111111-1111-4111-8111-111111111111\",\"level\":0}],\"decryption\":\"none\"}",
    "Network":"tcp","Security":"none","StreamSettingsJSON":"{}",
    "AdvancedJSON":"{\"outbounds\":[{\"tag\":\"direct\",\"protocol\":\"freedom\"}]}"
  }],
  "Listeners": [{
    "ID":"xray-a","Enabled":true,"BindHost":"172.31.249.1","BindPort":44900,
    "Transport":"xray","XrayProfileID":"xray-server","RawTCP":{"LengthMode":"uint16"},
    "Binding":{"ClientID":"client-a"}
  }],
  "Settings": [{"ID":"global","Enabled":true,"ExternalXrayPath":"$XRAY_BIN","DataDir":"${BUILD_DIR}/xray-a"}]
}
JSON
  cat >"${BUILD_DIR}/tapx-b.json" <<JSON
{
  "Devices": [{"ID":"tun-b","Enabled":true,"Type":"tun","IfName":"tapxrate1","MTU":1400}],
  "XrayProfiles": [{
    "ID":"xray-client","Enabled":true,"Runtime":"$runtime",
    "OutboundProtocol":"vless",
    "OutboundSettingsJSON":"{\"vnext\":[{\"address\":\"172.31.249.1\",\"port\":44900,\"users\":[{\"id\":\"11111111-1111-4111-8111-111111111111\",\"encryption\":\"none\"}]}]}",
    "Network":"tcp","Security":"none","StreamSettingsJSON":"{}"
  }],
  "Connectors": [{
    "ID":"xray-b","Enabled":true,"Remote":"tapx.frame.local","Port":1,
    "Transport":"xray","XrayProfileID":"xray-client","RawTCP":{"LengthMode":"uint16"},
    "Binding":{"DeviceID":"tun-b"}
  }],
  "Settings": [{"ID":"global","Enabled":true,"ExternalXrayPath":"$XRAY_BIN","DataDir":"${BUILD_DIR}/xray-b"}]
}
JSON
else
  echo "unsupported TRANSPORT=$TRANSPORT (want tcp, xray, or xray-external)" >&2
  exit 1
fi

ip netns add "$NS_A"
ip netns add "$NS_B"
ip link add tapxrate-a type veth peer name tapxrate-b
ip link set tapxrate-a netns "$NS_A"
ip link set tapxrate-b netns "$NS_B"
ip -n "$NS_A" link set lo up
ip -n "$NS_B" link set lo up
ip -n "$NS_A" addr add 172.31.249.1/30 dev tapxrate-a
ip -n "$NS_B" addr add 172.31.249.2/30 dev tapxrate-b
ip -n "$NS_A" link set tapxrate-a up
ip -n "$NS_B" link set tapxrate-b up

ip netns exec "$NS_A" "$CORE_BIN" -config "${BUILD_DIR}/tapx-a.json" >"${BUILD_DIR}/tapx-a.log" 2>&1 & PID_A="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/tapx-a.log' 2>/dev/null" || fail
ip netns exec "$NS_B" "$CORE_BIN" -config "${BUILD_DIR}/tapx-b.json" >"${BUILD_DIR}/tapx-b.log" 2>&1 & PID_B="$!"
wait_for "grep -q 'runtime started' '${BUILD_DIR}/tapx-b.log' 2>/dev/null" || fail
wait_for "ip -n '$NS_A' link show tapxrate0 >/dev/null 2>&1" || fail
wait_for "ip -n '$NS_B' link show tapxrate1 >/dev/null 2>&1" || fail
ip -n "$NS_A" addr add 10.91.0.1/30 dev tapxrate0
ip -n "$NS_B" addr add 10.91.0.2/30 dev tapxrate1
ip -n "$NS_A" link set tapxrate0 up
ip -n "$NS_B" link set tapxrate1 up
wait_for "ip netns exec '$NS_B' ping -c 1 -W 1 10.91.0.1 >/dev/null 2>&1" || fail

ip netns exec "$NS_A" iperf3 -s -1 >"${BUILD_DIR}/iperf-upload-server.log" 2>&1 &
sleep 0.2
ip netns exec "$NS_B" iperf3 -c 10.91.0.1 -t 4 -J >"${BUILD_DIR}/upload.json"
check_rate "${BUILD_DIR}/upload.json" upload 3000000 5500000

ip netns exec "$NS_A" iperf3 -s -1 >"${BUILD_DIR}/iperf-download-server.log" 2>&1 &
sleep 0.2
ip netns exec "$NS_B" iperf3 -c 10.91.0.1 -R -t 4 -J >"${BUILD_DIR}/download.json"
check_rate "${BUILD_DIR}/download.json" download 6000000 10500000

echo "user rate-limit netns integration ($TRANSPORT): ok"
