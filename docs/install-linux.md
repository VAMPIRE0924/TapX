# Linux Install

Linux is the primary local runtime target for the first TapX builds. The
service runs `tapx-panel`, which owns the Web/API control plane and starts local
runtime workers through the Go supervisor.

## Build

```bash
./scripts/build/linux-amd64.sh
./scripts/build/linux-arm64.sh
```

Outputs:

```text
build/linux-amd64/tapx-core
build/linux-amd64/tapx-panel
build/linux-arm64/tapx-core
build/linux-arm64/tapx-panel
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/scripts/install/install.sh | sudo bash
```

The installer copies:

- `tapx-core` and `tapx-panel` to `/usr/local/bin`.
- The environment file to `/etc/tapx/tapx.env`.
- The systemd unit to `/etc/systemd/system/tapx-panel.service`.

The first prompt selects English or Chinese. The wizard then selects SQLite or
PostgreSQL, panel port, secret URI path, administrator credentials, and an
optional certificate/private-key pair. Empty port, URI path, username, and
password values are generated randomly. PostgreSQL requires a
`postgres://` or `postgresql://` URL DSN.

The same script is the `tapx` management command after installation. Run
`sudo tapx` for its menu or `sudo tapx set-panel` to change the database and
panel parameters. Existing data is not migrated merely by changing the DSN;
export a `.db` backup first and restore it after switching backend.

## Defaults

The default `/etc/tapx/tapx.env` is:

```text
TAPX_DB_DRIVER=sqlite
TAPX_DB_SOURCE=/var/lib/tapx/tapx.db
TAPX_PANEL_LISTEN=0.0.0.0:<selected-port>
TAPX_PANEL_BASE_PATH=/tapx-<random>
TAPX_PANEL_HTTPS=0
```

PostgreSQL uses:

```text
TAPX_DB_DRIVER=postgres
TAPX_DB_SOURCE=postgres://tapx:password@127.0.0.1:5432/tapx?sslmode=require
```

The environment file is mode `0600`. The service reads the DSN from that file
instead of placing a database password in the process command line.

For unattended installation, set `TAPX_NONINTERACTIVE=1` together with
`TAPX_DB_DRIVER`, `TAPX_DB_SOURCE`, `TAPX_PANEL_HOST`, `TAPX_PANEL_PORT`,
`TAPX_PANEL_BASE_PATH`, `TAPX_ADMIN_USERNAME`, and `TAPX_ADMIN_PASSWORD`.

The installed panel listens on all interfaces. When UFW or firewalld is active,
the installer opens the selected TCP port. Other provider or host firewalls
must allow that port separately. The systemd unit passes
`TAPX_PANEL_BASE_PATH` to `tapx-panel -base-path`, so UI and API endpoints are
served under the selected path.

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
tapx-panel -db-driver sqlite -db /var/lib/tapx/tapx.db -check
TAPX_DB_DRIVER=postgres \
  TAPX_DB_SOURCE='postgres://tapx:password@127.0.0.1:5432/tapx?sslmode=require' \
  tapx-panel -check
tapx-core -config docs/examples/raw-udp-tun.json -check
tapx-core -config docs/examples/xray-external-listener.json -check
```
