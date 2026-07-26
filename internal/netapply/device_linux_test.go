//go:build linux

package netapply

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"tapx/internal/model"
)

func TestMain(m *testing.M) {
	managementPeerSource = func() []netip.Addr { return nil }
	os.Exit(m.Run())
}

func TestApplyDeviceBuildsIPCommandsAndRollback(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	handle, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		MTU:    1400,
		TUNDHCP: model.TUNDHCPConfig{
			Mode: model.TUNDHCPModeManual, IPv4CIDR: "10.10.0.1/24", IPv6CIDR: "2001:db8::1/64",
		},
	}, runner)
	if err != nil {
		t.Fatalf("apply device: %v", err)
	}

	want := [][]string{
		{"ip", "link", "set", "dev", "tapx0", "mtu", "1400"},
		{"ip", "link", "set", "dev", "tapx0", "up"},
		{"ip", "addr", "replace", "10.10.0.1/24", "dev", "tapx0"},
		{"ip", "addr", "replace", "2001:db8::1/64", "dev", "tapx0"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}

	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	want = append(want,
		[]string{"ip", "addr", "del", "2001:db8::1/64", "dev", "tapx0"},
		[]string{"ip", "addr", "del", "10.10.0.1/24", "dev", "tapx0"},
	)
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after rollback = %#v, want %#v", calls, want)
	}
}

func TestApplyDeviceStopsWhenLinkActivationFails(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		if len(args) >= 3 && args[0] == "link" && args[1] == "set" && args[len(args)-1] == "up" {
			return errors.New("up failed")
		}
		return nil
	}

	_, err := applyDevice(DeviceConfig{
		Type: model.DeviceTUN, IfName: "tapx0",
		TUNDHCP: model.TUNDHCPConfig{Mode: model.TUNDHCPModeManual, IPv4CIDR: "10.10.0.1/24"},
	}, runner)
	if err == nil {
		t.Fatalf("expected apply error")
	}
	want := [][]string{
		{"ip", "link", "set", "dev", "tapx0", "up"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestApplyDeviceBuildsBridgeCommandsAndRollback(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		if slices.Equal(call, []string{"ip", "link", "show", "dev", "brx0"}) {
			return errors.New("missing bridge")
		}
		return nil
	}

	handle, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTAP,
		IfName: "tapx0",
		Bridge: BridgeConfig{
			Enabled: true,
			Name:    "brx0",
			IfName:  "eth1",
			MTU:     1400,
		},
	}, runner)
	if err != nil {
		t.Fatalf("apply device bridge: %v", err)
	}

	want := [][]string{
		{"ip", "link", "show", "dev", "brx0"},
		{"ip", "link", "add", "name", "brx0", "type", "bridge"},
		{"ip", "link", "set", "dev", "brx0", "mtu", "1400"},
		{"ip", "link", "set", "dev", "brx0", "type", "bridge", "group_fwd_mask", "65528"},
		{"ip", "link", "set", "dev", "brx0", "up"},
		{"ip", "link", "set", "dev", "tapx0", "master", "brx0"},
		{"ip", "link", "set", "dev", "eth1", "master", "brx0"},
		{"ip", "link", "set", "dev", "eth1", "up"},
		{"tc", "qdisc", "replace", "dev", "tapx0", "clsact"},
		{"tc", "filter", "replace", "dev", "tapx0", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580001", "flower", "skip_hw", "dst_mac", "01:80:c2:00:00:00", "action", "mirred", "egress", "redirect", "dev", "eth1"},
		{"tc", "filter", "replace", "dev", "tapx0", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580002", "flower", "skip_hw", "dst_mac", "01:80:c2:00:00:01", "action", "mirred", "egress", "redirect", "dev", "eth1"},
		{"tc", "filter", "replace", "dev", "tapx0", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580003", "flower", "skip_hw", "dst_mac", "01:80:c2:00:00:02", "action", "mirred", "egress", "redirect", "dev", "eth1"},
		{"tc", "qdisc", "replace", "dev", "eth1", "clsact"},
		{"tc", "filter", "replace", "dev", "eth1", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580001", "flower", "skip_hw", "dst_mac", "01:80:c2:00:00:00", "action", "mirred", "egress", "redirect", "dev", "tapx0"},
		{"tc", "filter", "replace", "dev", "eth1", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580002", "flower", "skip_hw", "dst_mac", "01:80:c2:00:00:01", "action", "mirred", "egress", "redirect", "dev", "tapx0"},
		{"tc", "filter", "replace", "dev", "eth1", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580003", "flower", "skip_hw", "dst_mac", "01:80:c2:00:00:02", "action", "mirred", "egress", "redirect", "dev", "tapx0"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}

	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback bridge: %v", err)
	}
	want = append(want,
		[]string{"tc", "filter", "delete", "dev", "eth1", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580003", "flower"},
		[]string{"tc", "filter", "delete", "dev", "eth1", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580002", "flower"},
		[]string{"tc", "filter", "delete", "dev", "eth1", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580001", "flower"},
		[]string{"tc", "filter", "delete", "dev", "tapx0", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580003", "flower"},
		[]string{"tc", "filter", "delete", "dev", "tapx0", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580002", "flower"},
		[]string{"tc", "filter", "delete", "dev", "tapx0", "ingress", "protocol", "all", "pref", "62000", "handle", "0x54580001", "flower"},
		[]string{"ip", "link", "set", "dev", "tapx0", "nomaster"},
		[]string{"ip", "link", "set", "dev", "eth1", "nomaster"},
		[]string{"ip", "link", "delete", "brx0", "type", "bridge"},
	)
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after rollback = %#v, want %#v", calls, want)
	}
}

func TestApplyDeviceBuildsRouteCommandsAndRollback(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	handle, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		Routes: []RouteConfig{
			{
				Enabled:     true,
				Destination: "10.50.0.0/24",
				Gateway:     "10.10.0.2",
				Source:      "10.10.0.1",
				Metric:      20,
				Table:       "100",
			},
			{
				Enabled:     false,
				Destination: "10.60.0.0/24",
			},
		},
	}, runner)
	if err != nil {
		t.Fatalf("apply device route: %v", err)
	}

	want := [][]string{
		{"ip", "link", "set", "dev", "tapx0", "up"},
		{"ip", "route", "add", "10.50.0.0/24", "via", "10.10.0.2", "dev", "tapx0", "src", "10.10.0.1", "metric", "20", "table", "100"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}

	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback route: %v", err)
	}
	want = append(want,
		[]string{"ip", "route", "del", "10.50.0.0/24", "via", "10.10.0.2", "dev", "tapx0", "src", "10.10.0.1", "metric", "20", "table", "100"},
	)
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after rollback = %#v, want %#v", calls, want)
	}
}

func TestApplyDeviceSharedUsesOneNetworkTransaction(t *testing.T) {
	deviceRegistry.Lock()
	deviceRegistry.items = make(map[string]*sharedDeviceApply)
	deviceRegistry.Unlock()
	t.Cleanup(func() {
		deviceRegistry.Lock()
		deviceRegistry.items = make(map[string]*sharedDeviceApply)
		deviceRegistry.Unlock()
	})

	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	cfg := DeviceConfig{
		Type: model.DeviceTUN, IfName: "tapx-shared0",
		TUNDHCP: model.TUNDHCPConfig{Mode: model.TUNDHCPModeManual, IPv4CIDR: "10.77.0.1/30"},
	}
	first, err := applyDeviceShared(cfg, runner)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := applyDeviceShared(cfg, runner)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if got := len(calls); got != 2 {
		t.Fatalf("network config applied %d commands, want 2: %#v", got, calls)
	}
	if err := first.Rollback(); err != nil {
		t.Fatalf("release first reference: %v", err)
	}
	if got := len(calls); got != 2 {
		t.Fatalf("first release rolled back shared config: %#v", calls)
	}
	if _, err := applyDeviceShared(DeviceConfig{
		Type: model.DeviceTUN, IfName: cfg.IfName,
		TUNDHCP: model.TUNDHCPConfig{Mode: model.TUNDHCPModeManual, IPv4CIDR: "10.77.0.2/30"},
	}, runner); err == nil {
		t.Fatal("different active config was accepted")
	}
	if err := second.Rollback(); err != nil {
		t.Fatalf("release last reference: %v", err)
	}
	if got := len(calls); got != 3 {
		t.Fatalf("final release did not roll back shared config: %#v", calls)
	}
	deviceRegistry.Lock()
	_, exists := deviceRegistry.items[cfg.IfName]
	deviceRegistry.Unlock()
	if exists {
		t.Fatal("shared registry entry remains after final release")
	}
}

func TestSharedDeviceRollbackCanRetryAfterFailure(t *testing.T) {
	deviceRegistry.Lock()
	deviceRegistry.items = make(map[string]*sharedDeviceApply)
	deviceRegistry.Unlock()
	t.Cleanup(func() {
		deviceRegistry.Lock()
		deviceRegistry.items = make(map[string]*sharedDeviceApply)
		deviceRegistry.Unlock()
	})

	failDelete := true
	runner := func(_ string, args ...string) error {
		if len(args) > 1 && args[0] == "addr" && args[1] == "del" && failDelete {
			failDelete = false
			return errors.New("temporary delete failure")
		}
		return nil
	}
	handle, err := applyDeviceShared(DeviceConfig{
		Type: model.DeviceTUN, IfName: "tapx-retry0",
		TUNDHCP: model.TUNDHCPConfig{Mode: model.TUNDHCPModeManual, IPv4CIDR: "10.78.0.1/30"},
	}, runner)
	if err != nil {
		t.Fatalf("apply shared device: %v", err)
	}
	if err := handle.Rollback(); err == nil {
		t.Fatal("expected first rollback to fail")
	}
	if err := handle.Rollback(); err != nil {
		t.Fatalf("retry rollback: %v", err)
	}
	deviceRegistry.Lock()
	_, exists := deviceRegistry.items["tapx-retry0"]
	deviceRegistry.Unlock()
	if exists {
		t.Fatal("shared registry entry remains after successful retry")
	}
}

func TestApplyAddressLeaseReplacesPreviousLeaseAndRollsBackCurrent(t *testing.T) {
	var calls [][]string
	h := &appliedDevice{ifName: "tapx-tun0", allowDefaultRoute: true, runner: func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}}
	if err := h.ApplyAddressLease(AddressLease{IPv4CIDR: "10.80.0.2/24", Gateway: "10.80.0.1", AllowDefaultRoute: true}); err != nil {
		t.Fatalf("apply first lease: %v", err)
	}
	if err := h.ApplyAddressLease(AddressLease{IPv6CIDR: "fd80::2/64", Gateway: "fd80::1", AllowDefaultRoute: true}); err != nil {
		t.Fatalf("apply renewed lease: %v", err)
	}
	if err := h.Rollback(); err != nil {
		t.Fatalf("rollback lease: %v", err)
	}
	wantFragments := []string{
		"ip addr replace 10.80.0.2/24 dev tapx-tun0",
		"ip -4 route replace default via 10.80.0.1 dev tapx-tun0",
		"ip addr replace fd80::2/64 dev tapx-tun0",
		"ip -6 route replace default via fd80::1 dev tapx-tun0",
		"ip addr del 10.80.0.2/24 dev tapx-tun0",
		"ip -6 route del default via fd80::1 dev tapx-tun0",
		"ip addr del fd80::2/64 dev tapx-tun0",
	}
	joined := make([]string, 0, len(calls))
	for _, call := range calls {
		joined = append(joined, strings.Join(call, " "))
	}
	for _, want := range wantFragments {
		if !slices.Contains(joined, want) {
			t.Fatalf("missing command %q in %#v", want, joined)
		}
	}
}

func TestApplyAddressLeaseDoesNotInstallDefaultRouteWithoutPermission(t *testing.T) {
	var calls []string
	h := &appliedDevice{ifName: "tapx-tun0", runner: func(name string, args ...string) error {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		return nil
	}}
	if err := h.ApplyAddressLease(AddressLease{
		IPv4CIDR: "10.81.0.2/24", Gateway: "10.81.0.1", AllowDefaultRoute: true,
	}); err != nil {
		t.Fatalf("apply lease: %v", err)
	}
	for _, call := range calls {
		if strings.Contains(call, "route replace default") {
			t.Fatalf("default route was installed without device permission: %s", call)
		}
	}
}

func TestApplyDeviceBuildsMSSClampCommandsAndRollback(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	handle, err := applyDevice(DeviceConfig{
		Type:     model.DeviceTUN,
		IfName:   "tapx0",
		MSSClamp: 1360,
	}, runner)
	if err != nil {
		t.Fatalf("apply mss clamp: %v", err)
	}

	want := [][]string{
		{"ip", "link", "set", "dev", "tapx0", "up"},
		{"iptables", "-t", "mangle", "-A", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
		{"ip6tables", "-t", "mangle", "-A", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
		{"ip6tables", "-t", "mangle", "-A", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}

	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback mss clamp: %v", err)
	}
	want = append(want,
		[]string{"ip6tables", "-t", "mangle", "-D", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
		[]string{"ip6tables", "-t", "mangle", "-D", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
		[]string{"iptables", "-t", "mangle", "-D", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
		[]string{"iptables", "-t", "mangle", "-D", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1360"},
	)
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after rollback = %#v, want %#v", calls, want)
	}
}

func TestApplyDeviceBuildsAutomaticPMTUMSSCommands(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	handle, err := applyDevice(DeviceConfig{
		Type:             model.DeviceTUN,
		IfName:           "tapx0",
		LinkAutoOptimize: true,
	}, runner)
	if err != nil {
		t.Fatalf("apply automatic MSS optimization: %v", err)
	}

	want := [][]string{
		{"ip", "link", "set", "dev", "tapx0", "up"},
		{"iptables", "-t", "mangle", "-A", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		{"ip6tables", "-t", "mangle", "-A", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		{"ip6tables", "-t", "mangle", "-A", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}

	if err := handle.SetMSSClamp(1412, 1392); err != nil {
		t.Fatalf("replace automatic MSS optimization: %v", err)
	}
	want = append(want,
		[]string{"ip6tables", "-t", "mangle", "-D", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		[]string{"ip6tables", "-t", "mangle", "-D", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		[]string{"iptables", "-t", "mangle", "-D", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		[]string{"iptables", "-t", "mangle", "-D", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		[]string{"iptables", "-t", "mangle", "-A", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1412"},
		[]string{"iptables", "-t", "mangle", "-A", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1412"},
		[]string{"ip6tables", "-t", "mangle", "-A", "FORWARD", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1392"},
		[]string{"ip6tables", "-t", "mangle", "-A", "OUTPUT", "-o", "tapx0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", "1392"},
	)
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls after discovered MSS = %#v, want %#v", calls, want)
	}

	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback automatic MSS optimization: %v", err)
	}
}

func TestApplyDNSAndRollbackRemovesNewFile(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	outputPath := filepath.Join(t.TempDir(), "tapx.resolv.conf")

	handle := &appliedDevice{ifName: "tapx0", runner: runner}
	if err := handle.applyDNS(DNSConfig{
		Enabled:       true,
		Nameservers:   []string{"1.1.1.1", "2606:4700:4700::1111"},
		SearchDomains: []string{"example.com", "lan"},
		Options:       []string{"timeout:1", "attempts:2"},
		OutputPath:    outputPath,
	}); err != nil {
		t.Fatalf("apply DNS: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %#v, want no shell commands for DNS-only apply", calls)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read DNS output: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"nameserver 1.1.1.1",
		"nameserver 2606:4700:4700::1111",
		"search example.com lan",
		"options timeout:1 attempts:2",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("DNS output = %q, want %q", text, want)
		}
	}

	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback DNS: %v", err)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("DNS output stat err = %v, want not exist", err)
	}
}

func TestApplyDNSRestoresExistingFile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "tapx.resolv.conf")
	original := []byte("nameserver 9.9.9.9\n")
	if err := os.WriteFile(outputPath, original, 0o600); err != nil {
		t.Fatalf("write original DNS file: %v", err)
	}

	handle := &appliedDevice{ifName: "tapx0", runner: func(string, ...string) error { return nil }}
	if err := handle.applyDNS(DNSConfig{
		Enabled: true, Nameservers: []string{"1.1.1.1"}, OutputPath: outputPath,
	}); err != nil {
		t.Fatalf("apply DNS: %v", err)
	}
	if err := handle.Rollback(); err != nil {
		t.Fatalf("rollback DNS: %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read restored DNS file: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("restored DNS = %q, want %q", content, original)
	}
	stat, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat restored DNS file: %v", err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode = %v, want 0600", stat.Mode().Perm())
	}
}

func TestApplyDNSRejectsInvalidNameserver(t *testing.T) {
	handle := &appliedDevice{ifName: "tapx0", runner: func(string, ...string) error { return nil }}
	err := handle.applyDNS(DNSConfig{
		Enabled: true, Nameservers: []string{"bad-ip"}, OutputPath: filepath.Join(t.TempDir(), "tapx.resolv.conf"),
	})
	if err == nil {
		t.Fatalf("expected invalid DNS error")
	}
}

func TestApplyDeviceRejectsInvalidMSSClamp(t *testing.T) {
	_, err := applyDevice(DeviceConfig{
		Type:     model.DeviceTUN,
		IfName:   "tapx0",
		MSSClamp: 10,
	}, func(string, ...string) error { return nil })
	if err == nil {
		t.Fatalf("expected invalid mss clamp error")
	}
}

func TestApplyDeviceRejectsInvalidRoute(t *testing.T) {
	_, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		Routes: []RouteConfig{{
			Enabled:     true,
			Destination: "bad",
		}},
	}, func(string, ...string) error { return nil })
	if err == nil {
		t.Fatalf("expected invalid route error")
	}
}

func TestApplyDeviceRejectsBridgeOnTUN(t *testing.T) {
	_, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		Bridge: BridgeConfig{
			Enabled: true,
			Name:    "brx0",
		},
	}, func(string, ...string) error { return nil })
	if err == nil {
		t.Fatalf("expected bridge on TUN error")
	}
}
