# OpenWrt

当前只构建 x86-64。OpenWrt 复用 Linux 的完整 TapX-UI，LuCI 仅负责首次设置、服务控制、开机启动和日志。

## 构建

```bash
./scripts/dev/prepare-openwrt-sdk.sh x86-64
make package-openwrt-x86
```

构建格式由 SDK 决定：OpenWrt 25.12 及以后生成 APK，旧版 SDK 生成 IPK。输出位于：

```text
build/openwrt-x86-64/packages/
```

## 安装

将同一构建中的三个包和 `install.sh` 放在同一目录，以 root 运行：

```sh
./install.sh
```

脚本自动识别 `apk` 或 `opkg`，并由系统仓库安装 `kmod-tun`、`ip-full`、`tc-full`、iptables nft 后端、CA 证书和 LuCI 依赖。

安装后服务保持关闭。进入 `LuCI -> 服务 -> TapX`，设置监听网卡、面板端口、登录入口、用户名和密码后才能启动。

## 备份与升级

- `/etc/config/tapx` 是包配置文件，包升级不会覆盖用户设置。
- LuCI 可导出和恢复 OpenWrt 配置包，包内固定仅含 `/etc/config/tapx` 与一致性快照 `/etc/tapx/tapx.db`，不包含证书或其他文件。
- `/lib/upgrade/keep.d/tapx` 只登记上述 UCI 和 DB 文件，`sysupgrade` 固件升级时一并保留。
- `/etc/tapx/runtime.json` 是派生文件，不进入备份；核心启动前由 Go 控制面从 DB 校验并重新生成，避免升级或恢复后使用旧运行配置。
- 普通 IPK/APK 升级不会删除未由软件包覆盖的 DB，也不会重置首次初始化状态。
- LuCI 不保存用户名或密码；密码在浏览器中生成 PBKDF2 哈希后写入 SQLite，初始化完成后不回显凭据。

LuCI 的“重置到固件”与设备恢复出厂后的规则一致：只有将 TapX 软件包、`/etc/config/tapx` 和预置 `/etc/tapx/tapx.db` 一并编入固件时，才会恢复固件内的预置配置。固件没有预置 DB 时回到未初始化状态；运行后产生的 overlay 数据不会跨出厂重置保留。

## LuCI 与完整面板

LuCI 只提供：

- TapX 核心和 TapX-UI 的启动、重启、关闭、状态与独立开机启动；
- 从系统网卡列表选择面板监听网卡；
- 面板端口可用性检查、登录入口和登录凭据初始化；
- 仅含 UCI 与 DB 的配置导出、恢复和重置到固件；
- 近期日志与完整 TapX-UI 入口。

设备、监听端、用户、连接端、链路绑定、内核和高级配置继续由完整 TapX-UI 管理，和 Linux 保持一致。
