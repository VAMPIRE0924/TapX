# Release

Public GitHub releases target `VAMPIRE0924/TapX` and are expected to publish a
clean source tree plus generated artifacts. Do not include `.local/`, host
notes, credentials, generated SSH keys, or private lab data.

## Local Preflight

Run these before tagging:

```bash
make test
make verify-local
make build-linux-amd64
make build-linux-arm64
make package-openwrt-x86
make verify-release
```

On Linux/WSL with network permissions:

```bash
make integration-netns
```

Public-server validation is separate and must clean remote `/tmp/tapx-lab-*`
workspaces before release.

## GitHub Actions

`.github/workflows/ci.yml` runs on push and pull request:

- repository structural verification,
- Go tests and vet,
- frontend syntax check,
- C fastpath tests,
- Linux network namespace integration.

`.github/workflows/release.yml` runs on tag push `v*` or manual dispatch. The
initial release artifact set is intentionally small and x86-focused:

- `tapx-linux-amd64.tar.gz`, containing `tapx-core`, `tapx-panel`, and the installer
- `tapx-linux-arm64.tar.gz`, containing the same Linux bundle for arm64
- `tapx-openwrt-x86-64.tar.gz`, containing the native IPK or APK `tapx-core`, `tapx-panel`, `luci-app-tapx` packages and installer
- `SHA256SUMS`
- `tapx-update-manifest.json`, describing compatible panel, TapX Core, embedded Xray, platform assets, and SHA256 values for the panel updater

Do not add MT7986 or other architecture artifacts to the initial release.

Tag example:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Manual release dispatch accepts a package version and optional OpenWrt SDK
version override.

## Update Compatibility

TapX Core and the embedded, unmodified Xray module are one same-process data
plane and are upgraded together. The panel must never install an arbitrary
embedded Xray binary independently from the TapX Core build that was tested
against it. The release manifest records both versions and maps them to one
compatible platform archive.

TapX-UI uses the same release manifest and platform archive so panel/core ABI
and database compatibility can be checked before installation. External Xray
remains independently replaceable from official `XTLS/Xray-core` release ZIPs.
Update checks and downloads are explicit control-plane operations and do not
run in the packet path.

The first automatic compatible-bundle installer targets Linux amd64. It
downloads the manifest and archive, verifies SHA256 and both binary versions,
atomically replaces `tapx-panel` and `tapx-core` with rollback backups, then
requests a supervisor restart. OpenWrt publishes one combined native-package
archive. Package and firmware upgrades preserve `/etc/config/tapx` and
`/etc/tapx/tapx.db`; certificates are excluded. Automatic in-panel installation remains
disabled until both `apk` and `opkg` paths have transactional rollback coverage.
