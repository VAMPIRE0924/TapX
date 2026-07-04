# Requirements Map

Source: `TapX-UI_Requirements and Feature Specification`, version v1.0, dated 2026-07-03.

The binary source document is intentionally not committed by default. This map turns the implementation contract into reviewable source-control text.

## Product Boundary

- TapX is a TAP/TUN networking system with a panel, not a proxy sales panel.
- The panel must be advanced like 3x-ui and expose all meaningful parameters instead of simplified mode presets.
- The 3x-ui behavior to preserve is flexible object composition: users can freely combine objects and parameters, while the backend only rejects logically invalid combinations.
- Linux complex configuration belongs in the Web panel.
- OpenWrt configuration belongs in LuCI/UCI/procd.
- Subscriptions, source IP limits, policy groups, rulesets, Telegram, and commercial operation features are excluded.

## Advanced Parameter Surface

The UI/API must allow tuning parameters directly, including but not limited to:

- Device: TAP/TUN, ifname, MTU, MSS clamp, IPv4/IPv6, gateway, DNS, static routes, default route, bridge name, bridge member, bridge MTU.
- Raw UDP: bind/listen/connect address, port, fixed peer or peer behavior, optional vKey, optional Client/Route binding, optional allowed TAP/TUN IP/MAC limits, bind interface/address, socket buffers, reuse flags, keepalive, worker count, queue settings.
- Raw TCP: bind/listen/connect address, port, length field mode, optional vKey, optional Client/Route binding, optional allowed TAP/TUN IP/MAC limits, TCP_NODELAY, keepalive, Fast Open, socket buffers, connect timeout, reconnect interval, worker/buffer settings.
- Xray: embedded/external runtime, streamSettings, network, security, TLS, REALITY, XTLS/Vision, sockopt, mux, XUDP, sniffing, fallbacks, advanced JSON.
- Runtime/ops: service state, logs, statistics, backup/restore, diagnostics, Panel HTTPS, external binary paths.

These parameters are control-plane settings. The backend validates logical consistency when saving and when applying changes. Applying them must generate runtime config for core workers. Unset features must add no checks; configured features must become compact runtime flags or lookup tables, not per-packet DB/API lookups.

## Required Components

- `tapx-panel`: Web UI, API, DB, login, config, logs, stats, backup.
- `tapx-core`: TAP/TUN adapter, raw UDP/TCP, embedded Xray, external Xray manager, data-plane stats.
- `tapx-net`: TAP/TUN, bridge, IP, route, DNS, MTU, MSS apply and rollback.
- `luci-app-tapx`: OpenWrt LuCI management UI.

TAP/TUN devices are real Linux kernel network interfaces, not hidden in-process
pipes. While a runtime pipe is active, `ip a` and `ip -d addr show dev <ifname>`
must show the configured interface so operators can attach nftables, routing,
bridge, and port-mapping rules to that interface. Temporary lab devices may be
non-persistent and disappear after the owning process stops.

Raw transport security knobs are explicit advanced fields, not presets.
`RawTCP.TLS` and `RawUDP.DTLS` expose enable flags, cert/key/CA paths, SNI,
ALPN, version bounds, allow-insecure switches, and DTLS MTU/replay-window
settings for Listener/Connector objects. Save/apply validation accepts
logically valid values. Enabled RawTCP TLS uses a dedicated Go TLS frame
runtime, and enabled RawUDP DTLS uses a dedicated Go DTLS datagram runtime.
Neither path alters the naked Raw TCP/UDP C fastpaths; raw UDP/TCP
no-auth/no-encryption remains first-class and unaffected.

## Required UI Menus

- Dashboard.
- Devices.
- Listeners.
- Connectors.
- Clients.
- Routes.
- Xray Profiles.
- vKeys.
- Logs.
- Settings.
- Backup.

There is no top-level Certificates page. Transport certificates live inside Listener/Connector security sections. Panel HTTPS lives in Settings.

## Core Objects

- Device: TAP/TUN, interface, MTU, MSS, IP, gateway, DNS, routes, TAP bridge.
- Listener: inbound-style local entry bound to transport and route.
- Connector: outbound-style remote connection bound to device/route.
- Client: identity, credential, expiration, traffic cap, sharing, fixed TAP/TUN addresses.
- Route: direct binding only.
- XrayProfile: runtime mode, transport stream/security settings, sniffing, mux, sockopt, fallbacks, and advanced JSON.
- Settings: panel/runtime paths, panel login, HTTPS settings, log level, stats interval, backup/data directories, and current build target.

These objects are not rigid modes. A raw Listener can be left as a pure transport pipe, or it can reference vKey, Client, Route, Device, Connector, and AddressLimit settings. The same applies to Connector and Client objects. Empty references mean no behavior; filled references become generated runtime behavior.

Client objects now carry optional `CredentialType` and `CredentialValue`
fields. `Binding.ConnectorID` is also available so a Client or endpoint can bind
directly to a Connector or inherit it from a Route. AddressLimit carries both
Guard limits and fixed-client static configuration: MACs, IPv4/IPv6 CIDRs,
IPv4/IPv6 gateways, DNS servers, pushed routes, and default-route permission.
The share API resolves the configured Client, Listener, Connector, Route, vKey,
Device, and AddressLimit combination before generating import data.

## Optional vKeys, Routes, and Address Limits

vKey, Route binding, Client binding, and allowed TAP/TUN IP/MAC limits are optional and composable. There is no separate built-in tag classifier for raw traffic; vKey covers the binding/admission role when configured.

If none are configured, raw UDP/TCP forwards with the minimum required transport work. If any are configured, the data plane must enforce the generated runtime behavior.

When allowed TUN addresses are configured, TUN must check packet source IP against the assigned addresses.

When allowed TAP identities are configured, TAP must check:

- Ethernet source MAC.
- ARP sender IP/MAC.
- IPv4 source address.
- IPv6 source address.
- IPv6 ND source/link-layer data.

Unauthorized traffic must be dropped and counted.
AddressLimit may also carry client import/static configuration such as gateway,
DNS, pushed routes, and default-route permission. Those fields are shared to the
client but do not add packet-time Guard checks.

Stats are aggregated in the Go control plane from C counter snapshots. The data
plane may increment counters, but it must not read SQLite, parse JSON, write
logs, or call API endpoints to produce stats.

Traffic expiration/cap enforcement is driven by the control plane from those
counter snapshots. Disabled, expired, or over-quota Client-bound pipes can be
closed without adding DB/API/JSON work to the per-packet path.
Client traffic reset stores reset timestamps and RX/TX counter offsets on the
Client object; used-traffic and quota views subtract those offsets while the C
fastpath counters remain raw monotonic counters.

Device MTU, MSS clamp, IPv4/IPv6 CIDR, TAP bridge/member settings, static
routes, and DNS resolv.conf output are applied by the Linux control plane after
TUN/TAP creation and before workers start. This is the first tapx-net slice.

## Raw Transport

Raw UDP and Raw TCP are required fast paths and do not require encryption or authentication. They can also be combined with optional vKey, Client binding, Route binding, and allowed address controls.

Panel login is a control-plane feature and must not be confused with raw
transport auth. Enabling panel login protects Web/API management endpoints only;
it must not make raw UDP/TCP vKey or encryption mandatory.

Raw UDP:

```text
UDP payload = TAP Ethernet frame / TUN IP packet
```

Raw TCP:

```text
uint16_be length + TAP Ethernet frame / TUN IP packet
```

`uint32_be length` is an optional advanced mode.

vKey is optional for raw UDP/TCP. It must not become mandatory and must not become expensive per-packet auth unless a future mode explicitly asks for that tradeoff. Xray mode does not show TapX vKey.

When configured, vKey is carried as a compact TapX wire marker:

```text
"TXV1" + uint16_be key_len + 2 reserved bytes + key + frame
```

For UDP, the marker prefixes the UDP payload. For TCP, the length field covers
the marker plus the TAP/TUN frame. This is a lightweight admission marker, not
encryption and not cryptographic authentication. Empty vKey keeps UDP no-header
raw mode and TCP length-prefix-only raw mode.

## Xray Runtime

- Embedded Xray is the default long-term runtime target.
- External Xray is compatibility mode from Settings.
- Raw UDP/TCP must not pass through Xray.
- Same-process embedded Xray transport is implemented after raw UDP/TCP fast paths.
- Embedded Xray must not use local redirect or expose visible internal ports.

Current implementation status: external Xray compatibility is wired into the
Go supervisor. XrayProfile fields compile into an external Xray JSON document,
Settings.ExternalXrayPath selects the binary, and runtime state exposes the
managed external process. The Web/API control plane can also inspect, stream
upload, or HTTP(S) download that external binary to Settings.ExternalXrayPath
or an explicit operator-provided path. Embedded same-process Xray now compiles
XrayProfile objects into an in-memory JSON document and starts xray-core
through its Go API, so embedded endpoints run inside `tapx-core` without an
external process.
Embedded frame/TUN/TAP transport is also wired for bound Xray endpoints:
listener inbounds are routed to a TapX custom outbound handler, connector dials
force the configured outbound tag, and both sides keep real Linux TUN/TAP
netdevs visible to `ip a`/`ip -d addr`.

## Performance Contract

Forbidden in the hot path:

- per-packet memory allocation,
- per-packet goroutine,
- per-packet log write,
- per-packet DB lookup,
- per-packet JSON parsing,
- per-packet shell call,
- per-packet cgo boundary.

## Delivery Contract

- Linux installer.
- systemd service.
- OpenWrt `tapx-core.ipk`, with current development validation on x86-64 first.
- OpenWrt `luci-app-tapx.ipk`, with current development validation on x86-64 first.
- Non-x86 OpenWrt architectures, including MT7986 when explicitly needed, are follow-up packaging targets.
- Documentation for install, config, API, OpenWrt, troubleshooting, and development.

Current delivery status: Linux amd64 binary build, systemd unit, root installer,
and `docs/install-linux.md` exist. The Linux installer initializes an enabled
panel admin account, random password, and random Web base path on first install,
then prints the login information. Panel HTTPS settings are honored on startup
when cert/key paths are configured. The panel also has an initial Client
sharing API/UI: raw clients generate compressed `tapx://client/gzip/...` import
links, and VLESS/Xray clients generate `vless://` links with QR PNG output.
The Dashboard API/UI now surfaces runtime/core status, Xray runtime state,
counter totals, rate estimates, error/drop counts, fastpath/process diagnostics,
object counts, and recent logs.
Client traffic reset is also wired through API/UI, closing the required
used-traffic/reset part of Client traffic-package management.
OpenWrt
currently has an x86-64 SDK binary
build target, initial `tapx-core.ipk` packaging, UCI defaults, procd service,
and initial `luci-app-tapx.ipk` packaging. The LuCI slice can edit the UCI
service switch/runtime path, expose field-level object templates for Device,
Listener, Connector, Client, Route, vKey, AddressLimit, XrayProfile, and
Settings, append/replace those objects in runtime JSON, run `tapx-core -check`,
show service status, and reload the service. Non-x86 package validation and
full architecture packaging remain follow-up work.
