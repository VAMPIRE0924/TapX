# OpenWrt 安装与升级

TapX 的 OpenWrt 发布归档同时提供 APK 与 IPK。两种包面向不同包管理器，必须选择与设备固件、CPU 架构和内核 ABI 匹配的文件。

## 当前公开支持范围

当前正式 Release 提供两个 OpenWrt 归档：

- `tapx-openwrt-x86-64.tar.gz`：OpenWrt x86-64，内含一个 APK 和一个 IPK。
- `tapx-openwrt-arm64.tar.gz`：OpenWrt ARM64，内含一个 APK 和一个 IPK。

先用 `uname -m` 确认 CPU 架构，再下载对应归档。不要跨架构安装，也不要把 Linux ARM64 归档当作 OpenWrt ARM64 安装包。

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

不要根据 OpenWrt 版本号推断包格式；同一个大版本可能使用不同的包管理器。只以设备上实际存在的命令为准。APK 与 IPK 不能互相改名或跨包管理器安装。

## 安装前检查

1. 从 [Releases](https://github.com/VAMPIRE0924/TapX/releases/latest) 下载与设备架构一致的 OpenWrt 归档及同一版本的 `SHA256SUMS`。
2. 校验归档 SHA-256，并确认文件名中的平台与设备一致。
3. 备份 `/etc/config/tapx`、TapX 数据库、证书、GeoData 和外置 Xray 文件。
4. 记录管理网卡、管理 IP、默认路由和恢复入口。
5. 确认设备有足够的 `/overlay` 空间，并且软件源与当前固件版本匹配。

## 安装 APK

```sh
archive=tapx-openwrt-arm64  # x86-64 设备改为 tapx-openwrt-x86-64
tar -xzf "$archive.tar.gz"
cd "$archive"
apk add --allow-untrusted --force-reinstall ./tapx-*.apk
```

LuCI 上传 APK 若提示 `UNTRUSTED signature`，请校验下载来源后按上述命令安装。
该错误表示包签名未被固件信任，修改文件名不能解决；本版本不改变固件的签名验证规则。

## 安装 IPK

```sh
archive=tapx-openwrt-arm64  # x86-64 设备改为 tapx-openwrt-x86-64
tar -xzf "$archive.tar.gz"
cd "$archive"
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

## 单 IP 复用客户端与 LuCI

关闭客户端桥接后，TapX 会创建 `tapx_` 开头的逻辑接口，直接绑定对应 TAP。LuCI 可以显示其地址、启停接口，并为其关联防火墙区域；不需要再创建网桥。

地址和续租由 TapX 内置 DHCP 客户端管理。保留「TapX managed address」协议，不要在同一 TAP 上同时启用 OpenWrt DHCP 客户端。默认不会接管现有 WAN 的默认路由或 DNS。

需要端口映射时，先编辑该逻辑接口的「防火墙设置」，创建或选择合适的区域；再到「网络 → 防火墙 → 端口转发」选择对应区域，填写协议、外部端口、内部地址与内部端口。TapX 不会自动开放公网服务。

首次安装此协议支持会重启一次网络服务以注册协议，网络会短暂中断。已注册协议的后续升级不需要因此重启网络。

## 网络安全边界

- 修改桥接、TAP/TUN、策略路由或默认路由前，保留独立恢复链路。
- 不要把公网 Raw UDP/TCP 无加密模式当作安全隧道；公网应启用 TLS、DTLS 或合适的 Xray 安全传输。
- TapX 自管 DHCP 仅服务明确指定的接口，不会接管 OpenWrt 系统 `dnsmasq` 的其他 LAN 职责。
- 升级时不要清空数据库、证书或首次初始化状态。
