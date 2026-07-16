# Verification

## Database backup and restore

- SQLite is the default backend; PostgreSQL is selectable with `-db-driver postgres` or `TAPX_DB_DRIVER=postgres`.
- `GET /api/backup` always returns one portable TapX `.db`. SQLite uses its online backup API; PostgreSQL exports TapX-owned state into the same application format.
- `POST /api/backup/restore` rejects JSON, truncated SQLite, integrity failures, missing TapX tables, and invalid saved configuration.
- SQLite restore keeps a private pre-restore snapshot and rolls back on failure. PostgreSQL restore replaces TapX tables in one transaction.
- Automated tests cover SQLite and real PostgreSQL config, integrations, logs, metrics, `.db` export/restore, and SQLite/PostgreSQL migration in both directions.

TapX has two verification layers:

- local structural checks that can run on Windows, Linux, or CI,
- Linux/network checks that require WSL/Linux with TAP/TUN and network
  namespace permissions.

## Local Structural Checks

```bash
make verify-local
```

This runs `go run ./scripts/verify` and checks:

- required source/package files exist,
- JSON files under `docs/` and `openwrt/` parse,
- example runtime configs generate successfully,
- OpenWrt package source definitions and LuCI integration,
- known private lab markers are absent outside ignored paths.

For release artifacts:

```bash
make package-openwrt-x86
make verify-release
```

`verify-release` requires all three native OpenWrt packages. IPK artifacts are
unpacked to verify control metadata, conffiles, and installed paths; APK
artifacts must be emitted together by the selected OpenWrt SDK.

## Linux Runtime Checks

Run these in WSL/Linux:

```bash
make test
go test -race ./internal/pathmtu ./internal/core
make -C core/fastpath clean test
make integration-netns
make integration-address-guard-netns
make build-linux-amd64
make build-openwrt-x86
```

The C worker accumulates counters locally while it drains one epoll batch,
publishes that batch with relaxed atomic additions before sleeping, and exports
one atomic snapshot to Go. Statistics collection therefore has no C data race,
per-frame forwarding avoids cross-core atomic increments, and cgo remains
outside the per-frame path. The same C suite also passes an
AddressSanitizer/UndefinedBehaviorSanitizer build.

The Device MTU defaults to 1500. Automatic link optimization does not lower
the TUN/TAP interface MTU: Raw UDP confirms the outer path, segments frames
that exceed one safe datagram, and reassembles them at the peer. After
confirmation it replaces the temporary PMTU MSS rule with separate exact IPv4
and IPv6 values. MSS optimization reduces avoidable inner TCP segmentation;
it is not used as a substitute for UDP or Ethernet-frame segmentation.

`DEVICE_MTU=9000 TRANSPORT=udp|xray|xray-external
scripts/integration/tap-l2-transparent-netns.sh` carries a 9022-byte
QinQ/PPPoE Ethernet frame over a 1280-byte underlay in all three runtimes. This
verifies optional jumbo-frame support without making jumbo frames the default.

`make integration-netns` requires root or CAP_NET_ADMIN. It verifies the raw
UDP/TCP TUN/TAP paths, vKey path, Address Guard allow/drop behavior, and
tapx-net apply/rollback slices. The matrix also runs Raw UDP/TUN and Raw
TCP/TUN over an IPv6 underlay constrained to the IPv6 minimum MTU of 1280;
the UDP case asserts that outer IPv6 fragmentation and reassembly counters do
not increase while oversized inner packets cross through TapX segmentation.

For installer validation without touching real `/etc/tapx`, install into a
temporary root and force the database/unit paths into that root:

```bash
tmp="$(mktemp -d /tmp/tapx-install-test-XXXXXX)"
env \
  TAPX_NONINTERACTIVE=1 \
  TAPX_BUILD_DIR="$(pwd)/build/linux-amd64" \
  TAPX_PREFIX="$tmp/prefix" \
  TAPX_SYSCONFDIR="$tmp/etc" \
  TAPX_SYSTEMD_UNIT_DIR="$tmp/systemd" \
  TAPX_DB_DRIVER=sqlite \
  TAPX_DB_SOURCE="$tmp/state/tapx.db" \
  TAPX_PANEL_HOST=127.0.0.1 \
  TAPX_PANEL_PORT=18080 \
  TAPX_PANEL_BASE_PATH=/tapx-test \
  TAPX_ADMIN_USERNAME=admin \
  TAPX_ADMIN_PASSWORD=testpass \
  TAPX_ENABLE_SERVICE=n \
  TAPX_START_SERVICE=n \
  ./scripts/install/linux-install.sh install
"$tmp/prefix/bin/tapx-panel" -db-driver sqlite -db "$tmp/state/tapx.db" -check
rm -rf "$tmp"
```

The PostgreSQL integration test creates an isolated schema and is opt-in:

```bash
TAPX_TEST_POSTGRES_DSN='postgres://tapx:password@127.0.0.1:5432/tapx_test?sslmode=require' \
  go test ./internal/panel -run TestPostgresStoreAndPortableBackupRoundTrip -count=1 -v
```

## Public Lab Checks

Public servers are validation targets only. Keep host notes, temporary SSH
keys, and generated files under `.local/`, which is ignored by git. The local
machine must not open naked raw UDP/TCP data-plane sessions to foreign public
servers. The lab scripts below use local SSH only; raw data-plane traffic runs
between remote validation servers.

The current lab helper expects SSH key auth and cleans its remote `/tmp`
workspace by default:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\build-linux-amd64.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\raw-transport-smoke.ps1 `
  -HostA <remote-host-a> -HostB <remote-host-b> `
  -KeyPath .local\lab_ed25519 -Mode all
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\raw-transport-benchmark.ps1 `
  -HostA <remote-host-a> -HostB <remote-host-b> `
  -KeyPath .local\lab_ed25519 `
  -Tool auto -Duration 30 -Parallel 4 -Mode both -Traffic tcp
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\xray-embedded-smoke.ps1 `
  -HostName <public-host-a>,<public-host-b> -KeyPath .local\lab_ed25519
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\xray-frame-tun-smoke.ps1 `
  -HostA <public-host-a> -HostB <public-host-b> -KeyPath .local\lab_ed25519 `
  -Mode both -Runtime both -Throughput
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\xray-wrapped-raw-tcp-smoke.ps1 `
  -HostA <public-host-a> -HostB <public-host-b> -KeyPath .local\lab_ed25519
```

In the July 2026 two-host public run, Raw UDP/TUN confirmed an outer path MTU
of 1548 and Raw UDP/TAP confirmed 1562. Both kept the device MTU at 1500,
selected a single-datagram network MTU of 1500, installed IPv4/IPv6 MSS values
of 1460/1440, and reported zero guard or I/O drops. Raw TCP/TUN reported equal
10-packet, 808-byte counters in both directions. Raw TCP/TLS and Raw UDP/DTLS
also passed bidirectionally for both TUN and TAP at MTU 1500.

Do not commit server credentials, generated keys, or lab host notes.

## Xray TAP/TUN data path

- Embedded Xray uses the unmodified official `github.com/xtls/xray-core` module
  in the TapX process. TAP/TUN frames enter and leave Xray through its native
  `transport.Link`/MultiBuffer interfaces; there is no loopback TCP proxy,
  subprocess, per-packet cgo call, JSON parse, DB read, or shell call.
- The embedded send path reads a frame directly into an official pooled Xray
  buffer. It drains up to 32 immediately available TUN/TAP frames without
  waiting and submits the batch through one official `WriteMultiBuffer` call.
  The receive path consumes official pooled MultiBuffers directly when a frame
  is contiguous and uses caller-owned storage only when stream segmentation
  splits a frame. TLS/external bridge paths reuse fixed frame buffers, so
  steady-state framing performs no per-frame heap allocation.
- External Xray necessarily crosses a process boundary. TapX generates a
  random loopback-only frame bridge, supervises the official external binary,
  waits for its local inbound without creating probe traffic, and starts the
  connector bridge only after Xray is ready. The bridge is reported as
  `xray/external`, never as a Raw TCP endpoint.
- `TRANSPORT=udp|xray|xray-external scripts/integration/tap-l2-transparent-netns.sh`
  runs the same complete TAP matrix for all three paths: PPPoE discovery and
  session traffic, MAC learning, broadcast, multicast, unknown unicast, VLAN,
  QinQ, a full 1522-byte QinQ/PPPoE frame over a 1280-byte underlay, STP, LACP,
  802.1X, and LLDP.
- July 2026 current-build public-lab payload measurements on the two small
  validation VMs are regression evidence, not theoretical maxima. VLESS/TCP
  measured about 92.8/105.7 Mbit/s for embedded TUN, 108.4/113.3 Mbit/s for
  embedded TAP, 42.4/38.4 Mbit/s for external TUN, and 36.8/33.1 Mbit/s for
  external TAP. All four cases had bidirectional reachability and zero packet
  loss. External mode necessarily pays a process and loopback bridge cost.
- The 1500-byte reusable frame parser benchmark measured 52-68 ns/op,
  0 B/op, and 0 allocations/op on an i7-9700F. The allocating reference path
  measured 785-936 ns/op, 1538 B/op, and 2 allocations/op.
- A current-binary 32-case public matrix passes embedded and TapX-managed
  external Xray across TUN and TAP for VLESS/TCP, VMess/WebSocket,
  Trojan/gRPC/TLS, Shadowsocks/TCP, Hysteria, VLESS/mKCP,
  VLESS/HTTPUpgrade, and VLESS/XHTTP. Every case verifies bidirectional inner
  IPv4 and IPv6 plus DF large packets. External cases also verify that the
  official Xray binary is supervised as a TapX child process.
- Authenticated-user routing is also verified independently of the transport
  matrix. Embedded and managed external Xray both pass bind-device and drop
  actions with separate and shared devices. The test also rejects spoofed
  source addresses, traffic outside the user's address limit, and access to a
  fallback device that was not assigned to that user.
- Stream-based Xray transports use kernel PMTUD and segmentation offload; TapX
  does not derive their outer TCP MSS from the inner device MTU. The link test
  integrity-checks a full device-sized logical frame over a dedicated Xray
  control stream, so TAP links remain testable without assigning an IP.
- Automatic mKCP MTU is capped at 1232 bytes, the maximum UDP payload that fits
  the IPv6 minimum path MTU (1280 - 40 byte IPv6 header - 8 byte UDP header).
  A netns test carries 1400-byte inner DF packets over an outer IPv6 link fixed
  at MTU 1280 and confirms that IPv6 fragment/reassembly counters do not move.
- User upload/download limits are measured on delivered bytes and pass over
  Raw TCP, embedded Xray, and managed external Xray. A 4/8 Mbit/s profile
  measured approximately 3.87/7.60, 3.68/7.60, and 3.87/7.60 Mbit/s
  respectively in netns tests.
