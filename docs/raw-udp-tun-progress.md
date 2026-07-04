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
- `tapx-panel` SQLite object store and HTTP API for config read/write, object CRUD, validation, and runtime generation, including XrayProfile and Settings control-plane objects.
- `tapx-panel` local runtime apply/stop/state API backed by the Go supervisor and C fastpath workers.
- Runtime apply keeps the last successfully applied runtime and attempts to roll it back if replacement startup fails.
- Embedded `tapx-panel` Web UI for object CRUD, dense field editing, full JSON editing, runtime apply/stop/state, operation logs, backup/restore, diagnostics, and counters.
- Optional panel login/session support protects control-plane APIs when enabled and does not add raw transport auth.
- `/api/stats` aggregates current C counter snapshots by transport, endpoint,
  device, route, and client, including client traffic-cap/expiration views.
- Runtime manager periodically enforces disabled, expired, and over-quota
  Client bindings by closing matching pipes from the Go supervisor; the packet
  path still only updates counters.
- Client credential fields, direct Connector binding, and initial share output
  are wired through the panel API/UI. Raw clients generate compressed
  `tapx://client/gzip/...` links, VLESS clients can generate `vless://` links,
  and both include QR PNG output plus structured import payloads.
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
- Linux netns end-to-end raw UDP/TUN vKey integration: two `tapx-core` processes with matching generated vKey marker and bidirectional tunnel ping.
- Linux netns end-to-end raw UDP/TAP integration: two `tapx-core` processes, two TAP devices, UDP underlay, ARP over the tunnel, and bidirectional tunnel ping.
- Linux netns end-to-end raw TCP/TUN integration: listener/connector `tapx-core` processes, two TUN devices, TCP underlay, length-prefix framing, and bidirectional tunnel ping.
- Linux netns Address Guard integration: guarded TUN and TAP pairs allow
  configured source IP/MAC traffic and block unauthorized source IP traffic.
- Public-lab embedded Xray frame/TUN/TAP smoke: two `tapx-core` processes,
  same-process xray-core VLESS transport, real `tapxxray0` TUN and
  `tapxxraytap0` TAP devices visible through `ip a` and `ip -d addr`, and
  bidirectional tunnel ping for both interface types.

## Verification

```bash
GOTOOLCHAIN=local make test
GOTOOLCHAIN=local TAPX_TEST_TUNTAP=1 go test -race ./internal/core -run 'TestSupervisorStarts(UDP|TAPUDP|TCP)Pipe.*Optional' -v
GOTOOLCHAIN=local go run ./cmd/tapx-core -config docs/examples/raw-udp-tun.json -check
GOTOOLCHAIN=local go run ./cmd/tapx-core -config docs/examples/raw-udp-tun-vkey.json -check
GOTOOLCHAIN=local go run ./cmd/tapx-core -config docs/examples/raw-udp-tap-guard.json -check
GOTOOLCHAIN=local go run ./cmd/tapx-core -config docs/examples/raw-tcp-tun.json -check
make integration-netns
make integration-device-apply-netns
make integration-bridge-apply-netns
make integration-mss-clamp-netns
make integration-dns-apply-netns
make integration-tun-netns
make integration-tun-vkey-netns
make integration-tap-netns
make integration-tcp-tun-netns
GOTOOLCHAIN=local go test ./internal/panel -v
```

## Next Work

- Expand dense field editors with richer option pickers and dependency-aware selects.
- Add no-interruption reload where resources can be prepared in parallel.
- Replace shell-based tapx-net apply with a netlink/native backend where it
  materially improves reliability.
- Add guarded TAP/TUN netns integration cases for AddressLimit enforcement.
- Extend IPv6/ND handling for extension headers and broader ICMPv6 edge cases.
