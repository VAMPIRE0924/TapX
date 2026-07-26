//go:build linux

package netguard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vishvananda/netlink"

	"tapx/internal/config"
	"tapx/internal/model"
)

type networkUse struct {
	owner  string
	kind   string
	name   string
	ifName string
	prefix netip.Prefix
	start  netip.Addr
	end    netip.Addr
}

type inventory struct {
	links []netlink.Link
	uses  []networkUse
}

var scanHost = scanHostInventory

func ValidateConfig(cfg config.RuntimeConfig) error {
	host, err := scanHost()
	if err != nil {
		return fmt.Errorf("network safety scan failed: %w", err)
	}
	return validateConfigAgainst(cfg, host)
}

func scanHostInventory() (inventory, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return inventory{}, fmt.Errorf("list interfaces: %w", err)
	}
	out := inventory{links: links}
	for _, link := range links {
		attrs := link.Attrs()
		if attrs == nil {
			continue
		}
		addrs, addrErr := netlink.AddrList(link, netlink.FAMILY_ALL)
		if addrErr != nil {
			return inventory{}, fmt.Errorf("list addresses on %s: %w", attrs.Name, addrErr)
		}
		for _, addr := range addrs {
			prefix, ok := prefixFromIPNet(addr.IPNet)
			if !ok || prefix.Addr().IsLinkLocalUnicast() || prefix.Addr().IsLoopback() {
				continue
			}
			hostBits := 128
			if prefix.Addr().Is4() {
				hostBits = 32
			}
			out.uses = append(out.uses,
				networkUse{
					kind: "interface network", name: attrs.Name + " " + prefix.Masked().String(),
					ifName: attrs.Name, prefix: prefix.Masked(),
				},
				networkUse{
					kind: "interface address", name: attrs.Name + " " + prefix.String(),
					ifName: attrs.Name, prefix: netip.PrefixFrom(prefix.Addr(), hostBits),
				},
			)
		}
	}
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		routes, routeErr := netlink.RouteList(nil, family)
		if routeErr != nil {
			return inventory{}, fmt.Errorf("list routes: %w", routeErr)
		}
		for _, route := range routes {
			if route.Dst == nil || route.Dst.IP == nil {
				continue
			}
			prefix, ok := prefixFromIPNet(route.Dst)
			if !ok || prefix.Bits() == 0 || prefix.Addr().IsLinkLocalUnicast() || prefix.Addr().IsLoopback() {
				continue
			}
			ifName := ""
			if route.LinkIndex > 0 {
				if link, linkErr := netlink.LinkByIndex(route.LinkIndex); linkErr == nil && link.Attrs() != nil {
					ifName = link.Attrs().Name
				}
			}
			out.uses = append(out.uses, networkUse{
				kind: "route", name: prefix.String() + routeSuffix(ifName, route.Gw),
				ifName: ifName, prefix: prefix.Masked(),
			})
		}
	}
	out.uses = append(out.uses, scanDHCPPools()...)
	return out, nil
}

func validateConfigAgainst(cfg config.RuntimeConfig, host inventory) error {
	var problems []string
	desired := make([]networkUse, 0)
	for _, device := range cfg.Devices {
		if !device.Enabled {
			continue
		}
		if conflict := existingLinkConflict(device, host.links); conflict != "" {
			problems = append(problems, conflict)
		}
		desired = append(desired, deviceNetworkUses(device)...)
	}
	for i, candidate := range desired {
		for _, current := range host.uses {
			ownedInterface := ownsInterface(candidate, current.ifName)
			if ownedInterface && (candidate.prefix.IsValid() || current.kind != "interface address") ||
				!usesOverlap(candidate, current) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"%s conflicts with host %s %s", candidate.name, current.kind, current.name,
			))
		}
		for j := 0; j < i; j++ {
			if candidate.owner != desired[j].owner && usesOverlap(candidate, desired[j]) {
				problems = append(problems, fmt.Sprintf(
					"%s conflicts with %s", candidate.name, desired[j].name,
				))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	problems = compactStrings(problems)
	return &ConflictError{Problems: problems}
}

func existingLinkConflict(device model.Device, links []netlink.Link) string {
	ifName := strings.TrimSpace(device.IfName)
	if ifName == "" {
		return ""
	}
	for _, link := range links {
		if link.Attrs() == nil || link.Attrs().Name != ifName {
			continue
		}
		tuntap, ok := link.(*netlink.Tuntap)
		if !ok {
			return fmt.Sprintf("Device[%s].IfName %s conflicts with existing %s interface", device.ID, ifName, link.Type())
		}
		if device.Type == model.DeviceTAP && tuntap.Mode != netlink.TUNTAP_MODE_TAP ||
			device.Type == model.DeviceTUN && tuntap.Mode != netlink.TUNTAP_MODE_TUN {
			return fmt.Sprintf("Device[%s].IfName %s exists with the other TUN/TAP type", device.ID, ifName)
		}
	}
	return ""
}

func deviceNetworkUses(device model.Device) []networkUse {
	owner := "Device[" + device.ID + "]"
	ifNames := []string{device.IfName}
	if device.Bridge != nil {
		ifNames = append(ifNames, device.Bridge.Name, device.Bridge.IfName)
	}
	addPrefix := func(out []networkUse, field, value string, extraOwners ...string) []networkUse {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return out
		}
		names := append(append([]string(nil), ifNames...), extraOwners...)
		return append(out, networkUse{
			owner: owner,
			kind:  "TapX network", name: owner + "." + field + " " + prefix.String(),
			ifName: strings.Join(compactStrings(names), ","), prefix: prefix.Masked(),
		})
	}
	var out []networkUse
	if device.Type == model.DeviceTAP && device.DHCP != nil && device.DHCP.Mode == model.DHCPModeServer {
		out = addPrefix(out, "DHCP.IPv4CIDR", device.DHCP.IPv4CIDR)
		out = appendPool(out, owner, owner+".DHCP pool", device.DHCP.PoolStart, device.DHCP.PoolEnd, strings.Join(ifNames, ","))
	}
	if device.Type == model.DeviceTUN && device.TUNDHCP != nil {
		switch device.TUNDHCP.Mode {
		case model.TUNDHCPModeManual, model.TUNDHCPModeServer:
			out = addPrefix(out, "TUNDHCP.IPv4CIDR", device.TUNDHCP.IPv4CIDR)
			out = addPrefix(out, "TUNDHCP.IPv6CIDR", device.TUNDHCP.IPv6CIDR)
		}
		if device.TUNDHCP.Mode == model.TUNDHCPModeServer {
			out = appendPool(out, owner, owner+".TUNDHCP IPv4 pool", device.TUNDHCP.PoolStart, device.TUNDHCP.PoolEnd, strings.Join(ifNames, ","))
			out = appendPool(out, owner, owner+".TUNDHCP IPv6 pool", device.TUNDHCP.IPv6PoolStart, device.TUNDHCP.IPv6PoolEnd, strings.Join(ifNames, ","))
		}
	}
	if device.Type == model.DeviceTAP && device.TapMode == model.TapModeSharedIP &&
		device.SharedIP != nil && device.SharedIP.AddressSource == "manual" {
		out = addPrefix(out, "SharedIP.IPv4CIDR", device.SharedIP.IPv4CIDR, device.SharedIP.UplinkInterface)
	}
	return out
}

func appendPool(out []networkUse, owner, name, startText, endText, ifName string) []networkUse {
	start, startErr := netip.ParseAddr(strings.TrimSpace(startText))
	end, endErr := netip.ParseAddr(strings.TrimSpace(endText))
	if startErr != nil || endErr != nil {
		return out
	}
	return append(out, networkUse{owner: owner, kind: "TapX DHCP pool", name: name + " " + start.String() + "-" + end.String(), ifName: ifName, start: start, end: end})
}

func ownsInterface(use networkUse, ifName string) bool {
	if ifName == "" {
		return false
	}
	for _, owner := range strings.Split(use.ifName, ",") {
		if strings.TrimSpace(owner) == ifName {
			return true
		}
	}
	return false
}

func usesOverlap(a, b networkUse) bool {
	switch {
	case a.prefix.IsValid() && b.prefix.IsValid():
		return a.prefix.Addr().BitLen() == b.prefix.Addr().BitLen() &&
			(a.prefix.Contains(b.prefix.Addr()) || b.prefix.Contains(a.prefix.Addr()))
	case a.prefix.IsValid() && b.start.IsValid():
		return rangeOverlapsPrefix(b.start, b.end, a.prefix)
	case a.start.IsValid() && b.prefix.IsValid():
		return rangeOverlapsPrefix(a.start, a.end, b.prefix)
	case a.start.IsValid() && b.start.IsValid():
		return a.start.BitLen() == b.start.BitLen() && a.start.Compare(b.end) <= 0 && b.start.Compare(a.end) <= 0
	default:
		return false
	}
}

func rangeOverlapsPrefix(start, end netip.Addr, prefix netip.Prefix) bool {
	if !start.IsValid() || !end.IsValid() || !prefix.IsValid() ||
		start.BitLen() != end.BitLen() || start.BitLen() != prefix.Addr().BitLen() {
		return false
	}
	return prefix.Contains(start) || prefix.Contains(end) ||
		start.Compare(prefix.Addr()) <= 0 && prefix.Addr().Compare(end) <= 0
}

func prefixFromIPNet(value *net.IPNet) (netip.Prefix, bool) {
	if value == nil {
		return netip.Prefix{}, false
	}
	addr, ok := netip.AddrFromSlice(value.IP)
	if !ok {
		return netip.Prefix{}, false
	}
	ones, bits := value.Mask.Size()
	if ones < 0 || bits != addr.BitLen() {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr.Unmap(), ones), true
}

func routeSuffix(ifName string, gateway net.IP) string {
	var parts []string
	if ifName != "" {
		parts = append(parts, "dev "+ifName)
	}
	if gateway != nil {
		parts = append(parts, "via "+gateway.String())
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func scanDHCPPools() []networkUse {
	patterns := []string{
		"/tmp/etc/dnsmasq.conf*", "/var/etc/dnsmasq.conf*", "/etc/dnsmasq.conf",
		"/etc/dhcp/dhcpd.conf", "/etc/kea/kea-dhcp4.conf", "/etc/kea/kea-dhcp6.conf",
	}
	var out []networkUse
	seen := make(map[string]bool)
	for _, pattern := range patterns {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			for _, pool := range scanDHCPFile(path) {
				key := pool.start.String() + "-" + pool.end.String()
				if seen[key] {
					continue
				}
				seen[key] = true
				pool.kind = "DHCP pool"
				pool.name = path + " " + key
				out = append(out, pool)
			}
		}
	}
	return out
}

func scanDHCPFile(path string) []networkUse {
	if strings.Contains(filepath.Base(path), "kea-") {
		return scanKeaPools(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var out []networkUse
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "dhcp-range="):
			var addresses []netip.Addr
			for _, field := range strings.Split(strings.TrimPrefix(line, "dhcp-range="), ",") {
				if address, parseErr := netip.ParseAddr(strings.TrimSpace(field)); parseErr == nil {
					addresses = append(addresses, address.Unmap())
				}
			}
			if len(addresses) >= 2 {
				out = appendPoolUse(out, addresses[0], addresses[1])
			}
		case strings.HasPrefix(line, "range "):
			fields := strings.Fields(strings.TrimSuffix(line, ";"))
			if len(fields) >= 3 {
				start, startErr := netip.ParseAddr(fields[1])
				end, endErr := netip.ParseAddr(fields[2])
				if startErr == nil && endErr == nil {
					out = appendPoolUse(out, start.Unmap(), end.Unmap())
				}
			}
		}
	}
	return out
}

func scanKeaPools(path string) []networkUse {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var document any
	if json.Unmarshal(data, &document) != nil {
		return nil
	}
	var out []networkUse
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "pool" {
					if text, ok := child.(string); ok {
						parts := strings.SplitN(text, "-", 2)
						if len(parts) == 2 {
							start, startErr := netip.ParseAddr(strings.TrimSpace(parts[0]))
							end, endErr := netip.ParseAddr(strings.TrimSpace(parts[1]))
							if startErr == nil && endErr == nil {
								out = appendPoolUse(out, start.Unmap(), end.Unmap())
							}
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(document)
	return out
}

func appendPoolUse(out []networkUse, start, end netip.Addr) []networkUse {
	if start.IsValid() && end.IsValid() && start.BitLen() == end.BitLen() && start.Compare(end) <= 0 {
		return append(out, networkUse{start: start, end: end})
	}
	return out
}

func compactStrings(values []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
