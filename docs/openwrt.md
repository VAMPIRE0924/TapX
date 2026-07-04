# OpenWrt

Current OpenWrt development is x86-64 only. MT7986 and other architectures are
deferred until that platform work is explicitly started.

## Build x86-64 Binary

```bash
./scripts/dev/prepare-openwrt-sdk.sh x86-64
make build-openwrt-x86
```

Output:

```text
build/openwrt-x86-64/tapx-core
```

## Build x86-64 Packages

```bash
make package-openwrt-x86
```

Outputs:

```text
build/openwrt-x86-64/packages/tapx-core_0.0.0-dev_x86_64.ipk
build/openwrt-x86-64/packages/luci-app-tapx_0.0.0-dev_all.ipk
```

Override the package version with:

```bash
TAPX_VERSION=0.1.0 make package-openwrt-x86
```

## tapx-core Package

The package installs:

- `/usr/bin/tapx-core`
- `/etc/config/tapx`
- `/etc/init.d/tapx`
- `/etc/tapx/runtime.json.example`

The service is disabled by default:

```text
config tapx 'core'
	option enabled '0'
	option config_path '/etc/tapx/runtime.json'
```

Enable it only after writing a valid runtime config:

```bash
cp /etc/tapx/runtime.json.example /etc/tapx/runtime.json
uci set tapx.core.enabled='1'
uci commit tapx
/etc/init.d/tapx enable
/etc/init.d/tapx start
```

## LuCI

`luci-app-tapx` adds a Services / TapX page for:

- enabling or disabling the procd service,
- selecting the runtime JSON path,
- editing Device, Listener, Connector, Client, Route, vKey, AddressLimit,
  XrayProfile, and Settings objects through field-level templates,
- editing Client traffic cap, reset timestamp, and RX/TX reset offsets,
- editing AddressLimit MAC/IP guard fields plus gateway, DNS, pushed routes,
  and default-route permission for import/static address assignment,
- appending or replacing those objects in the runtime JSON,
- editing `/etc/tapx/runtime.json`,
- checking the saved runtime with `tapx-core -check`,
- showing procd service status,
- reloading the TapX service after a saved config change.

This is still smaller than the full Linux/Web panel, but it exposes the same
core object families and low-level fields instead of fixed presets. Full JSON
editing remains available for advanced or newly-added fields.
