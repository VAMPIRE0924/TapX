# TapX Agent Handoff

This repository is intended for long-term local development first and clean public GitHub publishing later.

## Hard Product Boundaries

- Implement according to the root requirement document and the Markdown decisions under `docs/`.
- The Web panel must be advanced like 3x-ui: expose all meaningful low-level parameters through UI/API, not simplified presets.
- Follow 3x-ui's high-plasticity composition style: let operators freely combine Listener, Connector, Client, Route, vKey, Device, and AddressLimit objects, then validate the combination on save/apply.
- Raw UDP and Raw TCP no-encryption/no-auth modes are first-class core features.
- Do not treat no-auth raw transport as a conflict to remove.
- Raw UDP/TCP features are composable: vKey, Client binding, Route binding, and allowed TAP/TUN IP/MAC limits can all be unset or configured in flexible combinations.
- If a feature is unset, it must not add hot-path work. If it is configured, Go validates it and prepares runtime config before workers run.
- Do not add a separate `tag` classifier for raw traffic. vKey is the raw binding/admission key when the user wants that layer.
- Go is the control plane: Web/API, DB, runtime config generation, lifecycle, logs, stats aggregation, Xray runtime management.
- C is the fast path for raw UDP/TCP/TAP/TUN where maximum throughput matters.
- Never design per-packet cgo calls.
- Never put DB reads, JSON parsing, shell calls, or log writes in the per-packet path.
- Same-process embedded Xray transport comes after the raw fastpath MVP. It must not block raw UDP/TCP.

## Local vs Lab

- Development happens locally, preferably in WSL Ubuntu.
- Public servers are only validation/benchmark targets.
- Do not commit server credentials, private host notes, or generated keys.
- Keep lab-specific files in `.local/`, which is ignored by git.
- Current OpenWrt development and build validation target is x86-64 only.
- MT7986 and other OpenWrt architectures are deferred until they are explicitly needed.

## First Build Target

Build the raw data plane before the full UI:

1. TUN + Raw UDP no-header forwarding with optional generated feature checks.
2. TUN + Raw TCP length-prefix forwarding.
3. C fastpath counters and drops.
4. Go runtime config process.
5. Same-process embedded Xray transport prototype.
6. vKey/Route/Client/address-limit composition.
7. TAP, MAC Guard, ARP/ND Guard.
8. Web UI completion.
