//go:build linux

package netguard

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestValidateConfigRejectsHostIPv4AndIPv6Conflicts(t *testing.T) {
	host := inventory{uses: []networkUse{
		{kind: "route", name: "10.20.0.0/16 dev eth0", ifName: "eth0", prefix: mustPrefix("10.20.0.0/16")},
		{kind: "route", name: "fd20::/32 dev eth0", ifName: "eth0", prefix: mustPrefix("fd20::/32")},
	}}
	cfg := config.RuntimeConfig{Devices: []model.Device{
		{ID: "tun4", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-tun4", AccessRole: model.AccessRoleClient,
			TUNDHCP: &model.TUNDHCPConfig{Mode: model.TUNDHCPModeManual, Protocol: "ipv4", IPv4CIDR: "10.20.1.2/24"}},
		{ID: "tun6", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-tun6", AccessRole: model.AccessRoleClient,
			TUNDHCP: &model.TUNDHCPConfig{Mode: model.TUNDHCPModeManual, Protocol: "ipv6", IPv6CIDR: "fd20:0:1::2/64"}},
	}}
	err := validateConfigAgainst(cfg, host)
	if err == nil || !strings.Contains(err.Error(), "tun4") || !strings.Contains(err.Error(), "tun6") ||
		!strings.Contains(err.Error(), "eth0") {
		t.Fatalf("expected explicit IPv4 and IPv6 conflicts, got %v", err)
	}
}

func TestValidateConfigRejectsInterfaceNetworkWithoutRoute(t *testing.T) {
	host := inventory{uses: []networkUse{{
		kind: "interface network", name: "br-lan 192.168.1.0/24",
		ifName: "br-lan", prefix: mustPrefix("192.168.1.0/24"),
	}}}
	cfg := config.RuntimeConfig{Devices: []model.Device{{
		ID: "tun-lab", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-tun0",
		AccessRole: model.AccessRoleClient,
		TUNDHCP: &model.TUNDHCPConfig{
			Mode: model.TUNDHCPModeManual, Protocol: "ipv4", IPv4CIDR: "192.168.1.2/24",
		},
	}}}
	err := validateConfigAgainst(cfg, host)
	if err == nil || !strings.Contains(err.Error(), "br-lan 192.168.1.0/24") {
		t.Fatalf("expected interface subnet conflict without a route, got %v", err)
	}
}

func TestValidateConfigAllowsOwnedDHCPNetworkButRejectsHostAddressInPool(t *testing.T) {
	base := model.Device{
		ID: "tap", Enabled: true, Type: model.DeviceTAP, IfName: "tapx-tap0",
		AccessRole: model.AccessRoleServer, TapMode: model.TapModeStandalone,
		DHCP: &model.DHCPConfig{
			Mode: model.DHCPModeServer, IPv4CIDR: "10.30.0.1/24",
			PoolStart: "10.30.0.10", PoolEnd: "10.30.0.20",
		},
	}
	host := inventory{uses: []networkUse{
		{kind: "interface address", name: "tapx-tap0 10.30.0.1/24", ifName: "tapx-tap0", prefix: mustPrefix("10.30.0.1/32")},
		{kind: "route", name: "10.30.0.0/24 dev tapx-tap0", ifName: "tapx-tap0", prefix: mustPrefix("10.30.0.0/24")},
	}}
	if err := validateConfigAgainst(config.RuntimeConfig{Devices: []model.Device{base}}, host); err != nil {
		t.Fatalf("owned interface network should be accepted: %v", err)
	}
	host.uses[0].prefix = mustPrefix("10.30.0.15/32")
	host.uses[0].name = "tapx-tap0 10.30.0.15/24"
	if err := validateConfigAgainst(config.RuntimeConfig{Devices: []model.Device{base}}, host); err == nil ||
		!strings.Contains(err.Error(), "10.30.0.15") {
		t.Fatalf("expected pool conflict with owned host address, got %v", err)
	}
}

func TestValidateConfigAllowsOneArmAndSharedUplinkNetworks(t *testing.T) {
	host := inventory{uses: []networkUse{
		{kind: "route", name: "192.0.2.0/24 dev eth0", ifName: "eth0", prefix: mustPrefix("192.0.2.0/24")},
	}}
	cfg := config.RuntimeConfig{Devices: []model.Device{
		{ID: "one-arm", Enabled: true, Type: model.DeviceTAP, IfName: "tapx-one", TapMode: model.TapModeOneArm,
			Bridge:     &model.BridgeConfig{Enabled: true, Name: "br-tapx", IfName: "eth0"},
			AccessRole: model.AccessRoleServer},
		{ID: "shared", Enabled: true, Type: model.DeviceTAP, IfName: "tapx-shared", TapMode: model.TapModeSharedIP,
			AccessRole: model.AccessRoleServer,
			SharedIP: &model.SharedIPConfig{Role: model.SharedIPRoleService, UplinkInterface: "eth0",
				AddressSource: "manual", IPv4CIDR: "192.0.2.10/24"}},
	}}
	if err := validateConfigAgainst(cfg, host); err != nil {
		t.Fatalf("one-arm/shared uplink network should be accepted: %v", err)
	}
}

func TestValidateConfigRejectsExistingPhysicalInterfaceName(t *testing.T) {
	host := inventory{links: []netlink.Link{&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}}}
	cfg := config.RuntimeConfig{Devices: []model.Device{{
		ID: "bad", Enabled: true, Type: model.DeviceTUN, IfName: "eth0",
	}}}
	err := validateConfigAgainst(cfg, host)
	if err == nil || !strings.Contains(err.Error(), "existing dummy interface") {
		t.Fatalf("expected existing physical interface conflict, got %v", err)
	}
}

func mustPrefix(value string) netip.Prefix {
	return netip.MustParsePrefix(value)
}
