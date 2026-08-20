# TapX

[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)

TapX 是面向 Linux 与 OpenWrt 的网络连接、分流与管理工具。它将监听端、连接端、用户、设备、路由、DNS 与 GeoData 组合在同一套 Web 面板和运行时中，用于构建可观测、可诊断的网络转发方案。

> 本仓库是 TapX 的公开发布仓库，只保存产品说明、安装文档、发布工作流和 Release 资产，不保存源码、开发日志或实验记录。

## 核心能力

- Raw UDP、Raw TCP、内置 Xray 和外置 Xray 运行内核。
- TUN/TAP、终端分流、接入分流、DNS 分流与 GeoData 规则。
- 连接端诊断、出口信息、延迟检测与 Speedtest。
- 用户、vKey、路由、地址限制与设备的组合管理。
- Linux amd64/arm64 与 OpenWrt 发布资产。

## 安装

从 [Releases](https://github.com/VAMPIRE0924/TapX/releases) 选择与平台匹配的资产。每个新版本应同时提供 `SHA256SUMS` 和 `tapx-update-manifest.json`，请先校验再安装。

- [Linux 安装与升级](docs/安装与升级.md)
- [OpenWrt 安装与升级](docs/OpenWrt.md)
- [发布资产校验](docs/发布校验.md)

## 文档

- [工作原理](docs/工作原理.md)
- [面板与对象关系](docs/面板说明.md)
- [常见故障排查](docs/故障排查.md)
- [安全政策](SECURITY.md)
- [支持说明](SUPPORT.md)

## 发布与许可

发布资产由独立的私有源码仓库完成构建、测试和校验，然后上传到本仓库的草稿 Release，经确认后才发布。TapX 自有材料采用[专有闭源许可](LICENSE)；第三方组件继续遵循 Release 资产内附带的独立许可证和声明。
