//go:build linux

package core

import (
	"fmt"
	"net"
	"net/netip"

	"tapx/internal/config"
	"tapx/internal/fastpath"
)

func fastpathAddressGuard(runtimeGuard config.RuntimeAddressGuard) (fastpath.AddressGuard, error) {
	out := fastpath.AddressGuard{
		IPv4Prefixes: make([]netip.Prefix, 0, len(runtimeGuard.IPv4CIDRs)),
		IPv6Prefixes: make([]netip.Prefix, 0, len(runtimeGuard.IPv6CIDRs)),
		MACs:         make([][6]byte, 0, len(runtimeGuard.MACs)),
	}
	for _, cidr := range runtimeGuard.IPv4CIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fastpath.AddressGuard{}, fmt.Errorf("core: parse IPv4 address guard %q: %w", cidr, err)
		}
		if !prefix.Addr().Is4() {
			return fastpath.AddressGuard{}, fmt.Errorf("core: address guard %q is not IPv4", cidr)
		}
		out.IPv4Prefixes = append(out.IPv4Prefixes, prefix.Masked())
	}
	for _, cidr := range runtimeGuard.IPv6CIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fastpath.AddressGuard{}, fmt.Errorf("core: parse IPv6 address guard %q: %w", cidr, err)
		}
		if !prefix.Addr().Is6() {
			return fastpath.AddressGuard{}, fmt.Errorf("core: address guard %q is not IPv6", cidr)
		}
		out.IPv6Prefixes = append(out.IPv6Prefixes, prefix.Masked())
	}
	for _, value := range runtimeGuard.MACs {
		hw, err := net.ParseMAC(value)
		if err != nil {
			return fastpath.AddressGuard{}, fmt.Errorf("core: parse MAC address guard %q: %w", value, err)
		}
		if len(hw) != 6 {
			return fastpath.AddressGuard{}, fmt.Errorf("core: MAC address guard %q is not 6 bytes", value)
		}
		var mac [6]byte
		copy(mac[:], hw)
		out.MACs = append(out.MACs, mac)
	}
	return out, nil
}
