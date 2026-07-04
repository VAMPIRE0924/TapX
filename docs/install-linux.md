# Linux Install

Linux is the primary local runtime target for the first TapX builds. The
service runs `tapx-panel`, which owns the Web/API control plane and starts local
runtime workers through the Go supervisor.

## Build

```bash
./scripts/build/linux-amd64.sh
```

Outputs:

```text
build/linux-amd64/tapx-core
build/linux-amd64/tapx-panel
```

## Install

```bash
sudo ./scripts/install/linux-install.sh
```

The installer copies:

- `tapx-core` and `tapx-panel` to `/usr/local/bin`.
- The environment file to `/etc/tapx/tapx.env`.
- The systemd unit to `/etc/systemd/system/tapx-panel.service`.

It reloads systemd but does not enable or start the service unless requested:

```bash
sudo ./scripts/install/linux-install.sh --enable --start
```

On first install, the script initializes panel login in the SQLite database,
generates a random Web base path, and prints the URL plus admin credentials.
Override those values when needed:

```bash
sudo ./scripts/install/linux-install.sh \
  --admin-username admin \
  --admin-password 'change-this' \
  --base-path /tapx-private
```

Packaging or test environments can also override install paths:

```bash
sudo ./scripts/install/linux-install.sh \
  --prefix /opt/tapx \
  --sysconfdir /etc/tapx \
  --unit-dir /etc/systemd/system \
  --db /var/lib/tapx/tapx.db \
  --listen 127.0.0.1:8080
```

If `/etc/tapx/tapx.env` already exists, the installer preserves the existing
panel configuration and writes `/etc/tapx/tapx.env.example` instead.

## Defaults

`/etc/tapx/tapx.env` starts with:

```text
TAPX_DB_PATH=/var/lib/tapx/tapx.db
TAPX_PANEL_LISTEN=127.0.0.1:8080
TAPX_PANEL_BASE_PATH=/tapx-<random>
```

The panel listens on localhost by default. Expose it intentionally through a
reverse proxy, firewall rule, VPN, or by changing `TAPX_PANEL_LISTEN`. The
systemd unit passes `TAPX_PANEL_BASE_PATH` to `tapx-panel -base-path`, so UI and
API endpoints are served under that path.

If the enabled Settings object has `PanelHTTPS=true`, `PanelCertFile`, and
`PanelKeyFile`, `tapx-panel` starts its listener with TLS after restart. The
`-listen` flag or `TAPX_PANEL_LISTEN` still selects the socket address; when no
listen override is supplied, `Settings.PanelListen` is used.

## Service

```bash
sudo systemctl status tapx-panel
sudo systemctl restart tapx-panel
sudo journalctl -u tapx-panel -f
```

The first service runs as root because TapX creates TAP/TUN devices, applies
addresses, routes, bridge settings, MSS clamp rules, and raw sockets. A later
hardening pass can split privileges after the network apply layer stabilizes.

## Check

```bash
tapx-panel -db /var/lib/tapx/tapx.db -check
tapx-core -config docs/examples/raw-udp-tun.json -check
tapx-core -config docs/examples/xray-external-listener.json -check
```
