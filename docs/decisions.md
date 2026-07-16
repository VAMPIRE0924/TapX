# Requirement Decisions

These decisions resolve implementation ambiguity while preserving the requirement document.

## Raw no-auth modes are required

Raw UDP and Raw TCP without encryption or authentication are first-class TapX modes. They must not be removed or downgraded to unsafe-only experiments.

Raw UDP/TCP can be completely bare when the operator configures no vKey, no Client, no Route binding, and no allowed IP/MAC limits. This is useful for trusted fabrics such as private LAN, leased line, or an outer tunnel that already provides security.

The same raw UDP/TCP transport can also be combined with vKey, Client binding, Route binding, traffic accounting, and allowed TAP/TUN IP/MAC limits. These are optional features, not separate products. `tag` is not a separate built-in traffic classifier; vKey is the raw binding/admission key when that layer is configured.

When vKey is configured for raw UDP/TCP, TapX uses a compact `TXV1` wire marker
to admit matching traffic. This marker is not encryption and not cryptographic
authentication. It exists so advanced operators can compose a lightweight key
with routes/clients/address limits while keeping the bare no-header/no-auth raw
mode available when vKey is unset.

## Advanced panel is required

TapX must be an advanced Web panel like 3x-ui, not a simplified script wrapper. Every meaningful transport, socket, device, bridge, route, DNS, TLS/DTLS/Xray, runtime, logging, statistics, and backup parameter should be configurable from UI/API.

The part borrowed from 3x-ui is the flexible object-composition logic. Users should be able to combine Listener, Connector, Client, Route, vKey, Device, and AddressLimit settings in many valid ways. The backend checks whether the combination is logically valid; it does not reduce the product to fixed presets.

This does not conflict with raw performance. UI/API settings are control-plane inputs. They are validated, turned into runtime config, and applied before runtime. The packet hot path uses prepared endpoints, enabled feature flags, and compact lookup tables, not dynamic UI objects.

Backend validation should only reject logically invalid combinations, such as referencing a disabled object, binding a TUN address limit to a TAP-only identity, using a missing vKey, or creating conflicting address assignments. It must not invent defaults such as implicit Client binding, implicit Route binding, or implicit address limits.

## Durable panel data lives in the database

All durable backend panel state must be stored in the database so backup and
restore can reproduce the complete manageable TapX state.

This includes Settings, Devices, Listeners, Connectors, Clients, Routes, vKey
bindings, address limits, Xray profiles, TapX runtime templates, panel security
settings, panel-managed certificate material, kernel/runtime configuration,
compatibility-layer metadata, and any other operator-created object that affects
the generated runtime.

File paths can still point to external files when the operator deliberately uses
external assets, but panel-managed data must be exportable and restorable from
the database backup. A backup is an administrator artifact and may contain
secrets needed to restore the node; it must never be committed to the public
repository.

Compatibility endpoints for the 3x-ui-derived Web panel should be implemented
as a panel API adapter over the TapX database and runtime manager. They must not
change the raw C fast path, the embedded Xray forwarding path, or the prepared
runtime config model.

Historical chart data can be persisted for backup/restore, but it must be
written by a Go-side periodic collector from aggregated counters. The C fast
path must never perform DB writes, JSON parsing, shell calls, or log writes in
the per-packet path.

## Database backends

SQLite is the installer default and PostgreSQL is an optional control-plane
backend. Both use the same typed object validation and generate the same
immutable runtime configuration; changing the database never changes the C
fastpath or embedded/external Xray packet path.

Backup and restore intentionally keep one 3x-ui-style downloadable `.db`
format. SQLite uses its online backup API. PostgreSQL exports all TapX-owned
tables into a portable SQLite application database. Restore validates that
file and atomically replaces TapX objects, integrations, management logs, and
chart history in the active backend. This also provides supported
SQLite-to-PostgreSQL and PostgreSQL-to-SQLite migration. It is an application
backup, not a native PostgreSQL cluster backup.

## Device auto-create is a shortcut, not a separate state

When an inbound uses "auto-create by name" for a TUN/TAP interface, TapX treats
that as a creation shortcut. Saving the inbound must create or update a real
Device object, then rewrite the inbound binding to reference that existing
device. On the next edit, the inbound should show "use existing device".

The Devices menu and Inbounds menu therefore share the same device identity.
Device-level routing, DNS, bridge, MTU, address, and future nftables-related
settings live under Devices. Inbounds only choose which existing Device they
bind to, with auto-create available to avoid forcing the operator to leave the
inbound modal during first setup.

If an auto-create request uses a name that already exists with a different
interface type, validation fails. TapX must not silently convert an existing TUN
into TAP, or the reverse.

## Device address assignment is optional

Device, Listener, and Connector binding editors must not force an interface
address. Some deployments use TapX as a wide-area bridge and only need the
TUN/TAP link itself. Address fields are shown and saved only when the operator
enables interface address configuration.

When a Listener or Connector binds an existing Device, the Device remains the
owner of address, gateway, MTU, route, DNS, and bridge settings. Listener and
Connector binding payloads store only the selected interface identity. Address
assignment fields belong in the binding payload only for the "auto create by
name" workflow, where saving the Listener or Connector also creates or updates
the corresponding Device record.

When address configuration is enabled, the operator can choose automatic
assignment or manual assignment. Automatic assignment records the intent but
does not save manual IPv4, IPv6, or gateway values. Manual assignment saves the
operator-provided address fields. Legacy records or calls that already contain
IPv4, IPv6, or gateway values are treated as manual address configuration for
compatibility.

TapX raw Connectors mirror Listener composition for Raw TCP/UDP. They can run
bare for maximum performance, or optionally carry vKey plus Raw TCP TLS / Raw
UDP DTLS settings. Disabled vKey/TLS/DTLS options are omitted from the saved
wire payload so the backend does not enable extra hot-path work by accident.
TapX Raw TLS/DTLS does not expose CA-file or ALPN fields in the panel. Domain
or IP certificates should rely on the system trust chain on the connecting side,
while Listener-side certificate and private-key paths remain available because
the Listener terminates TLS/DTLS.

The Connector test toggle label is "Gateway" for the existing fast probe path
and "HTTP" for the full xray request path. The backend should eventually map
Gateway mode to a TapX-aware gateway ping when connector-bound TUN/TAP gateway
state is available.

## Current panel persistence audit

This is the current implementation state, not the final target:

- The Go panel backend has a SQLite-default/PostgreSQL-optional object store for Devices,
  Listeners, Connectors, Clients, Routes, vKeys, AddressLimits, XrayProfiles,
  and Settings.
- Backup and restore already operate on `RuntimeConfig`, so objects stored in
  that backend store can be exported and restored together.
- The 3x-ui-derived Web panel is still in a transition state. Some pages still
  use 3x-ui compatibility endpoints or frontend-only state instead of first-class
  TapX objects.
- The Devices page currently uses frontend local storage. That must be migrated
  to the backend Device object store before it can be considered DB-backed and
  included in reliable backup/restore.
- The Connector page currently edits Xray `templateSettings.outbounds`. It can
  be backed up as part of Xray JSON, but it is not yet the same thing as a
  first-class TapX Connector object. The final panel must persist Connectors as
  DB objects and expose Xray JSON only as generated/advanced configuration.
- UI-only preferences such as theme, sidebar collapsed state, and table filters
  may remain in browser storage because they do not affect generated runtime
  configuration.

## Current TapX link audit

This is the current link/export/import state, not the final target:

- Standard Xray listener links such as `vless://`, `vmess://`, `trojan://`,
  `ss://`, `hysteria://`, and WireGuard config/link output already exist in the
  frontend link generator.
- Connector link import currently parses common Xray client links into outbound
  form objects.
- Raw TapX listener export is not complete as a `raw-tcp://`, `raw-udp://`, or
  equivalent compact URL format. The backend currently has a client share path
  that can emit `tapx://client/gzip/...`, which is a full TapX payload format
  rather than a simple vless-like URL.
- Raw TapX connector import must be able to consume the chosen TapX Raw URL
  format and fill Connector runtime mode, protocol, remote address, port,
  vKey, TLS/DTLS, and TUN/TAP binding fields.
- The final import/export contract should support one-click transfer from
  Listener to Connector while still allowing bare Raw TCP/UDP with no auth and
  no encryption.

## Current Web demo audit

The current 3x-ui-derived Web panel is a front-end demo/reference layer. It is
used to confirm operator workflow, naming, and screen composition before the
final TapX-specific front-end rewrite.

Items accepted as demo-only records:

- The System Status management card keeps the current 3x-ui-style layout with
  logs and backup/restore actions only. Runtime behavior behind those buttons
  will be adapted with the backend compatibility/API layer later.
- Embedded Xray and external Xray status cards are visually separated, but their
  current lifecycle actions still share compatibility handlers. The final
  rewrite must split state and actions by runtime.
- Device, Listener, Client, Connector, Route, Kernel, and panel data must move
  to DB-backed TapX objects before backup/restore is considered complete. Any
  frontend local-storage state that affects runtime config is temporary only.
- TapX Raw Listener export and Connector import are not complete until the
  chosen `raw://`/`tapx://` link format round-trips runtime mode, Raw TCP/UDP,
  endpoint, vKey, TLS/DTLS, and TUN/TAP binding.
- Kernel Settings currently define the desired screen shape, but save behavior
  is not final until the backend DB/API adapter persists TapX, embedded Xray,
  and external Xray runtime configuration.
- Link Binding IP/MAC fields are address-limit/source-guard fields, not
  interface address assignment fields. They must map to AddressLimit/runtime
  guard data in the final model.
- Link Test in the current demo is a front-end configuration inspection tool:
  it shows Connector to TUN/TAP binding, Listener user/vKey to TUN/TAP binding,
  and rule-level allowed IP/MAC limits. The final backend should expose a live
  DB/runtime lookup for the same questions.

Fixed in the demo layer:

- Device bridge settings are shown only for TAP devices. TUN devices cannot be
  bridged and any saved TUN device must clear bridge fields.
- The System Status management card no longer exposes the old config action.
- Two-factor authentication QR metadata uses `TapX-UI` as the issuer.

## Current front-end source

The front-end review records above remain product constraints. The independent
rewrite is now the production source:

- `web/` is the single TapX front-end source directory. The old 3x-ui-derived
  reference/demo tree was removed after the rewrite became production-ready.
- The new shell owns the sidebar, theme toggle, logout, and route naming. It
  keeps the menu order as: System Status, Devices, Listeners, Users,
  Connectors, Link Binding, Panel Settings, Kernel Settings.
- System Status is the first migrated vertical slice. It uses the native
  `/api/dashboard` contract instead of 3x-ui compatibility dashboard calls.
- The `web/` Vite dev server proxies native `/api/*` calls to the Go panel backend.
- Browser verification confirmed the clean `web/` System Status shell
  opens at `http://127.0.0.1:31990/`, contains no 3X-UI branding, exposes
  white/dark/deep theme controls, and keeps the agreed menu order.
- `web/` Link Binding is implemented as clean code over the native
  `/api/config` object set. It displays and edits Route objects, stores source
  IP/MAC limits through AddressLimit objects, and includes Link Test for
  device, connector, listener, user, and vKey lookups.
- The production pages use first-class TapX DB/API objects rather than
  frontend-only compatibility state.

## C and Go split

Go is the control plane. C is the raw fast path where performance matters.

There must be no per-packet cgo call. Go generates runtime config and supervises C workers. C forwards packets/frames and periodically exposes counters.

## Address limits are optional

Address Guard is enabled only when the operator configures allowed TAP/TUN IP/MAC information. If nothing is configured, TapX must not invent limits. If limits are configured, they must be enforced by the data plane.

Allowed IP/MAC limits remain useful even when a TUN/TAP interface itself has no
address and is bridged to a physical NIC. In that deployment, the address limit
is not a local interface address; it is a source guard applied to packets or
frames crossing the TapX data plane.

TUN checks:

- IPv4 source address.
- IPv6 source address.

TAP checks:

- Ethernet source MAC.
- ARP sender MAC/IP.
- IPv4 source address.
- IPv6 source address.
- ND source/link-layer data.

## MVP order

The first runnable product should be raw and measurable:

1. TUN + Raw UDP no-header forwarding.
2. TUN + Raw TCP length-prefix forwarding.
3. Optional vKey/Route/Client/address-limit runtime generation.
4. C fastpath counters.
5. Go runtime config process.
6. Same-process embedded Xray core runtime.
7. TUN IPv4/IPv6 allowed-address enforcement when configured.
8. TAP and Layer-2 guard.
9. Web UI completion.
10. External Xray compatibility.

External Xray compatibility now has a managed-runtime implementation: TapX
compiles external Xray JSON from panel objects and supervises the external
binary selected in Settings. Same-process embedded Xray now starts xray-core
inside `tapx-core`, exposes state, and includes the first direct frame/TUN/TAP
adapter for bound Xray endpoints. This adapter uses an in-process xray-core
path rather than a local redirect.

## Initial target platforms

Linux and OpenWrt must both be designed for small-memory deployments. The current
development and first OpenWrt build-validation target is x86-64 only. MT7986 and
other OpenWrt architectures are deferred until they are explicitly needed.
Future packaging should expand to all practical OpenWrt architectures.

## Public validation servers

Public servers are validation targets only. They are not development machines and must be restorable. Do not commit hostnames, credentials, or private keys.

## Device-scoped path MTU adaptation

- One `LinkAutoOptimize` switch is configured on a TAP/TUN Device. It enables the policy for every TapX, embedded Xray, and external Xray Listener or Connector bound to that Device. Protocol pages must not duplicate this switch, but each active endpoint/transport path keeps its own confirmed result.
- When disabled, the configured Device `MTU` and optional fixed `MSSClamp` are applied unchanged.
- `MSS` is a TCP-only value. The common cross-protocol result is a confirmed effective payload/MTU ceiling; TCP derives IPv4 and IPv6 MSS values from it, while datagram transports use it as their packetization ceiling.
- When enabled, the configured Device `MTU` is an upper bound. The Go control plane first tries that ceiling through the actual selected transport. A successful peer acknowledgement activates it after one round trip; only a failed ceiling probe falls back to the kernel route candidate and a short peer-confirmed binary search. No conservative fixed MTU is imposed.
- After activation there is no periodic or per-packet probing. Netlink route changes, a validated ICMP Packet Too Big indication, a connected-socket `EMSGSIZE`/error-queue event, or endpoint reconnect triggers a new confirmation. The old value remains immutable for existing workers until the replacement is ready.
- The confirmed result is compiled into immutable per-endpoint runtime values before workers carry normal traffic. The Device MTU remains the operator ceiling because one Device can feed several paths with different limits; one endpoint's poor path must not lower unrelated links.
- Raw TCP/TLS and Xray stream transports use normal outer TCP segmentation. The Device boundary receives a fixed inner TCP MSS derived from the confirmed effective MTU. Linux PMTU MSS clamping is only a TCP safety net; it is not the path discovery mechanism.
- TUN and TAP keep the configured Device MTU as their common upper bound. Where routing identifies a single endpoint, Go installs the endpoint-derived TCP MSS policy for that route. Packets or frames that cannot be made endpoint-specific before the Device boundary are handled against the selected endpoint's fixed payload ceiling after routing. This avoids pretending that one Linux interface MTU can represent several different transport paths.
- Raw UDP/DTLS and managed Xray datagram transports use the confirmed payload ceiling. Ordinary frames below the ceiling keep the normal fast path. A TapX datagram carrying an oversized inner IP packet or Ethernet frame is segmented and reassembled above the selected transport, so the outer IP layer never fragments it. The fixed size comparison and the already-selected segmentation function are the only data-plane work added by this enabled feature.
- TapX-owned IPv4 datagram sockets use discover-only/DF PMTU behavior and IPv6 datagram sockets prohibit fragmentation. A probe is successful only after the peer acknowledges the exact probe identity and size; a successful local `send` is never treated as proof that the path delivered it.
- Raw UDP cannot confirm a path without peer feedback. Enabling optimization therefore enables a minimal TapX probe/segment control format for that link. Disabling it preserves the original no-header Raw UDP mode with no optimizer work or added bytes.
- mKCP and any other generated Xray transport with an explicit MTU/packet-size field receive the confirmed value in the generated runtime copy. QUIC/Hysteria-style transports retain their own congestion and packetization logic but are capped by the same confirmed TapX payload. Embedded and external Xray are compiled from the same TapX runtime objects; operator-owned source JSON is never rewritten in place.
- Existing advanced protocol values remain editable. When automatic optimization is enabled, an explicit Xray `tcpMaxSeg`, mKCP MTU, DTLS MTU, QUIC packet-size limit, or equivalent value is treated as an additional upper bound: the generated effective value is the smaller of the operator ceiling and the confirmed path value. An advanced value can tune a link lower but cannot silently raise it above the confirmed safe limit.
- A path is not activated until peer-confirmed probes succeed. If no probe size is confirmed, startup fails with a visible diagnostic instead of silently selecting an unverified value. If an active path reports a smaller limit, TapX immediately blocks newly oversized datagrams, confirms the replacement ceiling, atomically swaps the endpoint runtime values, and resumes without allowing a persistent black hole.
- Probe orchestration, route inspection, overhead calculation, Xray JSON mutation, MSS derivation, and segment-plan generation live in the Go control plane. The C fast path receives only fixed numeric limits and preselected function paths; it performs no DB read, JSON parse, shell call, dynamic MSS calculation, or per-packet cgo call.

## Component update boundary

- TapX and embedded Xray are displayed separately for runtime state, logs, stop, and restart operations, but they share one core update action.
- Embedded Xray is not hot-swapped independently. It is upgraded with the compatible TapX Core build so TUN/TAP and the selected transport remain in the same data-plane process.
- TapX-UI can have a separate update entry, but release installation must validate the panel/core compatibility described by `tapx-update-manifest.json`.
- External Xray remains an independently managed official binary.
- Version checks, release downloads, checksum verification, extraction, and process restart are Go control-plane work initiated by an administrator. None of them add work to raw UDP/TCP, TAP/TUN, C fastpath, or embedded Xray packet handling.
