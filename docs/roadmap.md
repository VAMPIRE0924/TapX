# Roadmap

## Phase 0: Environment and ABI

- Local WSL development environment.
- C fastpath ABI skeleton.
- Go command skeleton.
- Build/test scripts.
- Public lab scripts without committed secrets.

## Phase 1: Raw TUN MVP

- Create/open TUN.
- Read/write TUN packets.
- Apply Device MTU, MSS clamp, IPv4/IPv6 addresses, interface up state, static routes, and DNS resolv.conf output after TUN/TAP creation. (Initial Linux implementation exists.)
- Raw UDP no-header forwarding.
- Raw TCP length-prefix forwarding.
- C worker counters.
- Go process starts/stops workers.
- Runtime config supports unset vKey/Client/Route/address limits with no extra hot-path checks.
- Runtime config can generate optional vKey/Route/Client/address-limit behavior when configured.
- Optional raw UDP/TCP vKey wire marker in C fastpath when vKey is configured. (Initial implementation exists.)
- Raw UDP socket bind address/interface, buffers, and reuse flags generated from
  RawUDP settings and applied by the Go core. (Initial implementation exists.)
- Raw TCP socket bind address/interface, buffers, TCP_NODELAY, keepalive,
  connect timeout, and TCP Fast Open generated from RawTCP settings and applied
  by the Go core. (Initial implementation exists.)
- Raw TCP TLS advanced config fields are exposed and run through a dedicated
  Go TLS frame runtime when enabled; naked Raw TCP remains on the C fastpath.
- Raw UDP DTLS advanced config fields are exposed and run through a dedicated
  Go DTLS datagram runtime when enabled; naked Raw UDP remains on the C
  fastpath.
- TUN IPv4 AddressLimit checks in C fastpath when configured.
- TAP MAC Guard and ARP/IPv4 Guard in C fastpath when configured.
- TUN/TAP IPv6 Guard and initial TAP ND Guard in C fastpath when configured.
- Raw UDP/TUN, Raw UDP/TUN with vKey, Raw UDP/TAP, Raw TCP/TUN, and Address
  Guard allow/drop integration tests with Linux network namespaces. (Initial
  tests exist.)

## Phase 2: Same-Process Xray

- Add embedded Xray transport inside the TapX core process.
- Keep raw UDP/TCP independent from Xray.
- Avoid local redirect and visible internal ports.
- Current code starts xray-core inside the TapX process, reports runtime state
  without an external binary, and passes bound TUN packets or TAP Ethernet
  frames over xray-core through an in-process frame adapter.

## Phase 3: Optional Binding and Limits

- Generate vKey/Client/Route bindings into fastpath tables.
- TUN IPv4 allowed-address checks when configured. (Initial C fastpath implementation exists.)
- TUN IPv6 allowed-address checks when configured. (Initial C fastpath implementation exists.)
- Drop counters and aggregated stats.

## Phase 4: TAP

- TAP fd handling.
- Ethernet frame forwarding.
- MAC Guard. (Initial C fastpath implementation exists.)
- ARP Guard. (Initial C fastpath implementation exists.)
- IPv4/IPv6 Guard. (Initial C fastpath implementation exists.)
- ND Guard. (Initial C fastpath implementation exists.)
- Bridge apply and rollback. (Initial Linux implementation exists.)
- Static route apply and rollback. (Initial Linux implementation exists.)
- MSS clamp apply and rollback. (Initial Linux implementation exists.)
- DNS resolv.conf output and rollback. (Initial Linux implementation exists.)

## Phase 5: Control Plane and UI

- SQLite flexible object store and migration. (Initial implementation exists.)
- Device, Listener, Connector, Client, Route, vKey, AddressLimit, Xray Profile, and Settings APIs. (Initial implementation exists.)
- Save/apply validation API and runtime generation API. (Initial implementation exists.)
- Runtime apply/stop/state API backed by the Go supervisor. (Initial implementation exists.)
- Rollback-on-start-failure for runtime apply, preserving the last successfully applied runtime when replacement startup fails. (Initial implementation exists.)
- Embedded Web UI for object CRUD, dense field editing, full JSON editing, runtime controls, and counters. (Initial implementation exists.)
- Dense field editors for core Device, Listener, Connector, Client, Route, vKey, AddressLimit, Xray Profile, and Settings objects. (Initial implementation exists.)
- Optional Settings-driven panel login with PBKDF2 password hashes and HTTP-only sessions. (Initial implementation exists.)
- Backup export/restore API and UI. (Initial implementation exists.)
- In-process panel operation log API and UI. (Initial implementation exists.)
- Read-only diagnostics API and UI for process, runtime, object counts,
  fastpath ABI, and current x86-64 OpenWrt build target. (Initial implementation exists.)
- Aggregated stats API/UI for total, endpoint, device, route, client, and
  client quota/expiration views. (Initial implementation exists.)
- Runtime traffic quota/expiration enforcement lifecycle. (Initial
  control-plane implementation exists.)
- Client traffic reset API/UI with reset timestamp and RX/TX offset metadata.
  (Initial implementation exists.)
- Client credential and sharing flow for raw TapX imports and VLESS links, with
  QR PNG output. (Initial implementation exists.)
- No-interruption reload where resources can be prepared in parallel. (Initial
  conservative implementation exists: disjoint resource sets use prepare-first;
  conflicting ifnames/listener ports keep stop-first rollback.)
- Dashboard expansion and richer diagnostics. (Initial implementation exists:
  `/api/dashboard` returns runtime, stats, object counts, diagnostics, rates,
  and recent logs; Web Dashboard renders the same overview.)

## Phase 6: Xray Compatibility

- External Xray compatibility mode. (Initial implementation exists: config
  compiler, process lifecycle, generated config cleanup, runtime state, and
  Web/API binary status/upload/download management.)

## Phase 7: OpenWrt

- Minimal tapx-core package for x86-64 first. (Initial `.ipk` packaging exists.)
- UCI config. (Initial implementation exists.)
- procd service. (Initial implementation exists.)
- LuCI app matching Linux panel fields. (Initial UCI/procd/runtime editor exists
  with object field templates, saved-runtime check, service status, and reload
  actions.)
- Non-x86 packaging, including MT7986, only when explicitly needed.
- Future full-architecture packaging.

## Delivery

- Linux amd64 build script. (Initial implementation exists.)
- Linux systemd unit and installer script. (Initial implementation exists,
  including first-install admin/password/base-path initialization.)
