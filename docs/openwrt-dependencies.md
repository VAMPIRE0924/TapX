# OpenWrt Dependencies

TapX packages declare runtime dependencies in `openwrt/Makefile`. On an OpenWrt
system with matching package feeds, installing `tapx-core`, `tapx-panel`, or
`luci-app-tapx` must resolve the required APK/IPK packages automatically.

## Runtime dependencies

`tapx-core` directly declares:

- `libc`: C fast path and Go cgo runtime.
- `kmod-tun`: `/dev/net/tun` and both TUN/TAP device modes.
- `ip-full`: device, address, route, bridge and JSON route/link operations.
- `tc-full`: `clsact`, flower filters and mirred actions.
- `kmod-sched-flower`: flower classifier used for transparent TAP control frames.
- `iptables-nft`: IPv4 MSS rules through the nftables compatibility backend.
- `ip6tables-nft`: IPv6 MSS rules through the nftables compatibility backend.
- `dnsmasq`: DHCP service for TAP server mode.
- `isc-dhcp-relay-ipv6`: `dhcrelay` with IPv4/IPv6 support for the optional relay mode.
- `ca-bundle`: system trust store for TLS, DTLS and HTTPS downloads.

The OpenWrt package resolver installs transitive dependencies such as
`kmod-sched-core`, `act_mirred`, `kmod-ipt-core`, `kmod-ip6tables`,
`kmod-nft-compat`, `xtables-nft`, `libbpf`, `libnl-tiny` and `libxtables`.
Do not duplicate these transitive packages in the TapX manifest because their
exact package graph can change between OpenWrt releases.

`tapx-panel` declares `libc`, `tapx-core` and `ca-bundle`. The PostgreSQL driver,
SQLite driver, ZIP extraction, checksum verification and HTTP downloader are
compiled into the TapX binaries and do not require `libpq`, `sqlite3-cli`,
`unzip`, `curl`, `wget` or `openssl-util` at runtime.

`luci-app-tapx` declares `luci-base`, `rpcd`, `tapx-core` and `tapx-panel`.
Installing it therefore installs the complete TapX runtime and Web panel.

## Firmware-only validation packages

These packages are useful on the MT7986 lab firmware but are not TapX runtime
dependencies and must not be forced onto every production device:

- `ip-bridge`, `ss`, `nstat`: bridge FDB and network diagnostics.
- `iperf3`: upload and download throughput validation.
- `tcpdump`: TAP, VLAN, PPPoE, ARP and IPv6 packet evidence.
- `ethtool`: NIC offload and link inspection.
- `openssl-util`: certificate and TLS diagnostics.
- `kmod-8021q`: local VLAN/QinQ endpoint configuration for validation.
- `kmod-ppp`, `kmod-pppox`, `kmod-pppoe`: local PPPoE endpoint validation.

TAP transports VLAN and PPPoE Ethernet frames as opaque layer-2 payloads, so
the VLAN and PPPoE packages are not required merely to forward those frames.

## Offline or unavailable feeds

Kernel packages must match the firmware kernel ABI. If the device has no
matching package feed, select the direct TapX runtime dependencies in the same
firmware build tree and run `make defconfig`; the build system will include the
matching transitive dependencies. Do not install kernel packages built for a
different firmware revision.
