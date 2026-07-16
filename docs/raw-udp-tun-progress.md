# Raw UDP/TCP TAP/TUN Progress

This is the current first runnable path for TapX raw networking.

## Current Flow

```text
Panel/API objects
  -> config.ValidateForSave / ValidateForApply
  -> config.GenerateRuntime
  -> core.Supervisor
  -> tuntap.Open
  -> UDP/TCP socket
  -> fastpath.StartUDPPipe / fastpath.StartTCPPipe
  -> C epoll worker
```

The C worker receives only prepared runtime values:

- TUN/TAP fd,
- UDP socket fd,
- frame kind,
- max frame size,
- peer mode,
- optional fixed peer,
- optional TUN IPv4 address guard prefixes,
- optional TAP MAC guard entries,
- optional TAP IPv4/ARP guard prefixes,
- optional TUN/TAP IPv6 guard prefixes,
- optional TAP IPv6 ND guard checks,
- counters pointer.

It does not read DB state, parse JSON, call shell commands, or know about panel objects.

## Implemented

- Linux TUN/TAP open wrapper.
- Linux Device apply layer for MTU, IPv4/IPv6 CIDR assignment, interface up,
  MSS clamp, TAP bridge creation/member binding, static route apply, DNS
  resolv.conf output, and rollback on startup failure or close.
- Path-MTU control-plane implementation: structured `ip -json` route/link MTU
  candidate parsing, exact-size random peer probes, retry and binary-search
  fallback, endpoint/path-scoped confirmed-plan cache, immutable datagram and
  MSS planning, correct TAP Ethernet-frame allowance, and DF/PMTU socket policy.
  A local send is never accepted as path confirmation. The final successful
  random probe is committed and acknowledged by both peers before activation.
- Optional `TXS1` datagram segmentation is implemented in the C Raw UDP worker
  and the Go DTLS bridge. It supports vKey composition, out-of-order fragments,
  duplicate suppression, eight preallocated reassembly slots, asymmetric send
  and receive ceilings, and original-frame counters. A zero runtime ceiling
  preserves the original no-header Raw UDP wire format and allocation path.
- Confirmed limits can be compiled into the generated runtime copy without
  rewriting DB/operator settings. Raw UDP startup now performs the peer
  confirmation automatically when the Device switch is enabled. A listener
  waits in the background and atomically starts its C worker after commit; a
  connector confirms synchronously before its worker starts. Fixed-peer
  listeners can also confirm simultaneously with each other; neither side is
  assigned a hidden initiator role.
- The persisted and generated default Device MTU is 1500. Path confirmation
  never rewrites or lowers that interface MTU. The reported single-datagram
  network MTU only describes how much inner payload fits before TapX
  segmentation is required.
- Once Raw UDP or DTLS confirmation succeeds, the temporary generic MSS clamp
  is replaced with the exact confirmed IPv4 and IPv6 values. This happens in
  startup control-plane code and adds no packet-path shell call or lookup.
- DTLS performs the same confirmation inside the completed encrypted
  association. The negotiated DTLS cipher suite supplies exact AEAD expansion
  or worst-case CBC padding, so the generated segment ceiling includes record
  overhead. Automatic mode explicitly completes Pion's otherwise lazy DTLS
  handshake before reading that state. Real certificate/cipher negotiation and
  encrypted loopback commit are covered by tests.
- Oversized DF probe sends and responses are treated as failed sizes instead
  of terminating the responder. Automatic optimization remains startup-only;
  it adds no probe, route lookup, allocation, or branch to the unsegmented hot
  path.
- Raw TCP and TLS receive an address-family-aware `TCP_MAXSEG` ceiling on the
  outer socket when Device automatic optimization is enabled. The kernel can
  still lower it through normal PMTU handling; disabled automatic optimization
  leaves the socket setting untouched.
- Embedded and external Xray compile from the same generated policy. TCP,
  WebSocket, gRPC, HTTPUpgrade, and XHTTP receive `tcpMaxSeg`; mKCP receives a
  bounded protocol MTU; QUIC/Hysteria path-MTU discovery is enabled. Explicit
  advanced values below the generated ceiling are preserved, larger values are
  clamped only in the runtime copy, and conflicting attempts to disable
  automatic QUIC PMTU are rejected before apply.
- Linux error-queue feedback and active-worker reconfirmation/swap are still
  pending. Startup confirmation and cache replacement are implemented for Raw
  UDP/DTLS.
- Raw UDP/TUN C worker with epoll and pthread.
- UDP peer modes: any, fixed, learn.
- Raw UDP bind address override, bind interface, receive/send socket buffers,
  `SO_REUSEADDR`, and `SO_REUSEPORT` are generated into runtime config and
  applied when opening the UDP socket.
- Raw TCP/TUN C worker with uint16/uint32 length-prefix framing.
- Raw TCP bind address override, bind interface, receive/send socket buffers,
  TCP Fast Open, TCP_NODELAY, keepalive, and connect timeout are generated into
  runtime config and applied by the Go core before the C fastpath owns the TCP
  fd.
- Go cgo wrapper for starting/stopping C workers.
- Go supervisor that opens TUN/TAP, UDP sockets, and TCP listener/connector sockets, then starts fastpath workers from generated runtime config.
- AddressLimit IPv4 CIDRs generated into TUN IPv4 guard tables when configured.
- TAP MAC Guard generated into C fastpath when configured.
- TAP IPv4 and ARP Guard generated into C fastpath when configured.
- TUN/TAP IPv6 Guard generated into C fastpath when configured.
- TAP ND Guard for Neighbor Solicitation/Advertisement basics generated into C fastpath when configured.
- Optional vKey wire marker generated into raw UDP/TCP fastpath when `VKeyValue` is configured; empty vKey remains bare UDP or length-prefix-only TCP.
- `tapx-core -config <file>` apply/check entry for local JSON runtime-object files.
- `tapx-panel` SQLite/PostgreSQL object store and HTTP API for config read/write, object CRUD, validation, and runtime generation, including XrayProfile and Settings control-plane objects.
- `tapx-panel` local runtime apply/stop/state API backed by the Go supervisor and C fastpath workers.
- Runtime apply keeps the last successfully applied runtime and attempts to roll it back if replacement startup fails.
- Embedded `tapx-panel` Web UI for object CRUD, dense field editing, full JSON editing, runtime apply/stop/state, operation logs, backup/restore, diagnostics, and counters.
- Optional panel login/session support protects control-plane APIs when enabled and does not add raw transport auth.
- `/api/stats` aggregates current C counter snapshots by transport, endpoint,
  device, route, and client, including client traffic-cap/expiration views.
- C counters accumulate per epoll drain and publish atomically before the
  worker sleeps; Go reads an atomic snapshot. Snapshot collection crosses cgo
  once per aggregation interval, while frame forwarding never crosses cgo or
  performs a cross-core atomic increment per packet.
- Runtime manager periodically enforces disabled, expired, and over-quota
  Client bindings by closing matching pipes from the Go supervisor; the packet
  path still only updates counters.
- Client credential fields, direct Connector binding, and share output are
  wired through the panel API/UI. Raw clients generate `raw://` links,
  supported Xray clients generate native protocol links, and structured import
  payloads remain available without QR-code or subscription output.
- Durable dashboard history is sampled from Go-side aggregated counters into
  SQLite and included in backup/restore. No metric write occurs in the C packet
  path.
- C tests for both directions: TUN to UDP and UDP to TUN.
- C and Go tests for both Raw TCP directions: TUN to TCP framed stream and TCP framed stream to TUN.
- C and Go tests for optional vKey marker on Raw UDP and Raw TCP.
- C and Go tests for TUN IPv4 address guard drops.
- C and Go tests for TAP MAC/IPv4 guard drops.
- C tests for TAP ARP guard drops.
- C and Go tests for TUN IPv6 guard drops.
- C tests for TAP IPv6/ND guard drops.
- Optional real TUN supervisor tests with `TAPX_TEST_TUNTAP=1`.
- Optional real TAP supervisor test with `TAPX_TEST_TUNTAP=1`.
- Linux netns end-to-end raw UDP/TUN integration: two `tapx-core` processes, two TUN devices, UDP underlay, and bidirectional tunnel ping.
- Automatic-link netns coverage uses a 1280-byte outer link with a 1500-byte
  inner device, verifies large DF inner packets, and asserts that IPv4 fragment
  creation and reassembly counters do not increase.
- Linux netns end-to-end raw UDP/TUN vKey integration: two `tapx-core` processes with matching generated vKey marker and bidirectional tunnel ping.
- Linux netns end-to-end raw UDP/TAP integration: two `tapx-core` processes, two TAP devices, UDP underlay, ARP over the tunnel, and bidirectional tunnel ping.
- Linux netns end-to-end raw TCP/TUN integration: listener/connector `tapx-core` processes, two TUN devices, TCP underlay, length-prefix framing, and bidirectional tunnel ping.
- Linux netns Address Guard integration: guarded TUN and TAP pairs allow
  configured source IP/MAC traffic and block unauthorized source IP traffic.
- Public-lab embedded Xray frame/TUN/TAP smoke: two `tapx-core` processes,
  same-process xray-core VLESS transport, real `tapxxray0` TUN and
  `tapxxraytap0` TAP devices visible through `ip a` and `ip -d addr`, and
  bidirectional tunnel ping for both interface types.
- Embedded Xray framing now uses the official pooled MultiBuffer writer on the
  device-to-Xray direction, batches immediately available frames without
  adding latency, and directly consumes contiguous pooled receive buffers.
  Split frames fall back to caller-owned reusable storage. The adapter remains
  entirely outside the official Xray module.
- Device-bound external Xray endpoints now generate a supervised loopback-only
  frame bridge. The listener bridge is ready before external Xray starts; the
  connector bridge starts after the generated local Xray inbound is bound.
- The external bridge uses a dedicated allocation-free Go frame loop instead
  of the generic Raw TCP C worker. Public-lab TUN throughput improved from the
  broken 1-4 Mbit/s range to about 61-67 Mbit/s in the recorded VLESS/TCP run.
- Complete TAP Layer-2 integration passes unchanged over Raw UDP, embedded
  Xray, and managed external Xray, including PPPoE, VLAN/QinQ, control-protocol
  multicast, MAC learning, and 1522-byte Ethernet frames.
- The current product-mode Xray matrix passes 32/32 cases: embedded/external,
  TUN/TAP, and eight protocol/transport combinations. Each case validates
  bidirectional inner IPv4/IPv6 and DF large packets; external Xray is started
  and supervised by TapX rather than a test-side wrapper.
- Stream Xray transports now rely on outer kernel PMTUD instead of an inner-MTU
  TCP MSS cap. mKCP automatic framing uses a 1232-byte ceiling and passes an
  IPv6 MTU-1280 netns test without outer fragmentation.
- User rate limits pass on Raw TCP and both Xray runtime modes using delivered
  byte counts. Unset limits still return the original connection and add no
  hot-path pacing work.

## Verification

```bash
GOTOOLCHAIN=auto make test
GOTOOLCHAIN=auto TAPX_TEST_TUNTAP=1 go test -race ./internal/core -run 'TestSupervisorStarts(UDP|TAPUDP|TCP)Pipe.*Optional' -v
GOTOOLCHAIN=auto go run ./cmd/tapx-core -config docs/examples/raw-udp-tun.json -check
GOTOOLCHAIN=auto go run ./cmd/tapx-core -config docs/examples/raw-udp-tun-vkey.json -check
GOTOOLCHAIN=auto go run ./cmd/tapx-core -config docs/examples/raw-udp-tap-guard.json -check
GOTOOLCHAIN=auto go run ./cmd/tapx-core -config docs/examples/raw-tcp-tun.json -check
make integration-netns
make integration-device-apply-netns
make integration-bridge-apply-netns
make integration-mss-clamp-netns
make integration-dns-apply-netns
make integration-tun-netns
make integration-tun-vkey-netns
make integration-tap-netns
make integration-tcp-tun-netns
GOTOOLCHAIN=auto go test ./internal/panel -v
GOTOOLCHAIN=auto go test -race ./internal/pathmtu ./internal/core
make -C core/fastpath clean test
```

## Next Work

- Expand dense field editors with richer option pickers and dependency-aware selects.
- Add no-interruption reload where resources can be prepared in parallel.
- Replace shell-based tapx-net apply with a netlink/native backend where it
  materially improves reliability.
- Add guarded TAP/TUN netns integration cases for AddressLimit enforcement.
- Extend IPv6/ND handling for extension headers and broader ICMPv6 edge cases.
