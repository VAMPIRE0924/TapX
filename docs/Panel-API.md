# TapX Panel API

TapX Panel API 与 Web 面板使用同一地址和基础路径。面板部署在 `/tapx/` 等非根路径时，本文中的 `/api/...` 均位于该基础路径下。

API 用于面板自身、自动化运维和受控的多节点管理。写入配置前应先读取当前对象与配置修订，不要用不完整文档覆盖未知字段。

## 认证

以下端点无需登录：

```text
GET  /api/health
GET  /api/auth/session
POST /api/auth/login
POST /api/auth/logout
```

其他 `/api/` 端点需要以下任一认证方式：

- Web 会话：登录后由浏览器携带 HttpOnly、SameSite Cookie；非只读请求必须保持同源。
- API Token：在“面板设置 → 安全 → API Token”创建，通过 `Authorization: Bearer <token>` 发送。Token 只在创建时完整显示一次，可单独设置失效时间和撤销。

示例：

```bash
export TAPX_URL='https://panel.example.com/tapx'
export TAPX_API_TOKEN='replace-with-your-token'
curl -fsS -H "Authorization: Bearer ${TAPX_API_TOKEN}" \
  "${TAPX_URL}/api/runtime/state"
```

不要把 Token 写入仓库、命令历史、Issue、日志或截图。

## 通用约定

- JSON 请求使用 `Content-Type: application/json`。
- 错误响应保留对应 HTTP 状态码，并在 JSON 中返回可显示的具体原因。
- `400` 表示字段、组合或请求格式无效；`401` 表示未认证；`403` 表示同源或安全策略拒绝；`404` 表示对象不存在；`409` 表示配置修订冲突或互斥操作正在执行。
- `GET /api/config` 返回强 ETag。自动化写回完整配置时应发送相同的 `If-Match`；配置已被其他操作修改时，服务端拒绝覆盖，调用方应重新读取并合并。
- 诊断和测速在流式模式下返回 `text/event-stream`。客户端应持续读取事件、支持取消，并以最终完成或错误事件作为结论，不能仅使用中间样本。
- 托管节点接口由本地 Panel 代理。浏览器不直接连接远端节点，远端 Token 也不会在节点列表响应中返回。

## 配置对象

```text
GET    /api/config
PUT    /api/config
POST   /api/config/validate?mode=save|apply
GET    /api/objects/{kind}
GET    /api/objects/{kind}/{id}
PUT    /api/objects/{kind}/{id}
DELETE /api/objects/{kind}/{id}
```

对象 `kind` 包括：

```text
devices                 listeners
connectors              clients
routes                   vkeys
addresses                xrayProfiles
xrayRoutingContexts      xrayRouteGroups
xrayRouteRules           xrayRuleSets
xrayGlobalRoutings       tapxDNSConfigs
settings
```

单对象 `PUT` 的路径 ID 与请求体 ID 必须一致；请求体省略 ID 时使用路径 ID。`mode=save` 检查字段、结构和对象引用，`mode=apply` 还检查当前主机、设备、端口、路由、DNS、防火墙和运行内核能力。

## 运行时与组件

```text
GET    /api/runtime
POST   /api/runtime
GET    /api/runtime/state
POST   /api/runtime/apply
POST   /api/runtime/reload-routing
POST   /api/runtime/enforce
POST   /api/runtime/stop
POST   /api/runtime/components/{tapx|embedded-xray|external-xray}/{start|stop|restart}
```

- `GET /api/runtime` 返回由当前存储对象生成的运行配置。
- `apply` 生成并应用候选配置，失败时返回真实阶段与原因，并保留或恢复上一份可用配置。
- `reload-routing` 重新载入已保存的接入分流、终端分流和 DNS 配置，不保存草稿，也不等同于完整重启。
- 三个运行组件分别启停；一个组件的操作不会被伪装成全部内核状态。

## 状态、统计与日志

```text
GET    /api/dashboard
GET    /api/dashboard/history
GET    /api/stats
GET    /api/server/interfaces
GET    /api/diagnostics
GET    /api/logs
DELETE /api/logs
```

Dashboard 提供系统、进程、运行时、对象计数、实时速率和持久化历史样本。Stats 按传输、端点、设备、路由和用户聚合运行计数器。日志包含结构化时间、等级、组件和事件字段。

## 分享、流量与连接端诊断

```text
GET    /api/templates/raw-pair
GET    /api/share/clients/{id}
POST   /api/clients/{id}/traffic/reset
POST   /api/connectors/{id}/traffic/reset
POST   /api/listeners/{id}/traffic/reset
POST   /api/connectors/test
POST   /api/connectors/proxy-diagnostics
GET    /api/connectors/{id}/speedtest/targets
GET    /api/connectors/{id}/speedtest/results
POST   /api/connectors/{id}/speedtest
```

`/api/connectors/test` 用于 TapX 设备通道诊断，按能力执行通道状态、路径 MTU 或通道吞吐。`/api/connectors/proxy-diagnostics` 用于代理出站，TapX、内置 Xray 和外置 Xray 使用一致结构执行真实 TCP/UDP 请求、出口信息和 Speedtest；代理出站结果不会混入设备通道诊断。

流量重置只建立新的统计偏移，不清空底层累计计数器，也不影响其他对象。

## Xray、GeoData 与路由查询

```text
GET    /api/xray/external/status
POST   /api/xray/external/upload
POST   /api/xray/external/download
GET    /api/xray/vless-encryption
GET    /api/xray/reality/x25519
GET    /api/xray/reality/mldsa65
POST   /api/xray/reality/scan
POST   /api/xray/reality/scan-many
POST   /api/xray/tls/ech
POST   /api/xray/tls/cert-hash
POST   /api/xray/tls/remote-cert-hash
GET    /api/geodata/status
POST   /api/geodata/update
GET    /api/xray/route-groups/{id}/export
POST   /api/xray/route-groups/import
POST   /api/xray/routes/query
```

外置 Xray 候选文件必须通过版本与可执行性检查后才会替换当前文件。GeoData 更新会校验文件并成对切换；路由组导入、导出和链路查询不会绕过对象引用与配置验证。

## 更新、备份与集成

```text
GET    /api/updates/{component}
POST   /api/updates/{component}
GET    /api/backup
POST   /api/backup/restore
ANY    /api/integrations/warp/{action}
ANY    /api/integrations/nord/{action}
```

备份导出为可移植 TapX SQLite 数据库。恢复前会检查文件头、数据库完整性、TapX schema 和已存配置；恢复成功后旧会话失效，并重新应用运行配置。包含访问令牌或私钥的集成响应只能在经过认证的对应操作中读取。

## 托管节点

```text
GET    /api/nodes
POST   /api/nodes/test
PUT    /api/nodes/{id}
DELETE /api/nodes/{id}
POST   /api/nodes/{id}/test
GET|PUT /api/nodes/{id}/config
POST   /api/nodes/{id}/runtime/apply
POST   /api/nodes/{id}/runtime/reload-routing
POST   /api/nodes/{id}/update
GET    /api/nodes/{id}/geodata/status
POST   /api/nodes/{id}/geodata/update
GET    /api/nodes/{id}/stats
GET    /api/nodes/{id}/system/interfaces
GET    /api/nodes/{id}/share/clients/{objectID}
GET    /api/nodes/{id}/xray/route-groups/{objectID}/export
POST   /api/nodes/{id}/xray/route-groups/import
POST   /api/nodes/{id}/xray/routes/query
POST   /api/nodes/{id}/connectors/test
POST   /api/nodes/{id}/connectors/proxy-diagnostics
GET    /api/nodes/{id}/connectors/{objectID}/speedtest/targets
GET    /api/nodes/{id}/connectors/{objectID}/speedtest/results
POST   /api/nodes/{id}/connectors/{objectID}/speedtest
POST   /api/nodes/{id}/{kind}/{objectID}/traffic/reset
ANY    /api/nodes/{id}/integrations/{provider}/{action}
GET|PUT /api/nodes/mtls
```

节点 HTTPS 默认使用系统证书校验，也可固定精确的叶子证书 SHA-256。私有、回环、链路本地地址和明文 HTTP 必须显式允许；未指定地址、组播地址、环境代理和重定向不会被静默接受。

## 自动化调用建议

1. 使用独立、可撤销且有失效时间的 API Token。
2. 读取配置与 ETag，保留不认识的字段。
3. 调用保存验证；需要运行时变更时再调用应用验证。
4. 使用 `If-Match` 写入，遇到 `409` 时重新读取，不要盲目重试覆盖。
5. 应用后读取 `/api/runtime/state`，再用与业务相同连接端的诊断验证实际出口。
6. 不在自动化输出中记录 Token、协议凭据、私钥、完整配置或未脱敏诊断数据。
