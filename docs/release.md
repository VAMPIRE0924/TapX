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

- `tapx-linux-amd64-<version>.tar.gz`
- `tapx-core_<version>_x86_64.ipk`
- `luci-app-tapx_<version>_all.ipk`
- `SHA256SUMS`

Do not add MT7986 or other architecture artifacts to the initial release.

Tag example:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Manual release dispatch accepts a package version and optional OpenWrt SDK
version override.
