# TapX

TapX is a TAP/TUN networking tool with an advanced Web management panel.

It supports raw UDP, raw TCP, TLS/DTLS, embedded or external Xray transport, real Linux TUN/TAP interfaces, routing, address limits, traffic limits, centralized node management, logs, backup, and OpenWrt management.

Raw UDP and Raw TCP can run without encryption or authentication on private networks, dedicated lines, or an existing outer tunnel. vKey, user binding, link binding, IP/MAC limits, TLS/DTLS, and Xray can be enabled independently when required.

## Linux Install

```bash
curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/scripts/install/install.sh | sudo bash
```

The installer automatically detects Linux amd64 or arm64 and verifies the downloaded Release package. The setup wizard supports English and Chinese and configures:

- SQLite or PostgreSQL
- Public panel port
- Panel URI path
- Administrator username and password
- Optional panel certificate and private key

Empty port, URI path, username, and password inputs generate random values. The installed panel listens on all network interfaces. When UFW or firewalld is active, the selected TCP port is opened automatically.

Run the interactive manager after installation:

```bash
sudo tapx
```

Common commands:

```bash
sudo tapx status
sudo tapx restart
sudo tapx settings
sudo tapx set-panel
sudo tapx set-auth
sudo tapx set-database
sudo tapx logs
sudo tapx update
```

## OpenWrt x86-64

Download `tapx-openwrt-x86-64.tar.gz` from the latest Release and run:

```bash
tar -xzf tapx-openwrt-x86-64.tar.gz
cd tapx-openwrt-x86-64
./install.sh
```

The package installs TapX Core, TapX-UI, and the minimal LuCI management page. Full object and runtime configuration remains available in TapX-UI.

OpenWrt upgrades preserve `/etc/config/tapx` and `/etc/tapx/tapx.db`. Configuration export contains only these panel settings and database files; certificate files are excluded.

## Basic Usage

1. Install TapX and open the generated panel URL.
2. Create TUN or TAP devices.
3. Add listening endpoints, users, connecting endpoints, and link bindings.
4. Select TapX, embedded Xray, or external Xray runtime mode.
5. Save and apply the configuration.

TAP mode carries Ethernet frames and supports layer-2 networking. TUN mode carries IP packets and supports layer-3 networking.

## Build

Requirements: Go 1.26, Node.js 22 or newer, GCC, and Make.

```bash
make panel-web
make test
make verify-local
make build-linux-amd64
make build-linux-arm64
```

OpenWrt x86-64 packages require the OpenWrt SDK:

```bash
make package-openwrt-x86
```

## Release Files

```text
tapx-linux-amd64.tar.gz
tapx-linux-arm64.tar.gz
tapx-openwrt-x86-64.tar.gz
tapx-update-manifest.json
SHA256SUMS
```

See the [latest Release](https://github.com/VAMPIRE0924/TapX/releases/latest).

## License

[MPL-2.0](LICENSE)
