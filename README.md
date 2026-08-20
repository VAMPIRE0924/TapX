# TapX

[![Latest Release](https://img.shields.io/github/v/release/VAMPIRE0924/TapX?display_name=tag&sort=semver)](https://github.com/VAMPIRE0924/TapX/releases/latest)
[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)
[![Linux](https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-2ea44f)](docs/安装与升级.md)
[![OpenWrt](https://img.shields.io/badge/OpenWrt-APK%20%7C%20IPK-00b5e2)](docs/OpenWrt.md)

TapX 是面向 Linux、OpenWrt 和 ImmortalWrt 的网络连接与策略分流平台。一个 Web 面板统一管理网卡、TUN/TAP、监听端、连接端、用户、路由、DNS、GeoData 和多节点，并从真实运行链路回读状态、流量、速度、延迟与诊断结果。

> 本仓库只提供正式发布资产、安装入口和用户文档，不包含 TapX 源代码。

## 快速开始

### Linux 一键安装

支持使用 `systemd` 的 Linux `amd64` 和 `arm64`：

```bash
curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/install.sh | sudo bash
```

入口脚本先读取最新正式 Release，校验其中 `install.sh` 的 GitHub SHA-256 摘要，再启动安装向导。希望先审阅脚本时：

```bash
curl -fLO https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/install.sh
less install.sh
sudo bash install.sh
```

指定版本、备份、升级和服务管理见 [Linux 安装与升级](docs/安装与升级.md)。

### OpenWrt / ImmortalWrt

当前 Release 提供 `tapx-openwrt-x86-64.tar.gz`，归档内同时包含原生 APK 和 IPK。它们是同一版本面向不同包管理器的安装包，必须按设备实际系统选择，不能改名或混装。

| 固件环境 | 包管理器 | 安装包 | 命令 |
| --- | --- | --- | --- |
| OpenWrt 25.12 及更新版本 | `apk` | `tapx-*.apk` | `apk add --allow-untrusted --force-reinstall ./tapx-*.apk` |
| OpenWrt 24.10 及更早版本 | `opkg` | `tapx_*.ipk` | `opkg install --force-reinstall ./tapx_*.ipk` |
| ImmortalWrt | 以设备实际命令为准 | `.apk` 或 `.ipk` | 先检查 `command -v apk`，否则检查 `command -v opkg` |

当前公开 OpenWrt 资产仅适用于 x86-64；ARM64、MIPS 等设备不要安装该归档。完整检查与安装步骤见 [OpenWrt / ImmortalWrt 安装与升级](docs/OpenWrt.md)。

## 能力概览

| 范围 | 能力 |
| --- | --- |
| 运行内核 | TapX Raw FastPath、内置 Xray、外置 Xray |
| 连接与设备 | Raw UDP、Raw TCP、TLS、DTLS、Xray 协议，物理网卡、TUN、TAP 和透明桥接 |
| 策略分流 | 接入分流、终端分流、DNS 分流、GeoData、规则集、路由组和负载均衡 |
| 访问控制 | 用户、vKey、IP/MAC 范围、有效期、流量额度和速率限制 |
| 诊断观测 | TCP、HTTP、真实 UDP 请求延迟、出口信息、Speedtest、流量、速度、日志和状态回读 |
| 集中管理 | 本机与远程节点、配置校验、运行应用、失败回滚和链路查询 |

TapX 保留有意义的低层参数，不把复杂网络能力压缩成固定套餐。网卡、监听端、连接端、用户、通道策略、路由、vKey、设备和地址限制均可独立配置并自由组合，保存或应用时再验证组合是否可运行。

## 工作方式

```text
入口设备 / 监听端
        ↓
用户、vKey、IP/MAC、额度与限速
        ↓
接入分流 / 终端分流 / DNS / GeoData
        ↓
路由组、负载均衡与最终出口
        ↓
TapX Raw / 内置 Xray / 外置 Xray
        ↓
运行状态、流量、速度、延迟与诊断回读
```

Go 控制面负责 Web/API、对象校验、数据库、运行配置和生命周期；Raw UDP/TCP 与 TAP/TUN 的高频数据路径由 C FastPath 处理。进一步说明见 [TapX 工作原理](docs/工作原理.md)。

## 安全边界

- Raw UDP/TCP 无加密模式用于私网、专线或已经具备安全外层的网络；公网和跨境链路应使用 TLS、DTLS 或合适的 Xray 安全传输。
- 修改桥接、TAP/TUN、策略路由或默认路由前，应保留独立管理与恢复链路。
- 面板管理员账号与 Linux/OpenWrt 的 SSH 账号彼此独立；不要公开提交密码、私钥、vKey、未脱敏配置、日志或抓包。
- 下载后应按 [发布资产校验](docs/发布校验.md) 核对 SHA-256、manifest 和归档内容。

## Release 资产

| 资产 | 用途 |
| --- | --- |
| `tapx-linux-amd64.tar.gz` | x86-64 Linux |
| `tapx-linux-arm64.tar.gz` | ARM64 Linux |
| `tapx-openwrt-x86-64.tar.gz` | x86-64 OpenWrt，内含 APK 与 IPK |
| `install.sh` | Linux Release 安装器 |
| `tapx-update-manifest.json` | 版本、平台、组件和 SHA-256 清单 |
| `SHA256SUMS` | 本 Release 下载资产的 SHA-256 清单 |

所有资产以 [最新正式 Release](https://github.com/VAMPIRE0924/TapX/releases/latest) 页面为准。不同版本的资产、manifest 和校验文件不能混用。

## 文档

| 文档 | 内容 |
| --- | --- |
| [文档中心](docs/README.md) | 按安装、使用、原理、安全和排障分类浏览 |
| [Linux 安装与升级](docs/安装与升级.md) | 一键安装、指定版本、服务管理、备份和升级 |
| [OpenWrt 安装与升级](docs/OpenWrt.md) | 平台限制、APK/IPK 选择、LuCI、升级与恢复 |
| [工作原理](docs/工作原理.md) | 控制面、数据面、对象关系和流量处理链 |
| [面板使用说明](docs/面板说明.md) | 页面功能、保存/应用语义、诊断和多节点管理 |
| [常见故障排查](docs/故障排查.md) | 面板、服务、连接端、路由、DNS 和升级问题 |
| [发布资产校验](docs/发布校验.md) | SHA-256、manifest 和归档内容检查 |
| [安全政策](SECURITY.md) | 漏洞报告与敏感信息保护 |
| [支持说明](SUPPORT.md) | 问题反馈前的检查和所需信息 |

## 许可

TapX 自有材料采用 [TapX 专有软件许可证](LICENSE)。第三方组件继续遵循各自许可证，相关许可文本随 Release 资产分发，汇总入口见 [第三方声明](THIRD_PARTY_NOTICES.md)。
