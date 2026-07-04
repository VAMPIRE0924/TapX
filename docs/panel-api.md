# Panel API and UI

TapX panel storage starts as a flexible SQLite object store. Each object is
stored by `kind/id` with the full JSON payload. This keeps the API suitable for
an advanced 3x-ui-style panel: the UI can expose low-level parameters without
forcing a fixed wizard or preset.

The panel binary serves the first embedded Web UI at `/` by default, or under a
configured `-base-path` such as `/tapx-private`. It can start HTTP or HTTPS
based on Settings and the selected listen address. It is an operational screen
for object CRUD, dense field editing, full JSON editing, runtime
apply/stop/state, operation logs, backup/restore, diagnostics, optional panel
login, aggregated stats, and fastpath counters. The UI calls the same API
documented below.

The current object kinds are:

- `devices`
- `listeners`
- `connectors`
- `clients`
- `routes`
- `vkeys`
- `addresses`
- `xrayProfiles`
- `settings`

## Run

```bash
go run ./cmd/tapx-panel -db .local/tapx.db -listen 127.0.0.1:8080
```

Use a non-root base path when the install flow should print a private panel
URL:

```bash
go run ./cmd/tapx-panel -db .local/tapx.db -listen 127.0.0.1:8080 -base-path /tapx-local
```

The default bind address is localhost. Panel login is configured from the
`settings` object and is disabled by default for local development. Generate a
password hash with:

```bash
printf 'change-me' | go run ./cmd/tapx-panel -hash-password-stdin
```

Then set `PanelAuthEnabled`, `AdminUsername`, `AdminPasswordHash`, and optional
`SessionTTLSecond` in Settings. This protects panel APIs only; it does not add
auth to raw UDP/TCP transports.

For installer-style initialization, write an enabled Settings object directly
from the binary:

```bash
go run ./cmd/tapx-panel \
  -db .local/tapx.db \
  -listen 127.0.0.1:8080 \
  -base-path /tapx-local \
  -init-admin \
  -admin-username admin \
  -admin-password change-me
```

## Endpoints

```text
GET    /api/health
GET    /api/auth/session
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/config
PUT    /api/config
POST   /api/config/validate?mode=save
POST   /api/config/validate?mode=apply
GET    /api/runtime
POST   /api/runtime
GET    /api/runtime/state
POST   /api/runtime/apply
POST   /api/runtime/enforce
POST   /api/runtime/stop
GET    /api/dashboard
GET    /api/stats
GET    /api/templates/raw-pair
GET    /api/share/clients/{id}
POST   /api/clients/{id}/traffic/reset
GET    /api/xray/external/status
POST   /api/xray/external/upload
POST   /api/xray/external/download
GET    /api/backup
POST   /api/backup/restore
GET    /api/logs
DELETE /api/logs
GET    /api/diagnostics
GET    /api/objects/{kind}
GET    /api/objects/{kind}/{id}
PUT    /api/objects/{kind}/{id}
DELETE /api/objects/{kind}/{id}
```

`PUT /api/config` replaces the full object set and runs save-time validation.
`GET /api/runtime` loads the stored config, runs apply-time validation, and
returns the generated runtime config used by `tapx-core`.

Listener and Connector raw settings include advanced `RawTCP.TLS` and
`RawUDP.DTLS` objects. The API accepts logically valid saved TLS/DTLS fields
for operator composition and sharing. Enabled RawTCP TLS applies through a
separate Go TLS frame runtime and keeps naked Raw TCP on the C fastpath.
Enabled RawUDP DTLS applies through a separate Go DTLS datagram runtime and
keeps naked Raw UDP on the C fastpath. Disabled TLS/DTLS fields add no raw
UDP/TCP hot-path work.

`POST /api/runtime/apply` loads the stored config and generates runtime config.
When the current and replacement runtimes use disjoint prepared resources
(interface names, TAP bridge resources, and listener ports), the manager starts
the replacement controller first and stops the old controller only after the new
one is running. Runtime state reports this as `lastReloadMode=prepare-first`.
When resources may conflict, apply uses the existing stop-first path with
rollback-on-start-failure. Runtime state reports that as
`lastReloadMode=stop-first`; if the replacement cannot start, the manager
attempts to restart the last successfully applied runtime and exposes the
rollback result in runtime state.

`POST /api/runtime/enforce` runs the Client limit enforcement pass immediately.
The runtime manager also starts a periodic enforcement loop after apply, using
the current Settings stats interval. Enforcement closes pipes bound to disabled,
expired, or over-quota Clients.

`GET /api/dashboard` returns the panel's high-signal operator overview: runtime
state, object counts, process/fastpath/OpenWrt diagnostics, aggregated stats,
recent logs, and rate estimates calculated from the previous dashboard counter
snapshot. The rate window is local to the panel process and is reset on panel
restart.

`GET /api/runtime/state` returns the current local runtime generation,
running/stopped status, apply/rollback/enforcement timestamps, enforcement
events, rollback errors, pipe endpoints, local/remote addresses, and fastpath
counters. Xray-bound frame pipes appear under `xrayPipes` with the same
endpoint/device/route/client/address/counter shape as raw UDP/TCP pipes. When
Xray mode is active, the state also includes Xray runtime information under
`xrayRuntimes`; external mode includes process/config fields, while embedded
mode reports the in-process xray-core adapter state without a PID or external
config path. `POST /api/runtime/stop` stops the local runtime and any managed
Xray runtime.

`GET /api/stats` returns a control-plane aggregation of the current runtime
counter snapshot. It groups fastpath counters by transport, endpoint, device,
route, and client, and includes client traffic-cap/expiration status. This is a
Go-side snapshot; the C fastpath still only updates counters and never reads the
DB or writes per-packet logs.

`GET /api/templates/raw-pair` generates a two-side raw UDP/TUN or raw TCP/TUN
configuration pair without saving it. Query parameters include `transport`,
`hostA`, `hostB`, `port`, `tunA`, `tunB`, `ifNameA`, `ifNameB`, `mtu`, and
optional `vkey`. The response contains side `a`, side `b`, and generated
runtime previews for both sides.

`GET /api/share/clients/{id}` generates a Client share document. Raw UDP/TCP
clients receive a compressed `tapx://client/gzip/<base64url>` import link.
VLESS/Xray clients receive a standard `vless://` link when the bound Listener
and XrayProfile provide enough information. The response also includes a PNG QR
data URI and the structured payload containing Listener/Connector, credential,
vKey, Device, Route, and AddressLimit data, including fixed IP/MAC, gateway,
DNS, pushed routes, and default-route permission when configured. This is not a
subscription API.

`POST /api/clients/{id}/traffic/reset` records the current runtime RX/TX
counters for that Client as reset offsets and updates `TrafficResetAt`,
`TrafficRXOffset`, and `TrafficTXOffset` in the Client object. Future Client
quota/used-traffic views subtract those offsets. This does not touch the C
counter hot path and does not reset unrelated runtime pipes.

`GET /api/xray/external/status?path=...` checks an external `xray` binary path
and reports existence, regular-file state, executable state, size, mode, and
mtime. If `path` is omitted, the first enabled `Settings.ExternalXrayPath` is
used.

`POST /api/xray/external/upload?path=...` writes an uploaded external Xray
binary to `path`, or to `Settings.ExternalXrayPath` when `path` is omitted. It
accepts either a raw request body or a `multipart/form-data` request with a
`file` part. The write is streamed through a temporary file, capped at 128 MiB,
installed atomically, and marked executable where the OS supports it.

`POST /api/xray/external/download` accepts
`{"url":"https://...","path":"..."}` and downloads an external Xray binary from
an operator-supplied HTTP(S) URL. `path` is optional and defaults to
`Settings.ExternalXrayPath`. The panel does not shell out; it streams the HTTP
response into the same atomic writer used by upload.

`GET /api/backup` exports a JSON backup document containing metadata and the
current object config. `POST /api/backup/restore` accepts either that backup
document or a raw config object and replaces the stored config after save-time
validation.

`GET /api/logs` returns the in-process panel operation log. `DELETE /api/logs`
clears it. These logs are control-plane events only; the C fastpath still never
writes per-packet logs.

`GET /api/diagnostics` returns a read-only report with product version, Go
process data, fastpath ABI, current x86-64 OpenWrt build target, object counts,
and runtime state. It does not execute shell commands.

`PUT /api/objects/{kind}/{id}` accepts the full object JSON for that kind. If
the JSON omits `ID`, the path ID is used. If the JSON has a different `ID`, the
request is rejected.

`GET /api/auth/session` reports whether panel login is enabled and whether the
current HTTP session is authenticated. `POST /api/auth/login` accepts
`{"username":"...","password":"..."}` and sets an HTTP-only session cookie when
the Settings password hash verifies. `POST /api/auth/logout` clears the session.

## Validation Model

Save-time validation rejects broken object shape and broken references. Apply
validation additionally rejects enabled objects that reference disabled objects.

This preserves the product rule:

- empty optional references remain valid,
- raw UDP/TCP can run without auth or encryption,
- vKey/Client/Route/Device/Connector/AddressLimit can be freely composed,
- Client credentials support `uuid`, `password`, `vkey`, or empty values,
- Client traffic reset is a control-plane offset and does not reset raw C
  counters,
- Xray Profiles and Settings are stored as first-class control-plane objects,
- external Xray profiles require `Settings.ExternalXrayPath` plus enough
  endpoint/profile fields to compile a valid external Xray config at apply time,
- embedded Xray profiles start xray-core in the TapX process and do not require
  `Settings.ExternalXrayPath`,
- invalid compositions are rejected before runtime,
- C fastpath never reads SQLite or parses JSON.

## Example

```bash
curl -X PUT http://127.0.0.1:8080/api/config \
  -H 'content-type: application/json' \
  --data-binary @docs/examples/raw-udp-tun.json

curl http://127.0.0.1:8080/api/runtime
curl -X POST http://127.0.0.1:8080/api/runtime/apply
curl http://127.0.0.1:8080/api/dashboard
curl http://127.0.0.1:8080/api/runtime/state
curl http://127.0.0.1:8080/api/stats
curl http://127.0.0.1:8080/api/share/clients/client-a
curl -X POST http://127.0.0.1:8080/api/clients/client-a/traffic/reset
curl http://127.0.0.1:8080/api/xray/external/status
curl http://127.0.0.1:8080/api/logs
curl http://127.0.0.1:8080/api/backup
curl http://127.0.0.1:8080/api/diagnostics
curl -X POST http://127.0.0.1:8080/api/runtime/stop
```
