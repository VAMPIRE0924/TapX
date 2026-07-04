# TapX

TapX is a TAP/TUN networking tool with an advanced Web panel.

It supports raw UDP, raw TCP, TLS/DTLS, Xray transport, real Linux TUN/TAP interfaces, routing, address limits, traffic stats, logs, and OpenWrt management.

Raw UDP and Raw TCP can run without encryption or authentication when your network is already private, dedicated, or protected by another tunnel. You can also combine vKey, Client binding, Route binding, address limits, TLS/DTLS, or Xray transport from the same panel.

## Linux One-Click Install

```bash
curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/scripts/install/install.sh | sudo bash
```

The installer downloads the latest Linux package, installs `tapx-core` and `tapx-panel`, creates the systemd service, starts the panel, and prints the panel URL plus the first admin password.

Default listen address:

```text
127.0.0.1:8080
```

Install with a public listen address:

```bash
curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/scripts/install/install.sh -o /tmp/tapx-install.sh
sudo TAPX_PANEL_LISTEN=0.0.0.0:8080 bash /tmp/tapx-install.sh
```

Install a specific release:

```bash
sudo TAPX_VERSION=v0.1.0 bash /tmp/tapx-install.sh
```

Service commands:

```bash
sudo systemctl status tapx-panel
sudo systemctl restart tapx-panel
sudo journalctl -u tapx-panel -f
```

Installed paths:

```text
/usr/local/bin/tapx-core
/usr/local/bin/tapx-panel
/etc/tapx/tapx.env
/var/lib/tapx/tapx.db
```

## OpenWrt x86-64

Download `tapx-openwrt-x86-64.tar.gz` from the latest release, then install:

```bash
tar -xzf tapx-openwrt-x86-64.tar.gz
cd tapx-openwrt-x86-64
opkg install ./tapx-core_*.ipk ./luci-app-tapx_*.ipk
/etc/init.d/tapx enable
/etc/init.d/tapx start
```

Open LuCI:

```text
Services -> TapX
```

The first OpenWrt release target is x86-64.

## Release Files

```text
tapx-linux-amd64.tar.gz       Linux amd64 package with tapx-core and tapx-panel
tapx-openwrt-x86-64.tar.gz    OpenWrt x86-64 bundle with core and LuCI IPK
SHA256SUMS                    Checksums
```

Latest release:

```text
https://github.com/VAMPIRE0924/TapX/releases/latest
```

## Basic Usage

1. Install TapX.
2. Open the panel URL printed by the installer.
3. Create or import Devices, Listeners, Connectors, Clients, Routes, vKeys, and Address Limits.
4. Save the configuration.
5. Apply runtime from the panel.
6. Check the created interface:

```bash
ip a
ip -d addr
```

TapX creates real Linux TUN/TAP devices, so nftables, routes, bridges, and normal Linux networking tools can use those interfaces.

## Examples

Example runtime configs are kept under:

```text
docs/examples/
```

Check a config:

```bash
tapx-core -config docs/examples/raw-udp-tun.json -check
```

## License

GPL-2.0
