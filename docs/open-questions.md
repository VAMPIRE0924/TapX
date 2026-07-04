# Implementation Notes

These notes track implementation details that should not be confused with product requirements already decided by the user.

## GitHub identity

Recommended product name: `TapX`.

Public repository target: `VAMPIRE0924/TapX`.

The Go module currently uses local path `tapx`. Before public GitHub publishing, choose the final owner/repository path so the module path can be changed once.

## OpenWrt packaging details

The current OpenWrt development target is x86-64 only. MT7986 and other
architectures are later packaging targets to enable when needed. Exact target
triples, libc expectations, package feed layout, and minimum RAM/flash test floor
are packaging details to verify during build work, not product-scope questions.
The current checked build entry is `make build-openwrt-x86`, which emits
`build/openwrt-x86-64/tapx-core`.

## Embedded Xray integration

Same-process embedded Xray now starts xray-core through its Go API and includes
the first TapX frame/TUN/TAP adapter over xray-core without visible internal
ports. Follow-up questions are reconnect policy, multiple simultaneous stream
policy, and UDP/datagram-specific Xray transport behavior.

External Xray compatibility is now implemented as a managed external process
for validation and compatibility. It does not provide the direct TapX
frame/datagram adapter, and it must not be used as a hidden local redirect
substitute for embedded mode.
