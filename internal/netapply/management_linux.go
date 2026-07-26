//go:build linux

package netapply

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type protectedManagementRoute struct {
	peer     netip.Addr
	route    netlink.Route
	previous []netlink.Route
}

var (
	netlinkRouteGet          = netlink.RouteGet
	netlinkRouteList         = netlink.RouteList
	netlinkRouteListFiltered = netlink.RouteListFiltered
	netlinkRouteReplace      = netlink.RouteReplace
	netlinkRouteDel          = netlink.RouteDel
	managementPeerSource     = establishedManagementPeers
)

func (h *appliedDevice) protectManagementPaths() error {
	if h == nil || len(h.managementRoutes) > 0 {
		return nil
	}
	for _, peer := range managementPeerSource() {
		routes, err := netlinkRouteGet(net.IP(peer.AsSlice()))
		if err != nil || len(routes) == 0 {
			continue
		}
		selected := routes[0]
		if selected.LinkIndex == 0 {
			continue
		}
		bits := 32
		family := netlink.FAMILY_V4
		if peer.Is6() {
			bits = 128
			family = netlink.FAMILY_V6
		}
		dst := net.IPNet{IP: net.IP(peer.AsSlice()), Mask: net.CIDRMask(bits, bits)}
		previous, _ := netlinkRouteListFiltered(family, &netlink.Route{Dst: &dst}, netlink.RT_FILTER_DST)
		route := netlink.Route{
			LinkIndex: selected.LinkIndex,
			Dst:       &dst,
			Gw:        append(net.IP(nil), selected.Gw...),
			Src:       append(net.IP(nil), selected.Src...),
			Table:     unix.RT_TABLE_MAIN,
			Protocol:  unix.RTPROT_STATIC,
		}
		if err := netlinkRouteReplace(&route); err != nil {
			return fmt.Errorf("netapply: protect management peer %s: %w", peer, err)
		}
		h.managementRoutes = append(h.managementRoutes, protectedManagementRoute{
			peer: peer, route: cloneNetlinkRoute(route), previous: cloneRoutes(previous),
		})
	}
	return nil
}

func (h *appliedDevice) verifyManagementPaths() error {
	for _, protected := range h.managementRoutes {
		routes, err := netlinkRouteGet(net.IP(protected.peer.AsSlice()))
		if err != nil || len(routes) == 0 {
			return fmt.Errorf("netapply: management path to %s is unavailable", protected.peer)
		}
		current := routes[0]
		if current.LinkIndex != protected.route.LinkIndex || !current.Gw.Equal(protected.route.Gw) {
			return fmt.Errorf("netapply: management path to %s changed unexpectedly", protected.peer)
		}
	}
	return nil
}

func (h *appliedDevice) rollbackManagementPaths() error {
	var firstErr error
	for i := len(h.managementRoutes) - 1; i >= 0; i-- {
		protected := h.managementRoutes[i]
		if err := netlinkRouteDel(&protected.route); err != nil &&
			!os.IsNotExist(err) && !errors.Is(err, unix.ESRCH) && firstErr == nil {
			firstErr = err
		}
		for _, previous := range protected.previous {
			route := cloneNetlinkRoute(previous)
			if err := netlinkRouteReplace(&route); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	h.managementRoutes = nil
	return firstErr
}

func snapshotDefaultRoutes(ipv6 bool) ([]netlink.Route, error) {
	family := netlink.FAMILY_V4
	if ipv6 {
		family = netlink.FAMILY_V6
	}
	routes, err := netlinkRouteList(nil, family)
	if err != nil {
		return nil, err
	}
	defaults := make([]netlink.Route, 0)
	for _, route := range routes {
		if route.Dst == nil {
			defaults = append(defaults, cloneNetlinkRoute(route))
			continue
		}
		ones, _ := route.Dst.Mask.Size()
		if ones == 0 {
			defaults = append(defaults, cloneNetlinkRoute(route))
		}
	}
	return defaults, nil
}

func restoreDefaultRoutes(routes []netlink.Route) error {
	var firstErr error
	for _, previous := range routes {
		route := cloneNetlinkRoute(previous)
		if err := netlinkRouteReplace(&route); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func establishedManagementPeers() []netip.Addr {
	seen := make(map[netip.Addr]bool)
	if fields := strings.Fields(os.Getenv("SSH_CONNECTION")); len(fields) >= 1 {
		if address, err := netip.ParseAddr(fields[0]); err == nil && !address.IsLoopback() {
			seen[address.Unmap()] = true
		}
	}
	for _, spec := range []struct {
		path string
		ipv6 bool
	}{{"/proc/net/tcp", false}, {"/proc/net/tcp6", true}} {
		file, err := os.Open(spec.path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		first := true
		for scanner.Scan() {
			if first {
				first = false
				continue
			}
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 || fields[3] != "01" {
				continue
			}
			remote := strings.SplitN(fields[2], ":", 2)[0]
			if address, ok := parseProcAddress(remote, spec.ipv6); ok && !address.IsLoopback() && !address.IsUnspecified() {
				seen[address.Unmap()] = true
			}
		}
		_ = file.Close()
	}
	out := make([]netip.Addr, 0, len(seen))
	for address := range seen {
		out = append(out, address)
	}
	return out
}

func parseProcAddress(value string, ipv6 bool) (netip.Addr, bool) {
	raw, err := hex.DecodeString(value)
	if err != nil {
		return netip.Addr{}, false
	}
	if !ipv6 && len(raw) == 4 {
		raw[0], raw[3] = raw[3], raw[0]
		raw[1], raw[2] = raw[2], raw[1]
		address, ok := netip.AddrFromSlice(raw)
		return address, ok
	}
	if ipv6 && len(raw) == 16 {
		for offset := 0; offset < len(raw); offset += 4 {
			raw[offset], raw[offset+3] = raw[offset+3], raw[offset]
			raw[offset+1], raw[offset+2] = raw[offset+2], raw[offset+1]
		}
		address, ok := netip.AddrFromSlice(raw)
		return address, ok
	}
	return netip.Addr{}, false
}

func cloneRoutes(input []netlink.Route) []netlink.Route {
	out := make([]netlink.Route, 0, len(input))
	for _, route := range input {
		out = append(out, cloneNetlinkRoute(route))
	}
	return out
}

func hasDefaultRoute(routes []RouteConfig) bool {
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		destination := strings.TrimSpace(route.Destination)
		if destination == "default" || destination == "0.0.0.0/0" || destination == "::/0" {
			return true
		}
	}
	return false
}

func managementProtectionRequired(cfg DeviceConfig) bool {
	return cfg.AllowDefaultRoute || hasDefaultRoute(cfg.Routes) ||
		cfg.Bridge.Enabled || cfg.TapMode == "one-arm" || cfg.TapMode == "shared-ip"
}
