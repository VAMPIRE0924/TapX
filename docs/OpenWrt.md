# OpenWrt / ImmortalWrt 安装与升级

TapX 的 OpenWrt 发布归档同时提供 APK 与 IPK。两种包面向不同包管理器，必须选择与设备固件、CPU 架构和内核 ABI 匹配的文件。

## 当前公开支持范围

当前正式 Release 提供 `tapx-openwrt-x86-64.tar.gz`，只适用于 x86-64 设备。归档中的 APK 面向 OpenWrt 25.12 系列，IPK 面向 OpenWrt 24.10 系列；ARM64、MIPS 等设备不能安装该资产。

未来 Release 若增加平台，会使用独立的 `tapx-openwrt-<platform>.tar.gz` 资产。不要把 Linux ARM64 归档当作 OpenWrt ARM64 安装包。

## 选择 APK 或 IPK

先通过 SSH 检查设备：

```sh
command -v apk || command -v opkg
uname -m
cat /etc/openwrt_release
```

| 检测结果 | 选择 |
| --- | --- |
| 存在 `/sbin/apk` 或 `apk` | 安装 `tapx-*.apk` |
| 存在 `/bin/opkg` 或 `opkg` | 安装 `tapx_*.ipk` |

OpenWrt 25.12 及更新版本使用 APK；OpenWrt 24.10 及更早版本使用 OPKG/IPK。ImmortalWrt 应以设备实际存在的包管理器为准。APK 与 IPK 不能互相改名或跨包管理器安装。

包管理器版本边界可参阅 [OpenWrt 官方软件包管理说明](https://openwrt.org/docs/guide-user/additional-software/managing_packages)。

## 安装前检查

1. 从 [Releases](https://github.com/VAMPIRE0924/TapX/releases/latest) 下载 `tapx-openwrt-x86-64.tar.gz` 及同一版本的 `SHA256SUMS`。
2. 校验归档 SHA-256，并确认文件名中的平台与设备一致。
3. 备份 `/etc/config/tapx`、TapX 数据库、证书、GeoData 和外置 Xray 文件。
4. 记录管理网卡、管理 IP、默认路由和恢复入口。
5. 确认设备有足够的 `/overlay` 空间，并且软件源与当前固件版本匹配。

## 安装 APK

```sh
tar -xzf tapx-openwrt-x86-64.tar.gz
cd tapx-openwrt-x86-64
apk add --allow-untrusted --force-reinstall ./tapx-*.apk
```

## 安装 IPK

```sh
tar -xzf tapx-openwrt-x86-64.tar.gz
cd tapx-openwrt-x86-64
opkg install --force-reinstall ./tapx_*.ipk
```

若包管理器报告依赖或内核 ABI 不匹配，应停止安装并更换与固件一致的资产或软件源，不要使用忽略依赖的强制选项。

## 打开面板

安装完成后进入 `LuCI → 服务 → TapX`，设置面板监听地址、端口、登录入口和管理员凭据，然后启动服务。TapX 管理员账号不是路由器 SSH 账号。

## 升级后检查

- `tapx-panel` 与 `tapx-core` 均处于运行状态。
- 面板、核心和内置 Xray 版本与当前 Release manifest 一致。
- 管理 IP、管理桥和默认路由未被意外改变。
- 代表性连接端诊断、终端分流、接入分流、DNS 与 GeoData 正常。
- LuCI 页面和完整 TapX Web 面板均可访问。

## 网络安全边界

- 修改桥接、TAP/TUN、策略路由或默认路由前，保留独立恢复链路。
- 不要把公网 Raw UDP/TCP 无加密模式当作安全隧道；公网应启用 TLS、DTLS 或合适的 Xray 安全传输。
- TapX 自管 DHCP 仅服务明确指定的接口，不会接管 OpenWrt 系统 `dnsmasq` 的其他 LAN 职责。
- 升级时不要清空数据库、证书或首次初始化状态。
