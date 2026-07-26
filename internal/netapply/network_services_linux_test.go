//go:build linux

package netapply

import (
	"reflect"
	"strings"
	"testing"

	"tapx/internal/model"
)

func TestDNSMasqSpecIncludesAddressOptionsAndLeases(t *testing.T) {
	spec, err := dnsmasqSpec("br-tapx", model.DHCPConfig{
		IPv4CIDR: "10.20.0.1/24", PoolStart: "10.20.0.20", PoolEnd: "10.20.0.200",
		Gateway: "10.20.0.1", DNS: []string{"1.1.1.1", "8.8.8.8"}, LeaseSeconds: 3600,
		Authoritative: true, ConflictDetection: true,
		StaticLeases: []model.DHCPStaticLease{{Name: "edge", MAC: "02:00:00:00:00:02", Address: "10.20.0.2"}},
	})
	if err != nil {
		t.Fatalf("dnsmasq spec: %v", err)
	}
	if spec.name != "dnsmasq" || len(spec.files) != 1 {
		t.Fatalf("unexpected service spec: %#v", spec)
	}
	text := string(spec.files[0].data)
	for _, want := range []string{
		"interface=br-tapx", "dhcp-range=10.20.0.20,10.20.0.200,255.255.255.0,3600s",
		"dhcp-authoritative", "dhcp-option=option:router,10.20.0.1",
		"dhcp-option=option:dns-server,1.1.1.1,8.8.8.8",
		"dhcp-host=02:00:00:00:00:02,10.20.0.2,edge",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dnsmasq config %q does not contain %q", text, want)
		}
	}
}

func TestRelaySpecsSeparateAddressFamiliesAndInterfaces(t *testing.T) {
	relays, err := relaySpecs("tapx-tun0", model.TUNDHCPConfig{
		RelayProtocol: "dual", RelayServers: []string{"10.0.0.1", "2001:db8::1"},
		RelayDownstreamInterfaces: []string{"br-lan"}, MaxHops: 8,
	})
	if err != nil {
		t.Fatalf("relay specs: %v", err)
	}
	if len(relays) != 2 || relays[0].name != "dhcrelay" || relays[1].name != "dhcrelay" {
		t.Fatalf("unexpected relay services: %#v", relays)
	}
	if !reflect.DeepEqual(relays[0].args, []string{"-4", "-d", "-q", "--no-pid", "-id", "br-lan", "-iu", "tapx-tun0", "-c", "8", "10.0.0.1"}) {
		t.Fatalf("unexpected DHCPv4 relay args: %#v", relays[0].args)
	}
	if !reflect.DeepEqual(relays[1].args, []string{"-6", "-d", "-q", "--no-pid", "-l", "br-lan", "-u", "2001:db8::1%tapx-tun0"}) {
		t.Fatalf("unexpected DHCPv6 relay args: %#v", relays[1].args)
	}
}

func TestIPTablesPortRangeConversion(t *testing.T) {
	got := iptablesPortRanges([]string{"22", "80-90", "443"})
	want := []string{"22", "80:90", "443"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("iptables ranges = %#v, want %#v", got, want)
	}
}

func TestSharedIPTablesRollbackOrder(t *testing.T) {
	var calls [][]string
	h := &appliedDevice{ifName: "tapx0", runner: func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}}
	if err := h.applySharedIPTables("eth0", []string{"22", "80-90"}, []string{"53"}); err != nil {
		t.Fatalf("apply iptables: %v", err)
	}
	if err := h.Rollback(); err != nil {
		t.Fatalf("rollback iptables: %v", err)
	}
	var flushIndex, deleteIndex = -1, -1
	for i, call := range calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "mangle -F TAPX_TAPX0") {
			flushIndex = i
		}
		if strings.Contains(joined, "mangle -X TAPX_TAPX0") {
			deleteIndex = i
		}
	}
	if flushIndex < 0 || deleteIndex < 0 || flushIndex > deleteIndex {
		t.Fatalf("chain rollback order is invalid: %#v", calls)
	}
}
