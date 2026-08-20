# TapX 第三方组件说明

TapX Release 包含若干独立的第三方软件和 Web 运行依赖。TapX 专有许可证仅适用于 TapX 自有材料，不替代、不收窄第三方组件直接授予的权利，也不移除其版权和分发义务。

## 随 Release 提供的清单

每个 Linux 与 OpenWrt 归档均包含：

- `GO_THIRD_PARTY_LICENSES.txt`：TapX 二进制使用的 Go 模块、版本、来源和完整许可文本；
- `WEB_THIRD_PARTY_LICENSES.txt`：Web 面板运行依赖、版本、来源和完整许可文本；
- `THIRD_PARTY_NOTICES.md`：第三方组件边界和许可文件入口；
- `LICENSE`：仅适用于 TapX 自有材料的专有许可证。

## 主要组件

- 内置 Xray 使用未经修改的官方 `github.com/xtls/xray-core`，适用 MPL-2.0；
- Web 面板使用 React、Ant Design、Day.js、QR Code for React、uPlot 及其依赖；
- Go 依赖图包含 `github.com/juju/ratelimit`，适用 LGPL-3.0 及其链接例外。

准确的模块版本、版权声明和许可正文以当前 Release 归档内的两份完整清单为准。
