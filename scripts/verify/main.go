package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/xrayruntime"
)

type verifier struct {
	root       string
	requireIPK bool
	failures   []string
}

func main() {
	root := flag.String("repo", ".", "repository root")
	requireIPK := flag.Bool("require-openwrt-ipk", false, "fail when OpenWrt ipk files are missing")
	flag.Parse()

	v := verifier{root: cleanRoot(*root), requireIPK: *requireIPK}
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
	v.checkSensitiveStrings()
	if len(v.failures) > 0 {
		for _, failure := range v.failures {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", failure)
		}
		os.Exit(1)
	}
	fmt.Println("verify local: ok")
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
		"internal/panel/static/app.js": {
			"Reload Mode",
			"lastReloadMode",
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
	for _, rel := range []string{
		"README.md",
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
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
		"scripts/build/openwrt-x86-64-ipk.sh",
		"scripts/build/mkipk.go",
		"docs/examples/raw-udp-tun.json",
		"docs/examples/raw-tcp-tun.json",
		"openwrt/tapx-core/files/etc/config/tapx",
		"openwrt/tapx-core/files/etc/init.d/tapx",
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/config.js",
	} {
		if _, err := os.Stat(v.path(rel)); err != nil {
			v.fail("required file %s: %v", rel, err)
		}
	}
}

func (v *verifier) checkJSONFiles() {
	for _, dir := range []string{"docs", "openwrt"} {
		root := v.path(dir)
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
	for _, rel := range []string{
		"docs/examples/raw-udp-tun.json",
		"docs/examples/raw-udp-tun-vkey.json",
		"docs/examples/raw-udp-tap-guard.json",
		"docs/examples/raw-tcp-tun.json",
		"docs/examples/xray-external-listener.json",
		"docs/examples/xray-embedded-core.json",
		"openwrt/tapx-core/files/etc/tapx/runtime.json.example",
	} {
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
		"internal/panel/static/app.js": {
			"/api/dashboard",
			"RX Rate",
			"Recent Logs",
			"dashboardLogsPanelHTML",
		},
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
		"internal/panel/static/app.js": {
			"reset-traffic",
			"TrafficResetAt",
			"/api/clients/",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/config.js": {
			"TrafficResetAt",
			"TrafficRXOffset",
			"TrafficTXOffset",
		},
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
		"internal/panel/static/app.js": {
			"Xray Binary",
			"/api/xray/external/status",
			"/api/xray/external/upload",
			"/api/xray/external/download",
			"xrayBinaryPath",
		},
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
			"init-admin",
			"PanelAuthEnabled",
			"PanelHTTPS",
			"ListenAndServeTLS",
			"HashPanelPassword",
		},
		"scripts/install/linux-install.sh": {
			"TAPX_PANEL_BASE_PATH",
			"--admin-username",
			"--admin-password",
			"--base-path",
			"--unit-dir",
			"-init-admin",
			"panel url:",
			"admin password:",
		},
		"scripts/install/install.sh": {
			"tapx-linux-amd64.tar.gz",
			"TAPX_VERSION",
			"TAPX_PANEL_LISTEN",
			"tapx-panel.service",
			"panel url:",
			"admin password:",
		},
		"README.md": {
			"curl -fsSL https://raw.githubusercontent.com/VAMPIRE0924/TapX/main/scripts/install/install.sh | sudo bash",
			"tapx-linux-amd64.tar.gz",
			"tapx-openwrt-x86-64.tar.gz",
		},
		"scripts/build/release-archives.sh": {
			`linux_name="tapx-linux-amd64"`,
			`openwrt_name="tapx-openwrt-x86-64"`,
			"SHA256SUMS",
		},
		"packaging/systemd/tapx.env": {
			"TAPX_PANEL_BASE_PATH",
		},
		"packaging/systemd/tapx-panel.service": {
			"-base-path=${TAPX_PANEL_BASE_PATH}",
		},
		"internal/panel/static/index.html": {
			`href="app.css"`,
			`src="app.js"`,
		},
		"internal/panel/static/app.js": {
			"detectBasePath",
			"apiURL",
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
		"go.mod": {
			"github.com/skip2/go-qrcode",
		},
		"internal/model/model.go": {
			"CredentialType",
			"CredentialValue",
			"ConnectorID string",
			"IPv4Gateway",
			"AllowDefaultRoute",
		},
		"internal/panel/share.go": {
			"tapx://client/gzip/",
			`Scheme:   "vless"`,
			"qrcode.Encode",
			"BuildClientShare",
		},
		"internal/panel/server.go": {
			"/api/share/clients/",
			"handleClientShare",
		},
		"internal/panel/static/app.js": {
			"Client Share",
			"/api/share/clients",
			"CredentialType",
			"Binding.ConnectorID",
			"IPv4Gateway",
			"AllowDefaultRoute",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/config.js": {
			"IPv4Gateway",
			"IPv6Gateway",
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
			"rawUDPServerDTLSConfig",
			"acceptFirstDTLSPacket",
		},
		"scripts/lab/raw-protected-smoke.ps1": {
			"Raw TCP/TLS/TUN",
			"Raw UDP/DTLS/TUN",
			"ip a show dev",
		},
		"internal/panel/static/app.js": {
			"RawTCP.TLS.CertFile",
			"RawUDP.DTLS.CertFile",
			"RawUDP.DTLS.ReplayWindow",
			"RawTCP.TLS.AllowInsecure",
			"RawUDP.DTLS.AllowInsecure",
		},
		"openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/config.js": {
			"RawTCP.TLS.CertFile",
			"RawUDP.DTLS.CertFile",
			"RawUDP.DTLS.ReplayWindow",
			"RawTCP.TLS.AllowInsecure",
			"RawUDP.DTLS.AllowInsecure",
		},
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
	view, err := os.ReadFile(v.path("openwrt/luci-app-tapx/root/www/luci-static/resources/view/tapx/config.js"))
	if err != nil {
		v.fail("read LuCI view: %v", err)
		return
	}
	viewText := string(view)
	for _, want := range []string{
		"tapx-core",
		"-check",
		"/etc/init.d/tapx",
		"Service status",
		"Reload TapX",
		"fs.exec",
		"Object field editor",
		"Append / Replace Object",
		"Binding.ConnectorID",
		"CredentialType",
		"TrafficResetAt",
		"RawUDP.ReceiveBuffer",
		"RawTCP.FastOpen",
		"RawTCP.TLS.CertFile",
		"RawUDP.DTLS.CertFile",
		"RawUDP.DTLS.ReplayWindow",
		"Xray Profile",
		"OpenWrt Build Target",
		"IPv4Gateway",
		"AllowDefaultRoute",
	} {
		if !strings.Contains(viewText, want) {
			v.fail("LuCI view missing %q", want)
		}
	}
	acl, err := os.ReadFile(v.path("openwrt/luci-app-tapx/root/usr/share/rpcd/acl.d/luci-app-tapx.json"))
	if err != nil {
		v.fail("read LuCI ACL: %v", err)
		return
	}
	aclText := string(acl)
	for _, want := range []string{
		"/usr/bin/tapx-core",
		"/etc/init.d/tapx",
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
	version := os.Getenv("TAPX_VERSION")
	if version == "" {
		version = "0.0.0-dev"
	}
	packages := map[string]ipkExpectation{
		fmt.Sprintf("build/openwrt-x86-64/packages/tapx-core_%s_x86_64.ipk", version): {
			ControlContains: []string{
				"Package: tapx-core",
				"Architecture: x86_64",
				"Depends: libc",
			},
			DataFiles: []string{
				"./usr/bin/tapx-core",
				"./etc/config/tapx",
				"./etc/init.d/tapx",
				"./etc/tapx/runtime.json.example",
			},
			Conffiles: []string{"/etc/config/tapx"},
		},
		fmt.Sprintf("build/openwrt-x86-64/packages/luci-app-tapx_%s_all.ipk", version): {
			ControlContains: []string{
				"Package: luci-app-tapx",
				"Architecture: all",
				"Depends: luci-base, tapx-core",
			},
			DataFiles: []string{
				"./usr/share/luci/menu.d/luci-app-tapx.json",
				"./usr/share/rpcd/acl.d/luci-app-tapx.json",
				"./www/luci-static/resources/view/tapx/config.js",
			},
			DataContains: map[string][]string{
				"./usr/share/rpcd/acl.d/luci-app-tapx.json": {
					"/usr/bin/tapx-core",
					"/etc/init.d/tapx",
					"exec",
				},
				"./www/luci-static/resources/view/tapx/config.js": {
					"Check saved runtime",
					"Service status",
					"Reload TapX",
					"fs.exec",
					"Object field editor",
					"Append / Replace Object",
					"Binding.ConnectorID",
					"CredentialType",
					"TrafficResetAt",
					"RawUDP.ReceiveBuffer",
					"RawUDP.DTLS.CertFile",
					"RawUDP.DTLS.ReplayWindow",
					"IPv4Gateway",
					"AllowDefaultRoute",
				},
			},
		},
	}
	for rel, expect := range packages {
		if _, err := os.Stat(v.path(rel)); err != nil {
			if v.requireIPK {
				v.fail("missing OpenWrt package %s: %v", rel, err)
			}
			continue
		}
		if err := verifyIPK(v.path(rel), expect); err != nil {
			v.fail("verify ipk %s: %v", rel, err)
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

type ipkExpectation struct {
	ControlContains []string
	DataFiles       []string
	DataContains    map[string][]string
	Conffiles       []string
}

func verifyIPK(path string, expect ipkExpectation) error {
	members, err := readAr(path)
	if err != nil {
		return err
	}
	for _, name := range []string{"debian-binary", "control.tar.gz", "data.tar.gz"} {
		if _, ok := members[name]; !ok {
			return fmt.Errorf("missing ar member %s", name)
		}
	}
	if strings.TrimSpace(string(members["debian-binary"])) != "2.0" {
		return errors.New("debian-binary is not 2.0")
	}
	controlFiles, err := readTarGz(members["control.tar.gz"])
	if err != nil {
		return fmt.Errorf("control.tar.gz: %w", err)
	}
	control := string(controlFiles["./control"])
	for _, want := range expect.ControlContains {
		if !strings.Contains(control, want) {
			return fmt.Errorf("control missing %q", want)
		}
	}
	if len(expect.Conffiles) > 0 {
		conffiles := splitLines(string(controlFiles["./conffiles"]))
		for _, want := range expect.Conffiles {
			if !contains(conffiles, want) {
				return fmt.Errorf("conffiles missing %q", want)
			}
		}
	}
	dataFiles, err := readTarGz(members["data.tar.gz"])
	if err != nil {
		return fmt.Errorf("data.tar.gz: %w", err)
	}
	for _, want := range expect.DataFiles {
		if _, ok := dataFiles[want]; !ok {
			return fmt.Errorf("data missing %s; got %s", want, strings.Join(sortedKeys(dataFiles), ", "))
		}
	}
	for name, markers := range expect.DataContains {
		data, ok := dataFiles[name]
		if !ok {
			return fmt.Errorf("data missing %s; got %s", name, strings.Join(sortedKeys(dataFiles), ", "))
		}
		text := string(data)
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				return fmt.Errorf("data %s missing %q", name, marker)
			}
		}
	}
	return nil
}

func readAr(path string) (map[string][]byte, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(payload) < 8 || string(payload[:8]) != "!<arch>\n" {
		return nil, errors.New("invalid ar magic")
	}
	out := map[string][]byte{}
	offset := 8
	for offset < len(payload) {
		if offset+60 > len(payload) {
			return nil, errors.New("truncated ar header")
		}
		header := string(payload[offset : offset+60])
		offset += 60
		name := strings.TrimSpace(header[:16])
		name = strings.TrimSuffix(name, "/")
		sizeText := strings.TrimSpace(header[48:58])
		var size int
		if _, err := fmt.Sscanf(sizeText, "%d", &size); err != nil {
			return nil, fmt.Errorf("parse ar size %q: %w", sizeText, err)
		}
		if offset+size > len(payload) {
			return nil, fmt.Errorf("ar member %s exceeds file", name)
		}
		out[name] = append([]byte(nil), payload[offset:offset+size]...)
		offset += size
		if offset%2 == 1 {
			offset++
		}
	}
	return out, nil
}

func readTarGz(payload []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		out[header.Name] = data
	}
	return out, nil
}

func splitLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
