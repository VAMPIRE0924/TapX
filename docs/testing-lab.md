# Public Validation Lab

Public servers are used only for validation and benchmarks after local work is ready.

Rules:

- Do not develop on the servers.
- Do not commit server credentials.
- Do not commit lab-specific host notes.
- Keep host notes under `.local/`.
- Any key or package added for testing must be tracked so it can be removed later.
- The local development machine must not open naked raw UDP/TCP data-plane
  sessions to the foreign public servers. The lab scripts use local SSH only;
  TapX raw data-plane traffic runs between the two remote validation servers.
- TLS/Xray/obfuscated wrapping remains available when that transport shape needs
  to be validated, but it is not required for server-to-server raw smoke.

Expected validation uses:

- Raw UDP reachability and throughput.
- Raw TCP throughput and reconnect behavior.
- TUN route interoperability.
- Device apply validation for MTU/IP settings in Linux network namespaces.
- MSS clamp rule apply/rollback validation in Linux network namespaces.
- DNS resolv.conf output/rollback validation in Linux network namespaces.
- Static route apply validation in Linux network namespaces.
- TAP bridge creation/member binding validation in Linux network namespaces.
- Address Guard drop tests.
- Linux netns Address Guard allow/drop validation for TUN source IP and TAP
  source IP/MAC paths.
- Long-running counter aggregation tests.

The repository includes generic scripts under `scripts/lab/`. Pass hosts and
key paths explicitly from local/private notes.

## Read-Only Preflight

Run a read-only preflight before uploading binaries or starting TapX on the
public validation servers:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/lab/preflight.ps1 `
  -HostName <public-ip-a>,<public-ip-b> `
  -KeyPath .local/lab-key `
  -User root
```

The preflight checks the local lab binary is an ELF file, then reads remote
hostname/kernel/architecture, `/dev/net/tun`, `ip`, `ping`, `ss`, existing
`/tmp/tapx-lab-*` leftovers, and whether the planned UDP/TCP ports are already
listening. It does not create files or directories on the servers.

## Generic Raw Transport Smoke

Build a Linux x86-64 `tapx-core` binary for public-lab validation:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/build-linux-amd64.ps1
```

The build helper defaults to `-Builder auto` and tries Docker first, then WSL.
Use `-Builder docker -DockerImage golang:1.26-bookworm` when Docker Desktop has
access to that image, or `-Builder wsl -Distro Ubuntu-24.04` when WSL carries the
Linux Go/gcc toolchain. Public servers must not be used as build machines.

Run raw UDP/TUN, raw TCP/TUN, and raw UDP/TAP smoke validation between the two
remote lab hosts. The local machine only uses SSH for orchestration.

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/raw-transport-smoke.ps1 `
  -HostA <remote-host-a> `
  -HostB <remote-host-b> `
  -KeyPath .local/lab-key `
  -User root `
  -Build
```

The smoke script:

- uploads only the built `tapx-core` binary and generated temporary JSON config,
- creates a unique `/tmp/tapx-lab-*` directory on each host,
- starts raw UDP/TUN, raw TCP/TUN, or raw UDP/TAP pairs,
- prints running `ip -d addr show dev ...` interface evidence before ping,
- verifies tunnel reachability with ping over the TUN/TAP addresses,
- stops TapX processes and removes the remote lab directory by default.

Use `-Mode udp`, `-Mode tcp`, `-Mode tap`, or `-Mode all` to run specific
paths. During runtime, `ip a` can see `tapxudp0`, `tapxtcp0`, or `tapxtap0`;
after the process stops, those non-persistent TUN/TAP interfaces disappear.
This visibility is a validation requirement because production operators may
attach nftables port mapping, forwarding, route, and bridge rules to the
TapX-created interface names.
Use `-KeepRemote` only for debugging; when it is used, manually remove the
printed remote directory and stop any lab PIDs before considering the servers
restored.

Local Linux netns integration also includes `raw-tcp-tls-tun-netns.sh` and
`raw-udp-dtls-tun-netns.sh`. They start Raw TCP/TUN over TLS or Raw UDP/TUN
over DTLS with vKey on both sides, print `ip -d addr show` for both generated
TUN interfaces, and verify bidirectional tunnel ping. These tests cover the
protected runtimes without changing the naked Raw TCP/UDP C fastpaths.

Credentials, host notes, private keys, and generated one-off access material
must stay in `.local/` or outside the repository.

## Generic Raw Transport Benchmark

The benchmark script uses the same restore rules as the smoke script. It does
not install packages on the servers. The hosts must provide `ip`, `ping`, and
`/dev/net/tun`. For throughput tooling, `-Tool auto` prefers `iperf3` when both
hosts have it, then falls back to a Python socket benchmark when both hosts have
`python3`. The local machine only uses SSH for orchestration.

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/raw-transport-benchmark.ps1 `
  -HostA <remote-host-a> `
  -HostB <remote-host-b> `
  -KeyPath .local/lab-key `
  -User root `
  -Build `
  -Tool auto `
  -Duration 30 `
  -Parallel 4 `
  -Mode both `
  -Traffic tcp
```

The script benchmarks raw UDP/TUN and raw TCP/TUN transports over the generated
TUN pair. `-Traffic tcp` runs TCP application traffic through the tunnel;
`-Traffic udp` or `-Traffic both` can be used when UDP payload throughput is
needed. Use `-OutputDir .local/lab-results` to keep benchmark JSON locally
without committing it. The Python fallback is a practical validation tool for
hosts where package installation is not desired; use `iperf3` for formal release
numbers when it is available.

## Foreign Public Servers

For foreign public validation servers, first run read-only preflight. Do not run
local-machine-to-server naked raw data-plane tests. Remote server-to-server raw
smoke/benchmark is acceptable through these SSH-orchestrated scripts. A
TLS/Xray-wrapped validation paths are also available when protected transport
shapes need to be tested, and they still clean `/tmp/tapx-lab-*` on exit.

The embedded Xray smoke validates the same-process runtime path:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/xray-embedded-smoke.ps1 `
  -HostName <public-ip-a>,<public-ip-b> `
  -KeyPath .local/lab-key `
  -User root
```

The embedded Xray smoke starts the Xray core inside `tapx-core`, verifies the
configured local listener with `ss`, and checks that no external `xray` process
was launched. It is a same-process runtime validation; it does not install Xray
on the remote host.

The embedded Xray frame/TUN/TAP smoke validates the direct TapX frame adapter
over xray-core:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/xray-frame-tun-smoke.ps1 `
  -HostA <public-ip-a> `
  -HostB <public-ip-b> `
  -KeyPath .local/lab-key `
  -User root `
  -Mode both `
  -Build
```

It starts a bound embedded Xray listener on HostA and a bound embedded Xray
connector on HostB, prints `ip a show dev` plus `ip -d addr show dev` for
`tapxxray0` TUN and `tapxxraytap0` TAP, then verifies bidirectional ping over
both generated interface pairs. It does not use an external `xray` process or a
hidden local redirect port.

The protected raw smoke validates remote server-to-server Raw TCP/TLS/TUN and
Raw UDP/DTLS/TUN without naked local-machine-to-public-server data-plane
traffic:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/raw-protected-smoke.ps1 `
  -HostA <public-ip-a> `
  -HostB <public-ip-b> `
  -KeyPath .local/lab-key `
  -User root `
  -Mode both
```

It generates a one-day self-signed certificate in the remote lab directory,
copies only the certificate to HostB, starts protected TapX pairs between the
two public hosts, prints `ip a show dev` plus `ip -d addr show dev` for
`tapxtls0` and `tapxdtls0`, verifies bidirectional ping over both TUN pairs,
and cleans the remote lab directories by default.

The repository also includes an initial wrapped raw TCP/TUN smoke helper:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/lab/xray-wrapped-raw-tcp-smoke.ps1 `
  -HostA <public-ip-a> `
  -HostB <public-ip-b> `
  -KeyPath .local/lab-key `
  -User root `
  -Build
```

It expects the public hosts to already have `xray`, `openssl`, `ip`, `ping`, and
`/dev/net/tun`. It starts TapX raw TCP/TUN on loopback, wraps the public segment
with Xray VLESS over TLS using a generated one-day self-signed certificate, then
cleans TapX, Xray, generated certs, and temporary configs by default.
