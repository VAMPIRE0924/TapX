//go:build linux

package netapply

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"tapx/internal/model"
)

func TestApplyDeviceBuildsIPCommandsAndRollback(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	handle, err := applyDevice(DeviceConfig{
		Type:     model.DeviceTUN,
		IfName:   "tapx0",
		MTU:      1400,
		IPv4CIDR: "10.10.0.1/24",
		IPv6CIDR: "2001:db8::1/64",
	}, runner)
	if err != nil {
		t.Fatalf("apply device: %v", err)
	}

	want := [][]string{
		{"ip", "link", "set", "dev", "tapx0", "mtu", "1400"},
		{"ip", "addr", "add", "10.10.0.1/24", "dev", "tapx0"},
		{"ip", "addr", "add", "2001:db8::1/64", "dev", "tapx0"},
		{"ip", "link", "set", "dev", "tapx0", "up"},
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

func TestApplyDeviceRollsBackAddressesWhenLaterApplyFails(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		call := append([]string{name}, args...)
		calls = append(calls, call)
		if len(args) >= 3 && args[0] == "link" && args[1] == "set" && args[len(args)-1] == "up" {
			return errors.New("up failed")
		}
		return nil
	}

	_, err := applyDevice(DeviceConfig{IfName: "tapx0", IPv4CIDR: "10.10.0.1/24"}, runner)
	if err == nil {
		t.Fatalf("expected apply error")
	}
	want := [][]string{
		{"ip", "addr", "add", "10.10.0.1/24", "dev", "tapx0"},
		{"ip", "link", "set", "dev", "tapx0", "up"},
		{"ip", "addr", "del", "10.10.0.1/24", "dev", "tapx0"},
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

func TestApplyDeviceWritesDNSAndRollbackRemovesNewFile(t *testing.T) {
	var calls [][]string
	runner := func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	outputPath := filepath.Join(t.TempDir(), "tapx.resolv.conf")

	handle, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		DNS: DNSConfig{
			Enabled:       true,
			Nameservers:   []string{"1.1.1.1", "2606:4700:4700::1111"},
			SearchDomains: []string{"example.com", "lan"},
			Options:       []string{"timeout:1", "attempts:2"},
			OutputPath:    outputPath,
		},
	}, runner)
	if err != nil {
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

func TestApplyDeviceRestoresExistingDNSFile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "tapx.resolv.conf")
	original := []byte("nameserver 9.9.9.9\n")
	if err := os.WriteFile(outputPath, original, 0o600); err != nil {
		t.Fatalf("write original DNS file: %v", err)
	}

	handle, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		DNS: DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
			OutputPath:  outputPath,
		},
	}, func(string, ...string) error { return nil })
	if err != nil {
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

func TestApplyDeviceRejectsInvalidDNS(t *testing.T) {
	_, err := applyDevice(DeviceConfig{
		Type:   model.DeviceTUN,
		IfName: "tapx0",
		DNS: DNSConfig{
			Enabled:     true,
			Nameservers: []string{"bad-ip"},
			OutputPath:  filepath.Join(t.TempDir(), "tapx.resolv.conf"),
		},
	}, func(string, ...string) error { return nil })
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
