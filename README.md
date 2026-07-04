# TapX

TapX is a high-performance TAP/TUN networking system with an advanced Web control panel. It follows the operational shape of 3x-ui for objects such as listeners, connectors, clients, traffic limits, logs, and sharing, but its core is not a proxy user panel. The core is TAP/TUN device management and raw packet/frame forwarding.

The panel must expose advanced parameters rather than hiding them behind simplified modes. Users should be able to tune TAP/TUN, bridge, MTU/MSS, socket buffers, TCP options, UDP behavior, routes, DNS, Xray stream/security settings, runtime paths, logs, backup, and statistics from the Web UI/API. Those settings are validated, turned into runtime config, and applied before traffic enters the fast path.

TapX borrows 3x-ui's high-plasticity management logic: objects are not locked into one fixed mode. Operators can freely combine Listener, Connector, Client, Route, vKey, Device, and AddressLimit settings. The backend validates whether the combination is logical; it should not force a simplified workflow.

The main data path is designed for maximum throughput:

- Raw UDP: payload is the TAP Ethernet frame or TUN IP packet.
- Raw TCP: length-prefixed TAP Ethernet frame or TUN IP packet.
- Raw UDP/TCP can run with no vKey, no authentication, no encryption, no Client binding, no address limit, and no route lookup.
- The same raw transports can also be combined with vKey, Client binding, Route binding, and allowed TAP/TUN IP/MAC limits when the operator configures them.
- Empty config means no extra work in the hot path. Configured features are prepared into the runtime path.
- Configured vKey uses a lightweight `TXV1` wire marker; unset vKey keeps bare raw UDP and length-prefix-only raw TCP.
- Xray is added after the raw fast path and runs in the same TapX core process when embedded.
- Go owns the control plane.
- C owns the hot data path where raw forwarding needs maximum performance.

## Repository Layout

```text
cmd/
  tapx-panel/      Web/API/control-plane service entry.
  tapx-core/       Local core supervisor entry.
core/
  fastpath/        C raw UDP/TCP/TAP/TUN fastpath ABI and implementation.
internal/
  config/          Runtime config generated from DB/API objects.
  model/           Product object model.
  panel/           SQLite object store and HTTP API.
docs/
  architecture.md  Engineering architecture.
  install-linux.md Linux build and systemd install guide.
  openwrt.md       OpenWrt x86-64 package and LuCI guide.
  panel-api.md     Current control-plane API.
  release.md       GitHub Actions and release artifact flow.
  verification.md  Local, Linux, and public lab verification flow.
  decisions.md     Requirement decisions and conflict handling.
  raw-udp-tun-progress.md
  roadmap.md       Phased implementation plan.
scripts/
  dev/             Local development environment checks.
  lab/             Public-server validation helpers.
web/               Future Web UI.
openwrt/           Future OpenWrt package and LuCI work.
tests/             Integration and network tests.
```

## Current Status

This repository has the first raw fast path and the initial control-plane API in place. Current implemented pieces are:

1. TUN device open/read/write.
2. TAP device open/read/write.
3. First Linux tapx-net apply layer for Device MTU, MSS clamp, IPv4/IPv6 address assignment, interface up, TAP bridge creation/member binding, static route apply, DNS resolv.conf output, and rollback.
4. Raw UDP no-header forwarding with optional generated route/address checks.
5. Raw TCP length-prefixed forwarding.
6. Raw UDP socket bind address/interface, receive/send buffers, and reuse flags applied from generated runtime config.
7. Raw TCP bind address/interface, receive/send buffers, TCP_NODELAY, keepalive, connect timeout, and TCP Fast Open applied from generated runtime config.
8. C fastpath counters and guard-drop counters.
9. Go control process that applies runtime config without entering the per-packet path.
10. SQLite-backed flexible object store for Device, Listener, Connector, Client, Route, vKey, AddressLimit, XrayProfile, and Settings objects.
11. HTTP API for config read/write, object CRUD, save/apply validation, and runtime generation.
12. HTTP API for local runtime apply, stop, state, and fastpath counters.
13. Embedded Web UI for object CRUD, dense field editing, full JSON editing, dashboard rates/recent logs, runtime apply/stop/state, logs, backup/restore, diagnostics, and counters.
14. First-class Xray Profiles and Settings API/UI surfaces for later runtime integration without blocking raw fastpath work.
15. Runtime apply rollback-on-start-failure plus conservative prepare-first reload when old/new resources are disjoint.
16. Optional Settings-driven panel login using PBKDF2 password hashes and HTTP-only sessions, with Settings-driven HTTPS startup when cert/key paths are configured.
17. Aggregated stats API/UI for total, endpoint, device, route, client, and quota/expiration views, including Client traffic reset offsets.
18. Runtime client-limit enforcement that closes pipes bound to disabled, expired, or over-quota Clients without adding packet-time DB work.
19. External Xray compatibility runtime: the core compiles Xray JSON from XrayProfile/Listener/Connector/Settings objects, starts the configured external `xray` binary, exposes Xray runtime state, and stops/cleans generated config on runtime stop.
20. Linux x86-64 build script, systemd unit, root-only installer script, randomized first-install panel path/admin initialization, and Linux install documentation.
21. OpenWrt x86-64 `.ipk` packaging for `tapx-core`, default UCI config, procd service, and initial `luci-app-tapx` package with object field templates plus full runtime JSON editing.
22. Raw UDP/TUN and Raw TCP/TUN pair template generation through API/UI, producing editable side-A/side-B configs without saving automatically.
23. Same-process embedded Xray core runtime: embedded Xray endpoints start inside `tapx-core` through the xray-core Go API and expose runtime state without launching an external binary or hidden local redirect.
24. Embedded Xray frame/TUN/TAP adapter: a bound Xray listener routes inbound streams to an in-process TapX frame handler, and a bound Xray connector dials its outbound tag directly through xray-core. The local interface is still a real Linux TUN/TAP netdev visible to `ip a`; only the underlay transport changes.
25. Initial Client sharing API/UI: Client credentials, direct Connector binding,
    compressed `tapx://client/gzip/...` raw import links, VLESS `vless://`
    links, QR PNG output, fixed-address/gateway/DNS/route payload data, and
    structured payload preview.
26. External Xray binary management API/UI: status check, streamed upload, and
    operator-supplied HTTP(S) download to `Settings.ExternalXrayPath` or an
    explicit path.
27. Dashboard API/UI expansion: runtime/core state, Xray runtime state,
    real-time rate estimates, traffic counters, error/drop counts, object
    counts, fastpath/process diagnostics, and recent logs.
28. Client traffic reset API/UI: stores reset timestamps and RX/TX offsets on
    Client objects so used traffic and quota enforcement can restart from the
    operator-selected reset point without touching C counters.
29. Raw TCP TLS and Raw UDP DTLS runtime paths: `RawTCP.TLS` and
    `RawUDP.DTLS` expose cert/key/CA, SNI, ALPN, version, allow-insecure, and
    DTLS MTU/replay-window fields in Linux Web, LuCI, API, and share payloads.
    When enabled, Raw TCP uses a separate Go TLS frame runtime and Raw UDP uses
    a separate Go DTLS datagram runtime, both preserving vKey marker semantics.
    Naked Raw TCP/UDP remain on the C fastpaths.

## Development

Preferred local development environment is WSL Ubuntu 24.04 because the target behavior depends on Linux TAP/TUN, epoll, sockets, routing, and bridge semantics.

```bash
./scripts/dev/check-env.sh
make test
make verify-local
make build-linux-amd64
make integration-netns
make build-openwrt-x86
make package-openwrt-x86
make verify-release
```

`make integration-netns` is optional and requires root or CAP_NET_ADMIN. It
creates temporary `tapx-it-*` network namespaces, starts local `tapx-core`
processes, and verifies raw UDP/TUN IP-packet forwarding, raw UDP/TAP
Ethernet-frame forwarding, raw UDP/TUN with vKey marker, raw TCP/TUN
length-prefix forwarding with bidirectional ping, and Address Guard allow/drop
behavior for guarded TUN/TAP paths. It also verifies Device MTU/IP apply, MSS
clamp rule apply/rollback, DNS resolv.conf output/rollback, TAP bridge
creation/member binding, and static route apply in network namespaces. Use
`make integration-device-apply-netns`, `make integration-bridge-apply-netns`,
`make integration-mss-clamp-netns`, `make integration-dns-apply-netns`,
`make integration-tun-netns`,
`make integration-tun-vkey-netns`,
`make integration-tap-netns`, or `make integration-tcp-tun-netns` to run one
path. Use `make integration-address-guard-netns` to run only the guarded TUN/TAP
allow/drop test.

`make build-openwrt-x86` builds `tapx-core` with the OpenWrt x86-64 SDK into
`build/openwrt-x86-64/tapx-core`. MT7986 and other architectures are not part of
the current development gate; add them later when that platform work starts.
`make package-openwrt-x86` creates the current x86-64 `tapx-core.ipk` and
`luci-app-tapx.ipk` under `build/openwrt-x86-64/packages/`.

`make verify-local` runs a pure-Go repository verifier for JSON validity,
runtime config generation, required packaging files, generated OpenWrt package
structure when packages are present, and secret-marker checks. `make
verify-release` requires the OpenWrt `.ipk` files to exist and verifies their
control/data contents.

Run the local panel API:

```bash
go run ./cmd/tapx-panel -db .local/tapx.db -listen 127.0.0.1:8080
open http://127.0.0.1:8080/
curl http://127.0.0.1:8080/api/health
curl -X POST http://127.0.0.1:8080/api/runtime/apply
curl http://127.0.0.1:8080/api/runtime/state
curl -X POST http://127.0.0.1:8080/api/runtime/enforce
curl http://127.0.0.1:8080/api/logs
curl http://127.0.0.1:8080/api/stats
curl http://127.0.0.1:8080/api/share/clients/client-a
curl http://127.0.0.1:8080/api/backup
curl http://127.0.0.1:8080/api/diagnostics
```

Validate an external-Xray profile without starting it:

```bash
go run ./cmd/tapx-core -config docs/examples/xray-external-listener.json -check
```

Generate a panel password hash when enabling Settings-based panel login:

```bash
printf 'change-me' | go run ./cmd/tapx-panel -hash-password-stdin
```

Build and install the Linux service locally:

```bash
make build-linux-amd64
sudo ./scripts/install/linux-install.sh
```

On first install the script prints the generated panel URL, admin username, and
random password. The URL includes a random base path stored in
`/etc/tapx/tapx.env`.

Public servers used for validation must stay outside the committed source. Put
host-specific notes under `.local/`. Lab scripts under `scripts/lab/` use local
SSH only; the local machine must not open naked raw UDP/TCP data-plane sessions
to foreign public servers. Remote server-to-server raw smoke/benchmark is valid,
and TLS/Xray-wrapped validation is available when the protected outer transport
needs to be tested. Scripts clean remote `/tmp/tapx-lab-*` directories by
default.

The public repository target is `VAMPIRE0924/TapX`. The first release should
stay clean and publish only Linux amd64 binaries plus OpenWrt x86-64 IPK
artifacts.
