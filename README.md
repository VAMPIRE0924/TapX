# TapX

[![Latest Release](https://img.shields.io/github/v/release/VAMPIRE0924/TapX?display_name=tag&sort=semver)](https://github.com/VAMPIRE0924/TapX/releases/latest)
[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)
[![Linux](https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-2ea44f)](docs/安装与升级.md)
[![OpenWrt](https://img.shields.io/badge/OpenWrt-APK%20%7C%20IPK-00b5e2)](docs/OpenWrt.md)

TapX 是面向 Linux、OpenWrt 与 ImmortalWrt 的网络连接、策略分流和集中管理平台。它在同一套 Web 面板中组合网卡、TUN/TAP、监听端、连接端、用户、路由、DNS、GeoData 和多节点管理，并提供真实链路诊断与运行状态回读。

## Linux 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/install.sh | sudo bash
```

脚本会识别 `amd64` 或 `arm64`，从最新正式 Release 下载对应资产，核对 GitHub 提供的 SHA-256 摘要后启动安装向导。希望先审阅脚本时，可先下载再执行：

```bash
curl -fLO https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/install.sh
less install.sh
sudo bash install.sh
```

完整步骤、指定版本和升级说明见 [Linux 安装与升级](docs/安装与升级.md)。

## OpenWrt / ImmortalWrt

从 [最新 Release](https://github.com/VAMPIRE0924/TapX/releases/latest) 下载与设备架构匹配的 `tapx-openwrt-<platform>.tar.gz`。归档同时提供 APK 与 IPK，请按设备实际包管理器选择，不能混装。

| 系统 | 包管理器 | 安装包 | 安装命令 |
| --- | --- | --- | --- |
| OpenWrt 25.12 及更新版本 | `apk` | `tapx-*.apk` | `apk add --allow-untrusted --force-reinstall ./tapx-*.apk` |
| OpenWrt 24.10 及更早版本 | `opkg` | `tapx_*.ipk` | `opkg install --force-reinstall ./tapx_*.ipk` |
| ImmortalWrt | 以设备实际命令为准 | `.apk` 或 `.ipk` | 先运行 `command -v apk || command -v opkg` |

安装前请确认 CPU 架构、固件版本和内核 ABI，并备份配置、数据库与证书。详见 [OpenWrt 安装与升级](docs/OpenWrt.md)。

## 核心能力

- **三种运行内核**：TapX Raw FastPath、内置 Xray、外置 Xray。
- **多种传输**：Raw UDP、Raw TCP、TLS、DTLS，以及 Xray 支持的协议与传输组合。
- **真实网络设备**：系统网卡、TUN、TAP、透明桥接、静态路由、PMTU 与 MSS。
- **统一分流**：接入分流、终端分流、DNS 分流、GeoData、规则集、路由组和负载均衡。
- **访问控制**：用户、vKey、IP/MAC 范围、限速、流量额度、有效期与对象绑定。
- **链路诊断**：TCP、HTTP、真实 UDP 请求延迟、出口信息、Speedtest、日志和状态回读。
- **集中管理**：从一个面板管理本机与远程节点，并按节点保存、校验、应用和回滚配置。

Raw UDP/TCP 无加密模式适用于私网、专线或已有安全外层的网络。公网和跨境链路应使用 TLS、DTLS 或合适的 Xray 安全传输。

## 发布资产

| 资产 | 用途 |
| --- | --- |
| `tapx-linux-amd64.tar.gz` | x86-64 Linux |
| `tapx-linux-arm64.tar.gz` | ARM64 Linux |
| `tapx-openwrt-<platform>.tar.gz` | 对应 OpenWrt 平台，内含 APK 与 IPK |
| `install.sh` | Linux Release 一键安装入口 |
| `tapx-update-manifest.json` | 版本、平台、组件与 SHA-256 清单 |
| `SHA256SUMS` | Release 全部下载资产的 SHA-256 |

下载后应先执行校验，方法见 [发布资产校验](docs/发布校验.md)。

## 文档

| 文档 | 内容 |
| --- | --- |
| [Linux 安装与升级](docs/安装与升级.md) | 一键安装、指定版本、服务管理、备份和升级 |
| [OpenWrt 安装与升级](docs/OpenWrt.md) | APK/IPK 选择、LuCI、依赖、升级与恢复 |
| [工作原理](docs/工作原理.md) | 控制面、数据面、对象关系和流量处理链 |
| [面板使用说明](docs/面板说明.md) | 页面功能、保存/应用语义、诊断和多节点管理 |
| [常见故障排查](docs/故障排查.md) | 面板、服务、连接端、路由、DNS 与升级问题 |
| [发布资产校验](docs/发布校验.md) | SHA-256、manifest 与归档内容检查 |
| [安全政策](SECURITY.md) | 漏洞报告与敏感信息保护 |
| [支持说明](SUPPORT.md) | 问题反馈所需信息 |

## 许可

TapX 自有材料采用 [TapX 专有软件许可证](LICENSE)。第三方组件继续遵循其独立许可证，相关文本随 Release 资产一并分发，汇总入口见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
