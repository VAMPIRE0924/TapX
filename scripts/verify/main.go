package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/xrayruntime"
)

type verifier struct {
	root                   string
	requireOpenWrtPackages bool
	failures               []string
}

func main() {
	root := flag.String("repo", ".", "repository root")
	requirePackages := flag.Bool("require-openwrt-package", false, "fail when native OpenWrt package files are missing")
	flag.Parse()

	v := verifier{root: cleanRoot(*root), requireOpenWrtPackages: *requirePackages}
	v.checkRequiredFiles()
	v.checkJSONFiles()
	v.checkRuntimeExamples()
	v.checkTemplates()
	v.checkRuntimeReload()
	v.checkDashboard()
	v.checkClientTrafficReset()
	v.checkEmbeddedXrayCore()
	v.checkExternalXrayBinaryManagement()
	v.checkLinuxInstall()
	v.checkClientSharing()
	v.checkRawSecurityConfigSurface()
	v.checkNetdevVisibilityIntegration()
	v.checkAddressGuardIntegration()
	v.checkOpenWrtLuCI()
	v.checkOpenWrtPackages()
	v.checkOpenWrtPackageVersion()
	v.checkSensitiveStrings()
	if len(v.failures) > 0 {
		for _, failure := range v.failures {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", failure)
		}
		os.Exit(1)
	}
	fmt.Println("verify local: ok")
}

func (v *verifier) checkOpenWrtPackageVersion() {
	payload, err := os.ReadFile(v.path("openwrt/Makefile"))
	if err != nil {
		v.fail("read OpenWrt package version rules: %v", err)
		return
	}
	text := string(payload)
	for _, want := range []string{
		"TAPX_PACKAGE_VERSION:=$(subst -dev,_git,$(TAPX_SOURCE_VERSION))",
		"PKG_VERSION:=$(subst -,_,$(TAPX_PACKAGE_VERSION))",
	} {
		if !strings.Contains(text, want) {
			v.fail("OpenWrt package version rules missing %q", want)
		}
	}

	source := "0.2.1-dev20260726"
	normalized := strings.Replace(source, "-dev", "_git", 1)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized != "0.2.1_git20260726" {
		v.fail("OpenWrt development package version normalized to %q", normalized)
	}
}

func (v *verifier) checkTemplates() {
	for _, transport := range []string{"udp", "tcp"} {
		template, err := config.BuildRawPairTemplate(config.RawPairTemplateOptions{
			Transport: model.Transport(transport),
			HostA:     "192.0.2.10",
			HostB:     "192.0.2.20",
			VKey:      "verify-key",
		})
		if err != nil {
			v.fail("build %s raw pair template: %v", transport, err)
			continue
		}
		if template.RuntimeA == nil || template.RuntimeB == nil {
			v.fail("build %s raw pair template: missing runtime previews", transport)
		}
	}
}

func (v *verifier) checkRuntimeReload() {
	checks := map[string][]string{
		"internal/panel/runtime_manager.go": {
			"canPrepareRuntimeInParallel",
			"prepare-first",
			"stop-first",
			"lastReloadMode",
		},
		"internal/panel/runtime_manager_test.go": {
			"TestRuntimeManagerPrepareFirstReloadForDisjointResources",
			"TestRuntimeManagerUsesStopFirstWhenResourcesConflict",
			"TestRuntimeManagerPrepareFirstFailureKeepsOldRuntime",
		},
		"web/src/pages/DashboardPage.tsx": {
			"restartRuntimeComponent",
			"stopRuntimeComponent",
			"dashboard.reload",
		},
		"web/src/shared/api.ts": {
			"/api/runtime/components/",
			"restartRuntimeComponent",
			"stopRuntimeComponent",
		},
		"internal/core/supervisor.go": {
			"RuntimeComponentTapX",
			"RestartComponent",
			"StopComponent",
		},
		"internal/core/supervisor_test.go": {
			"TestSupervisorComponentStopsAreIsolated",
		},
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read runtime reload check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("runtime reload check %s missing %q", rel, want)
			}
		}
	}
}

func cleanRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func (v *verifier) fail(format string, args ...any) {
	v.failures = append(v.failures, fmt.Sprintf(format, args...))
}

func (v *verifier) checkRequiredFiles() {
	required := []string{
		"README.md",
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
		"web/package.json",
		"web/src/app/App.tsx",
		"web/src/app/runtime-path.ts",
		"web/src/shared/api.ts",
		"web/src/pages/KernelPage.tsx",
		"scripts/build/sync-panel-web.mjs",
		"scripts/build/linux.sh",
		"scripts/build/linux-amd64.sh",
		"scripts/build/linux-arm64.sh",
		"scripts/install/install.sh",
		"scripts/build/release-archives.sh",
		"scripts/lab/common.ps1",
		"scripts/lab/preflight.ps1",
		"scripts/lab/raw-transport-smoke.ps1",
		"scripts/lab/raw-transport-benchmark.ps1",
		"scripts/lab/xray-embedded-smoke.ps1",
		"scripts/lab/xray-frame-tun-smoke.ps1",
		"scripts/lab/xray-wrapped-raw-tcp-smoke.ps1",
		"scripts/lab/raw-protected-smoke.ps1",
		"scripts/integration/raw-tcp-tls-tun-netns.sh",
		"scripts/integration/raw-udp-dtls-tun-netns.sh",
		"scripts/integration/address-guard-netns.sh",
		"scripts/build/openwrt-x86-64-packages.sh",
		"openwrt/Makefile",
		"docs/openwrt-dependencies.md",
		"openwrt/tapx-core/files/etc/config/tapx",
		"openwrt/tapx-core/files/etc/init.d/tapx",
		"openwrt/tapx-panel/files/etc/init.d/tapx-panel",
		"openwrt/luci-app-tapx/root/www/luci-static/resources/tapx/common.js",
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/overview.js",
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/panel.js",
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/backup.js",
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/logs.js",
	}
	if v.exists("AGENTS.md") {
		required = append(required,
			"AGENTS.md",
			"docs/requirements-map.md",
			"docs/architecture.md",
			"docs/panel-api.md",
			"docs/openwrt.md",
			"docs/install-linux.md",
			"docs/release.md",
			"docs/verification.md",
		)
	}
	for _, rel := range required {
		if _, err := os.Stat(v.path(rel)); err != nil {
			v.fail("required file %s: %v", rel, err)
		}
	}
}

func (v *verifier) checkJSONFiles() {
	for _, dir := range []string{"docs", "openwrt"} {
		root := v.path(dir)
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			v.fail("stat %s: %v", dir, err)
			continue
		}
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				v.fail("walk %s: %v", path, err)
				return nil
			}
			if entry.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				v.fail("read json %s: %v", v.rel(path), err)
				return nil
			}
			var decoded any
			if err := json.Unmarshal(payload, &decoded); err != nil {
				v.fail("invalid json %s: %v", v.rel(path), err)
			}
			return nil
		})
	}
}

func (v *verifier) checkRuntimeExamples() {
	examples := []string{
		"openwrt/tapx-core/files/etc/tapx/runtime.json.example",
	}
	if v.exists("docs/examples/raw-udp-tun.json") {
		examples = append(examples,
			"docs/examples/raw-udp-tun.json",
			"docs/examples/raw-udp-tun-vkey.json",
			"docs/examples/raw-udp-tap-guard.json",
			"docs/examples/raw-tcp-tun.json",
			"docs/examples/xray-external-listener.json",
			"docs/examples/xray-embedded-core.json",
		)
	}
	for _, rel := range examples {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read runtime config %s: %v", rel, err)
			continue
		}
		var cfg config.RuntimeConfig
		if err := json.Unmarshal(payload, &cfg); err != nil {
			v.fail("parse runtime config %s: %v", rel, err)
			continue
		}
		if _, err := config.GenerateRuntime(cfg); err != nil {
			v.fail("generate runtime %s: %v", rel, err)
		}
	}
}

func (v *verifier) checkDashboard() {
	checks := map[string][]string{
		"internal/panel/dashboard.go": {
			"DashboardReport",
			"DashboardRates",
			"recentLogEvents",
			"rxBytesPerSecond",
		},
		"internal/panel/server.go": {
			"/api/dashboard",
			"handleDashboard",
		},
		"web/src/shared/api.ts": {
			"/api/dashboard",
		},
		"web/src/pages/DashboardPage.tsx": {
			"getDashboard",
			"dashboard.management",
			"dashboard.realtimeTransport",
			"dashboard.tunnelStatus",
			"dashboard.activeObjects",
			"dashboard.linkProtection",
		},
	}
	if v.exists("docs/panel-api.md") {
		checks["docs/panel-api.md"] = []string{
			"GET    /api/dashboard",
			"rate estimates",
			"recent logs",
		}
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read dashboard check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("dashboard check %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkClientTrafficReset() {
	checks := map[string][]string{
		"internal/model/model.go": {
			"TrafficResetAt",
			"TrafficRXOffset",
			"TrafficTXOffset",
		},
		"internal/panel/client_traffic.go": {
			"handleClientTraffic",
			"resetClientTraffic",
			"clientRawCountersFromRuntimeState",
		},
		"internal/panel/stats.go": {
			"adjustClientCounters",
			"TrafficResetAt",
			"TrafficRXOffset",
		},
		"internal/panel/server.go": {
			"/api/clients/",
			"handleClientTraffic",
		},
		"web/src/pages/UserPage.tsx": {
			"resetClientTraffic",
			"user.resetTraffic",
		},
		"web/src/shared/api.ts": {
			"TrafficResetAt",
			"TrafficRXOffset",
			"resetClientTraffic",
			"managedTrafficResetPath",
			"'clients'",
		},
	}
	if v.exists("docs/panel-api.md") {
		checks["docs/panel-api.md"] = []string{
			"POST   /api/clients/{id}/traffic/reset",
			"TrafficResetAt",
			"TrafficRXOffset",
		}
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read client traffic reset check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("client traffic reset check %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkEmbeddedXrayCore() {
	port, err := freeTCPPort()
	if err != nil {
		v.fail("find free embedded xray port: %v", err)
		return
	}
	cfg := config.RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID:                  "verify-embedded",
			Enabled:             true,
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			AdvancedJSON:        `{"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"rules":[{"type":"field","inboundTag":["verify-xray-listener"],"outboundTag":"direct"}]}}`,
		}},
		Listeners: []model.Listener{{
			ID:            "verify-xray-listener",
			Enabled:       true,
			BindHost:      "127.0.0.1",
			BindPort:      uint16(port),
			Transport:     model.TransportXray,
			XrayProfileID: "verify-embedded",
		}},
	}
	runtime, err := config.GenerateRuntime(cfg)
	if err != nil {
		v.fail("generate embedded xray runtime: %v", err)
		return
	}
	manager := xrayruntime.NewManager()
	if err := manager.Start(runtime); err != nil {
		v.fail("start embedded xray runtime: %v", err)
		return
	}
	state := manager.State()
	if !state.Running || state.Runtime != "embedded" || state.Adapter != "xray-core" || state.EndpointCount != 1 {
		v.fail("embedded xray state = %+v, want running embedded xray-core with one endpoint", state)
	}
	if state.PID != 0 || state.ConfigPath != "" {
		v.fail("embedded xray used external process fields: %+v", state)
	}
	if err := manager.Stop(); err != nil {
		v.fail("stop embedded xray runtime: %v", err)
	}
}

func (v *verifier) checkExternalXrayBinaryManagement() {
	checks := map[string][]string{
		"internal/panel/xray_binary.go": {
			"handleXrayExternalStatus",
			"handleXrayExternalUpload",
			"handleXrayExternalDownload",
			"maxXrayBinarySize",
			"multipart/form-data",
		},
		"internal/panel/server.go": {
			"/api/xray/external/status",
			"/api/xray/external/upload",
			"/api/xray/external/download",
		},
		"web/src/pages/KernelPage.tsx": {
			"downloadExternalXray",
			"uploadExternalXray",
		},
		"web/src/shared/api.ts": {
			"/api/xray/external/status",
			"/api/xray/external/upload",
			"/api/xray/external/download",
		},
	}
	if v.exists("docs/panel-api.md") {
		checks["docs/panel-api.md"] = []string{
			"GET    /api/xray/external/status",
			"POST   /api/xray/external/upload",
			"POST   /api/xray/external/download",
		}
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read external xray binary check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("external xray binary check %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkLinuxInstall() {
	checks := map[string][]string{
		"cmd/tapx-panel/main.go": {
			"base-path",
			"db-driver",
			"init-admin",
			"panel-cert-file",
			"disable-panel-https",
			"PanelAuthEnabled",
			"PanelHTTPS",
			"ServeTLS",
			"HashPanelPassword",
		},
		"scripts/install/linux-install.sh": {
			"TAPX_PANEL_BASE_PATH",
			"TAPX_DB_DRIVER",
			"TAPX_DB_SOURCE",
			"1,English (default)",
			"数据库选择",
			"0.0.0.0:$PANEL_PORT",
			"set-panel",
			"set-database",
			"reset-password",
			"tapx-credentials-rollback",
			"-export-backup \"$rollback\"",
			"-restore-backup \"$rollback\"",
			"previous credentials were restored and verified",
			"update-script",
			"scripts/install/linux-install.sh",
			"bash -n",
			"随机重置面板用户名和密码",
			"随机生成的用户名和密码只显示这一次",
			"-init-admin",
			"随机生成的用户名和密码只显示这一次",
		},
		"scripts/install/install.sh": {
			"releases/latest/download",
			"detect_architecture",
			"tapx-linux-${arch}.tar.gz",
			"SHA256SUMS",
			"TAPX_BUILD_DIR",
		},
		"scripts/build/release-archives.sh": {
			"tapx-linux-amd64.tar.gz",
			"tapx-linux-arm64.tar.gz",
			"tapx-openwrt-x86-64.tar.gz",
			"SHA256SUMS",
			"tapx-update-manifest.json",
			"embeddedXray",
		},
		"packaging/systemd/tapx.env": {
			"TAPX_PANEL_BASE_PATH",
			"TAPX_PUBLIC_HOST",
			"TAPX_DB_DRIVER",
			"TAPX_DB_SOURCE",
		},
		"packaging/systemd/tapx-panel.service": {
			"ExecStart=/usr/local/bin/tapx-panel",
			"EnvironmentFile=-/etc/tapx/tapx.env",
		},
		"internal/panel/static/index.html": {
			`<div id="root">`,
			`./assets/`,
		},
		"web/src/app/runtime-path.ts": {
			"panelFetch",
			"panelPath",
			"tapx-base-path",
		},
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read Linux install check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("Linux install check %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkClientSharing() {
	checks := map[string][]string{
		"internal/model/model.go": {
			"UUID",
			"Password",
			"Auth",
			"ConnectorID string",
			"IPv4Gateway",
			"AllowDefaultRoute",
		},
		"internal/panel/share.go": {
			"tapx://client/gzip/",
			`Scheme: "raw"`,
			"buildClientLinks",
			"BuildClientShare",
		},
		"internal/panel/server.go": {
			"/api/share/clients/",
			"handleClientShare",
		},
		"web/src/pages/UserPage.tsx": {
			"getClientShare",
			"copyShareLinks",
		},
		"web/src/shared/api.ts": {
			"/api/share/clients",
			"UUID",
			"ConnectorID",
			"IPv4Gateway",
			"AllowDefaultRoute",
		},
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read client sharing check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("client sharing check %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkAddressGuardIntegration() {
	payload, err := os.ReadFile(v.path("scripts/integration/address-guard-netns.sh"))
	if err != nil {
		v.fail("read address guard integration script: %v", err)
		return
	}
	text := string(payload)
	for _, want := range []string{
		"expect_ping_ok",
		"expect_ping_blocked",
		"10.90.0.99",
		"10.91.0.99",
		"MACs",
		"IPv4CIDRs",
		"TAPX_CORE_BIN",
	} {
		if !strings.Contains(text, want) {
			v.fail("address guard integration script missing %q", want)
		}
	}
}

func (v *verifier) checkRawSecurityConfigSurface() {
	checks := map[string][]string{
		"go.mod": {
			"github.com/pion/dtls/v3",
		},
		"internal/model/model.go": {
			"RawTLSSettings",
			"RawDTLSSettings",
			"AllowInsecure",
			"ReplayWindow",
		},
		"internal/config/validate.go": {
			"RawTCP.TLS.CertFile",
			"RawUDP.DTLS.CertFile",
			"RawUDP.DTLS.ReplayWindow",
		},
		"internal/core/tcp_tls_pipe_linux.go": {
			"startTLSConnector",
			"rawTCPServerTLSConfig",
			"stripRawVKeyHeader",
		},
		"internal/core/udp_dtls_pipe_linux.go": {
			"startDTLSConnector",
			"rawUDPServerDTLSOptions",
			"acceptFirstDTLSPacket",
		},
		"scripts/lab/raw-protected-smoke.ps1": {
			"Raw TCP/TLS/TUN",
			"Raw UDP/DTLS/TUN",
			"ip a show dev",
		},
		"web/src/shared/api.ts": {
			"RawTCP?:",
			"RawUDP?:",
			"CertFile?:",
			"ReplayWindow?:",
			"AllowInsecure?:",
		},
	}
	if v.exists("docs/requirements-map.md") {
		checks["docs/requirements-map.md"] = []string{
			"RawTCP.TLS",
			"RawUDP.DTLS",
		}
	}
	for rel, markers := range checks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read raw security config check %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range markers {
			if !strings.Contains(text, want) {
				v.fail("raw security config check %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkNetdevVisibilityIntegration() {
	for _, rel := range []string{
		"scripts/integration/raw-udp-tun-netns.sh",
		"scripts/integration/raw-udp-tun-vkey-netns.sh",
		"scripts/integration/raw-udp-tap-netns.sh",
		"scripts/integration/raw-tcp-tun-netns.sh",
		"scripts/integration/raw-tcp-tls-tun-netns.sh",
		"scripts/integration/raw-udp-dtls-tun-netns.sh",
	} {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read netdev visibility integration script %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, want := range []string{
			"wait_for_link",
			"show_interface_evidence",
			`ip -n "$ns" -d addr show dev "$name"`,
		} {
			if !strings.Contains(text, want) {
				v.fail("netdev visibility integration %s missing %q", rel, want)
			}
		}
	}
}

func (v *verifier) checkOpenWrtLuCI() {
	packageMakefile, err := os.ReadFile(v.path("openwrt/Makefile"))
	if err != nil {
		v.fail("read OpenWrt package Makefile: %v", err)
	} else {
		makefileText := string(packageMakefile)
		for _, dependency := range []string{
			"+libc",
			"+kmod-tun",
			"+ip-full",
			"+tc-full",
			"+kmod-sched-flower",
			"+iptables-nft",
			"+ip6tables-nft",
			"+ca-bundle",
		} {
			if !strings.Contains(makefileText, dependency) {
				v.fail("OpenWrt tapx-core dependency missing %q", dependency)
			}
		}
	}

	viewChecks := map[string][]string{
		"openwrt/luci-app-tapx/root/www/luci-static/resources/tapx/common.js": {
			"tapx.luci.language", "languageControl", "serviceCard", "panelUrl", "'runtime_' + command",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/overview.js": {
			"tapx.status", "runtimeConfig", "database", "uciConfig", "runtime_status", "runtimeRunning",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/panel.js": {
			"Listening interface", "Save and apply", "coreAutostart", "panelAutostart", "tapx.languageControl", "runtime_status", "runtimeRunning",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/backup.js": {
			"tapx-openwrt-config", "Download backup", "Choose backup", "Reset TapX",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/logs.js": {
			"/sbin/logread", "level", "refresh",
		},
	}
	for rel, markers := range viewChecks {
		payload, err := os.ReadFile(v.path(rel))
		if err != nil {
			v.fail("read LuCI view %s: %v", rel, err)
			continue
		}
		text := string(payload)
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				v.fail("LuCI view %s missing %q", rel, marker)
			}
		}
		if strings.Contains(text, "managedByPanel") {
			v.fail("LuCI view %s still hides independent core controls behind managed mode", rel)
		}
	}
	helper, err := os.ReadFile(v.path("openwrt/luci-app-tapx/root/usr/libexec/tapx-openwrt-config"))
	if err != nil {
		v.fail("read OpenWrt config helper: %v", err)
	} else {
		helperText := string(helper)
		for _, want := range []string{
			"etc/config/tapx etc/tapx/tapx.db",
			"backup must contain only TapX UCI and database files",
			"/rom/etc/config/tapx",
			"/rom/etc/tapx/tapx.db",
		} {
			if !strings.Contains(helperText, want) {
				v.fail("OpenWrt config helper missing %q", want)
			}
		}
		for _, forbidden := range []string{"etc/tapx/cert", "etc/tapx/key", "runtime.json"} {
			if strings.Contains(helperText, forbidden) {
				v.fail("OpenWrt config helper must not archive %q", forbidden)
			}
		}
	}
	keep, err := os.ReadFile(v.path("openwrt/tapx-panel/files/lib/upgrade/keep.d/tapx"))
	if err != nil {
		v.fail("read OpenWrt sysupgrade keep list: %v", err)
	} else if got, want := strings.TrimSpace(string(keep)), "/etc/config/tapx\n/etc/tapx/tapx.db"; got != want {
		v.fail("OpenWrt sysupgrade keep list = %q, want only UCI and DB", got)
	}
	initScript, err := os.ReadFile(v.path("openwrt/tapx-core/files/etc/init.d/tapx"))
	if err != nil {
		v.fail("read OpenWrt core init: %v", err)
	} else {
		initText := string(initScript)
		if !strings.Contains(initText, "-export-runtime-config") {
			v.fail("OpenWrt core init must regenerate runtime config from the database")
		}
		for _, want := range []string{
			"config_get_bool panel_enabled panel enabled 0",
			"config_get_bool panel_initialized panel initialized 0",
			"runtime_status",
			"runtime_start",
			"runtime_restart",
			"runtime_stop",
			"-runtime-control-socket",
			"-runtime-action",
			"procd_set_param term_timeout 15",
		} {
			if !strings.Contains(initText, want) {
				v.fail("OpenWrt core init missing independent runtime control %q", want)
			}
		}
	}
	panelInit, err := os.ReadFile(v.path("openwrt/tapx-panel/files/etc/init.d/tapx-panel"))
	if err != nil {
		v.fail("read OpenWrt panel init: %v", err)
	} else {
		panelInitText := string(panelInit)
		for _, want := range []string{
			"pidof tapx-core",
			"/etc/init.d/tapx stop",
			"while pidof tapx-core",
			"standalone core did not stop before ownership handoff",
			"procd_set_param term_timeout 15",
		} {
			if !strings.Contains(panelInitText, want) {
				v.fail("OpenWrt panel init missing clean runtime handoff %q", want)
			}
		}
	}
	acl, err := os.ReadFile(v.path("openwrt/luci-app-tapx/root/usr/share/rpcd/acl.d/luci-app-tapx.json"))
	if err != nil {
		v.fail("read LuCI ACL: %v", err)
		return
	}
	aclText := string(acl)
	for _, want := range []string{
		"/usr/bin/tapx-panel",
		"/etc/init.d/tapx",
		"/etc/init.d/tapx-panel",
		"/sbin/logread",
		"exec",
	} {
		if !strings.Contains(aclText, want) {
			v.fail("LuCI ACL missing %q", want)
		}
	}
}

func freeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (v *verifier) checkOpenWrtPackages() {
	if !v.requireOpenWrtPackages {
		return
	}
	packageDir := v.path("build/openwrt-x86-64/packages")
	patterns := []struct {
		name string
		apk  string
	}{
		{name: "tapx-core", apk: "tapx-core-*.apk"},
		{name: "luci-app-tapx", apk: "luci-app-tapx-*.apk"},
		{name: "tapx-panel", apk: "tapx-panel-*.apk"},
	}
	for _, item := range patterns {
		matches, _ := filepath.Glob(filepath.Join(packageDir, item.apk))
		if len(matches) == 0 {
			v.fail("missing OpenWrt APK %s", item.name)
			continue
		}
		if len(matches) != 1 {
			v.fail("expected one OpenWrt APK for %s, found %d", item.name, len(matches))
			continue
		}
		if info, err := os.Stat(matches[0]); err != nil || info.Size() < 512 {
			v.fail("invalid apk %s", v.rel(matches[0]))
		}
	}
}

func (v *verifier) checkSensitiveStrings() {
	needles := []string{
		"ID" + "IOT",
		"ID" + "IOT" + "cc",
		"193" + "." + "123",
		"139" + "." + "185",
	}
	_ = filepath.WalkDir(v.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			v.fail("walk sensitive scan %s: %v", path, err)
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".local", "build", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			v.fail("stat sensitive scan %s: %v", v.rel(path), err)
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if filepath.Ext(path) == ".docx" {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			v.fail("read sensitive scan %s: %v", v.rel(path), err)
			return nil
		}
		text := string(payload)
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				v.fail("sensitive marker %q found in %s", needle, v.rel(path))
			}
		}
		return nil
	})
}

func (v *verifier) path(rel string) string {
	return filepath.Join(v.root, filepath.FromSlash(rel))
}

func (v *verifier) rel(path string) string {
	rel, err := filepath.Rel(v.root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func (v *verifier) exists(rel string) bool {
	_, err := os.Stat(v.path(rel))
	return err == nil
}
