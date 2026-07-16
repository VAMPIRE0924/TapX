# Architecture

TapX is split into control plane, network apply layer, and data plane.

```text
tapx-panel
  Web UI / API / SQLite or PostgreSQL / login / logs / backup / stats display
        |
        | local control API
        v
tapx-core-go
  lifecycle / runtime config generator / worker supervision / Xray manager
        |
        | control fd / shared memory / immutable runtime tables
        v
tapx-fastpath-c
  TAP/TUN fd / raw UDP / raw TCP / generated optional checks / counters
```

## Control Plane

Go owns:

- Web/API.
- SQLite/PostgreSQL storage and migrations.
- Device, Listener, Connector, Client, Route, XrayProfile, vKey, Settings.
- Config validation.
- Runtime config generation.
- Service lifecycle.
- Periodic counter collection.
- Aggregated stats and client traffic/quota views.
- Backup and restore.
- Linux/OpenWrt network apply orchestration.

Go must not enter the packet hot path.

The panel backend uses the same flexible object model on SQLite or PostgreSQL:

```text
tapx_objects(kind, id, payload, updated_at)
```

Each Device, Listener, Connector, Client, Route, vKey, AddressLimit,
XrayProfile, and Settings object is stored as a full JSON object. This keeps
the backend friendly to an advanced panel: adding low-level knobs does not
require rewriting the storage schema for every field. Save/apply validation
still unmarshals the payload into typed Go models before anything can become
runtime config.

The panel API also owns the first runtime lifecycle hooks:

- generate runtime from stored objects,
- apply runtime to the local Go supervisor,
- stop the local runtime,
- expose running pipe state and counters.

Current apply semantics are replace-style with rollback-on-start-failure: stop
the old local runtime, start the new generated runtime, and if that replacement
start fails, attempt to restart the last successfully applied runtime. This does
not pretend to be a no-interruption reload; true no-interruption switching is
later control-plane work and depends on whether the affected resources can be
prepared in parallel.

The embedded UI keeps both sides of the advanced-panel workflow: dense field
controls for core objects and a full JSON editor for fields that are not yet
promoted into dedicated controls. Field edits update the object JSON before
save, and backend validation remains the source of truth for logical
consistency.

The first ops surface is also in the panel: optional Settings-driven panel
login, portable `.db` backup export/restore, and a persistent operation log for
control-plane actions such as config save, object changes, runtime apply/stop,
auth login/logout, and backup restore. This log is not part of the packet path.

Panel login uses PBKDF2-SHA256 password hashes and in-memory HTTP-only session
cookies. It protects panel APIs only. Raw UDP/TCP no-auth modes remain valid
data-plane features and do not depend on panel sessions.

Diagnostics are read-only and generated from process/runtime state: version,
Go/OS details, memory counters, fastpath ABI, current x86-64 OpenWrt build
target, object counts, and runtime state. They do not execute shell commands or
inspect lab-specific files.

Stats are also read-only control-plane snapshots. Go reads C counter snapshots
from active pipes and aggregates them by transport, endpoint, device, route, and
client. Client traffic-cap and expiration status are derived from stored objects
plus those counter snapshots; no packet performs a DB lookup.

Client-limit enforcement is also control-plane driven. After runtime apply, the
manager keeps a config snapshot and periodically evaluates disabled, expired, or
over-quota Clients from current counter snapshots. Matching Client-bound pipes
are closed by the supervisor; the C fastpath still only updates counters and
never reads a control-plane database, parses JSON, or calls panel APIs per packet.

The first Xray runtime slices are external compatibility mode and embedded
xray-core mode. External mode compiles XrayProfile, Listener, Connector, and
Settings objects into one Xray JSON document, writes it outside the hot path,
starts the configured external Xray binary, exposes the process in runtime
state, and removes the generated config when stopped. Embedded mode compiles a
separate Xray document and starts xray-core through its Go API in the TapX
process. It does not launch an external binary or use a hidden local redirect.
For bound Xray endpoints, TapX also opens the configured real TUN/TAP device and
passes TUN packets or TAP Ethernet frames through xray-core by using a custom
in-process outbound handler on listener side and a forced outbound tag on
connector side.

## Advanced Panel Contract

The Web panel is a full advanced operator interface, similar in depth to 3x-ui. It must expose all meaningful parameters for:

- TAP/TUN device type, interface name, MTU, MSS clamp, IP, route, DNS, gateway, and bridge settings.
- Raw UDP listen/connect address, port, peer mode, socket buffers, reuse flags, keepalive behavior, bind interface, and worker settings.
- Raw TCP listen/connect address, port, length mode, TCP_NODELAY, keepalive, Fast Open, socket buffers, reconnect timers, bind interface, and worker settings.
- Xray runtime, stream settings, security settings, sockopt, mux, sniffing, fallback, and advanced JSON where needed.
- Logs, statistics, backup, service state, and diagnostics.

The panel can expose many knobs without adding per-packet overhead. The control
plane validates these settings and generates immutable runtime structures before
C workers or embedded Xray transports use them. Runtime apply uses a
prepare-first path when old and new runtimes have disjoint interface, bridge,
and listener-port resources; conflicting resources keep the stop-first rollback
path.

## Network Apply

The first Linux tapx-net apply layer runs after a TUN/TAP fd is created and
before the C worker starts. It applies Device MTU, MSS clamp, IPv4/IPv6 CIDR
assignment, interface up state, TAP bridge creation/member binding, static
routes, and DNS resolv.conf output through control-plane file and
`ip`/iptables-nft operations, then records applied state for rollback when the pipe
closes or startup fails. This is outside the packet path. A future
netlink-backed implementation remains later work.

The TUN/TAP fd is backed by a real kernel netdev with the configured interface
name. It must remain visible in `ip a` during runtime so nftables, Linux routes,
bridges, and port mappings can target the interface directly. Optimizations may
remove packet hot-path overhead, but must not replace the operator-visible
netdev contract with a hidden user-space pipe.

Raw TCP TLS and Raw UDP DTLS are modeled as optional Listener/Connector security
objects. Disabled security objects must be zero-cost for the raw fastpath.
Enabled Raw TCP TLS runs through a separate Go TLS frame path with the same
length-prefix and vKey marker semantics, and must not silently fall through to
the naked C worker. Enabled Raw UDP DTLS runs through a separate Go DTLS
datagram path with the same vKey marker semantics, and must not silently fall
through to the naked C worker.

TapX should copy the flexible composition logic that makes 3x-ui powerful: users can assemble objects in different ways instead of following a fixed wizard. Listener, Connector, Client, Route, vKey, Device, AddressLimit, XrayProfile, and Settings are independent configurable objects that can reference each other where it makes sense.

## Fast Path

C owns the raw forwarding hot path:

- TAP/TUN fd read/write.
- Raw UDP send/recv.
- Raw TCP framing.
- epoll or io_uring workers.
- Buffer pools.
- Optional generated checks for vKey, Client, Route, and address limits.
- Per-worker counters.

The fast path must avoid:

- per-packet malloc,
- per-packet goroutine,
- per-packet cgo,
- per-packet DB lookup,
- per-packet JSON parsing,
- per-packet logging,
- per-packet shell calls.

## Composable Runtime Features

TapX is an advanced panel, so transport features are not fixed presets. Raw UDP/TCP can be used with nothing extra configured, or with optional controls layered in by configuration:

- vKey for lightweight admission,
- Client identity and traffic accounting,
- Route binding to choose usable TAP/TUN devices or connectors,
- allowed TAP/TUN IP/MAC limits,
- TLS/DTLS or Xray security where the selected transport supports it.

The important rule is apply-time composition: the UI/API stores flexible settings, Go validates them, then the runtime receives a compact configuration containing only the enabled features. Unset features generate no packet-time checks.

Save/apply validation is about logical consistency only. The backend should reject broken references and impossible combinations, but it should not force optional vKey, Client, Route, Device, Connector, or AddressLimit bindings.

## Raw UDP

The minimum raw form remains intentionally bare:

```text
UDP payload = TAP Ethernet frame
UDP payload = TUN IP packet
```

No encryption, no authentication, no vKey, and no address limit are valid when the operator leaves those fields empty. If vKey, route binding, or allowed IP/MAC settings are configured, they become generated checks or lookup tables in the worker.

Raw UDP socket-level settings are control-plane options. `BindAddress` can
override the endpoint bind host, `BindInterface` maps to Linux device binding,
and receive/send buffers plus reuse flags are applied before `bind(2)`. Empty
values leave the socket at the lean default.

When `VKeyValue` is generated for a raw UDP pipe, only that pipe switches to a
compact vKey wire marker:

```text
UDP payload = "TXV1" + uint16_be key_len + 2 reserved bytes + key + TAP/TUN frame
```

This is a lightweight admission marker for operator-controlled fabrics, not
encryption and not a cryptographic authentication scheme. Empty vKey keeps the
original no-header raw UDP format.

## Raw TCP

TCP is a byte stream, so raw TCP needs frame boundaries:

```text
uint16_be length + TAP Ethernet frame
uint16_be length + TUN IP packet
```

`uint32_be length` can be added as an advanced mode. Raw TCP can run as a pure framed stream, or with configured vKey/Client/Route/address-limit controls.

Raw TCP socket-level settings are also control-plane options. `BindAddress` can
override the listener bind host or choose the connector local address,
`BindInterface` maps to Linux device binding, receive/send buffers are applied
to the raw socket and connection, and TCP_NODELAY, keepalive, connect timeout,
and TCP Fast Open are applied before the fd is handed to the C worker.

When `VKeyValue` is generated for a raw TCP pipe, the TCP length field covers
the vKey marker plus the TAP/TUN frame:

```text
uint16_be length + "TXV1" + uint16_be key_len + 2 reserved bytes + key + frame
```

`uint32_be length` works the same way. Empty vKey keeps the original length
prefix plus frame format.

## Xray Runtime

Xray comes after the raw UDP/TCP fast path is measurable. It is a transport/runtime option inside the same TapX core process:

- embedded runtime is the long-term default target,
- external runtime is compatibility mode,
- raw UDP/TCP must remain independent from Xray.

External compatibility is now a managed process path. The panel can inspect,
upload, or download the external xray-core binary selected by Settings without
shelling out. Same-process embedded support now starts xray-core directly and
includes a frame/TUN/TAP adapter for bound Xray endpoints. Listener inbounds can
route to a TapX custom outbound handler in the same xray-core instance;
connectors dial through xray-core with a forced outbound tag. No hidden local
redirect port is used, and the local packet interface remains the real TUN/TAP
netdev created by TapX.
