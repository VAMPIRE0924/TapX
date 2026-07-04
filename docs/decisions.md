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

## C and Go split

Go is the control plane. C is the raw fast path where performance matters.

There must be no per-packet cgo call. Go generates runtime config and supervises C workers. C forwards packets/frames and periodically exposes counters.

## Address limits are optional

Address Guard is enabled only when the operator configures allowed TAP/TUN IP/MAC information. If nothing is configured, TapX must not invent limits. If limits are configured, they must be enforced by the data plane.

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
