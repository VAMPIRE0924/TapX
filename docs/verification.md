# Verification

TapX has two verification layers:

- local structural checks that can run on Windows, Linux, or CI,
- Linux/network checks that require WSL/Linux with TAP/TUN and network
  namespace permissions.

## Local Structural Checks

```bash
make verify-local
```

This runs `go run ./scripts/verify` and checks:

- required source/package files exist,
- JSON files under `docs/` and `openwrt/` parse,
- example runtime configs generate successfully,
- OpenWrt `.ipk` package structure when packages are present,
- known private lab markers are absent outside ignored paths.

For release artifacts:

```bash
make package-openwrt-x86
make verify-release
```

`verify-release` requires both OpenWrt packages to exist and verifies ar
members, control metadata, conffiles, and installed data paths.

## Linux Runtime Checks

Run these in WSL/Linux:

```bash
make test
make integration-netns
make integration-address-guard-netns
make build-linux-amd64
make build-openwrt-x86
```

`make integration-netns` requires root or CAP_NET_ADMIN. It verifies the raw
UDP/TCP TUN/TAP paths, vKey path, Address Guard allow/drop behavior, and
tapx-net apply/rollback slices.

For installer validation without touching real `/etc/tapx`, install into a
temporary root and force the database/unit paths into that root:

```bash
tmp="$(mktemp -d /tmp/tapx-install-test-XXXXXX)"
./scripts/install/linux-install.sh \
  --build-dir "$(pwd)/build/linux-amd64" \
  --prefix "$tmp/prefix" \
  --sysconfdir "$tmp/etc" \
  --unit-dir "$tmp/systemd" \
  --db "$tmp/state/tapx.db" \
  --listen 127.0.0.1:18080 \
  --base-path /tapx-test \
  --admin-username admin \
  --admin-password testpass
"$tmp/prefix/bin/tapx-panel" -db "$tmp/state/tapx.db" -check
rm -rf "$tmp"
```

## Public Lab Checks

Public servers are validation targets only. Keep host notes, temporary SSH
keys, and generated files under `.local/`, which is ignored by git. The local
machine must not open naked raw UDP/TCP data-plane sessions to foreign public
servers. The lab scripts below use local SSH only; raw data-plane traffic runs
between remote validation servers.

The current lab helper expects SSH key auth and cleans its remote `/tmp`
workspace by default:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\build-linux-amd64.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\raw-transport-smoke.ps1 `
  -HostA <remote-host-a> -HostB <remote-host-b> `
  -KeyPath .local\lab_ed25519 -Mode all
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\raw-transport-benchmark.ps1 `
  -HostA <remote-host-a> -HostB <remote-host-b> `
  -KeyPath .local\lab_ed25519 `
  -Tool auto -Duration 30 -Parallel 4 -Mode both -Traffic tcp
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\xray-embedded-smoke.ps1 `
  -HostName <public-host-a>,<public-host-b> -KeyPath .local\lab_ed25519
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\xray-frame-tun-smoke.ps1 `
  -HostA <public-host-a> -HostB <public-host-b> -KeyPath .local\lab_ed25519 -Mode both
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lab\xray-wrapped-raw-tcp-smoke.ps1 `
  -HostA <public-host-a> -HostB <public-host-b> -KeyPath .local\lab_ed25519
```

Do not commit server credentials, generated keys, or lab host notes.
