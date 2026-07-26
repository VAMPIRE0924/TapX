//go:build linux

package netapply

import (
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type oneArmState struct {
	member     netlink.Link
	bridge     netlink.Link
	bridgeName string
	addrs      []netlink.Addr
	routes     []netlink.Route
	moved      bool
}

func snapshotOneArmState(memberName, bridgeName string) (*oneArmState, error) {
	if memberName == "" || bridgeName == "" {
		return nil, errors.New("netapply: one-arm member and bridge are required")
	}
	member, err := netlink.LinkByName(memberName)
	if err != nil {
		return nil, fmt.Errorf("netapply: inspect one-arm member %s: %w", memberName, err)
	}
	// The bridge may not exist yet. It is resolved again after applyBridge.
	addrs, err := netlink.AddrList(member, netlink.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("netapply: list one-arm addresses: %w", err)
	}
	filteredAddrs := addrs[:0]
	for _, addr := range addrs {
		ip := addr.IP
		if ip == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		filteredAddrs = append(filteredAddrs, addr)
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{LinkIndex: member.Attrs().Index}, netlink.RT_FILTER_OIF)
	if err != nil {
		return nil, fmt.Errorf("netapply: list one-arm routes: %w", err)
	}
	filteredRoutes := routes[:0]
	for _, route := range routes {
		if route.Protocol == unix.RTPROT_KERNEL || route.Type == unix.RTN_LOCAL {
			continue
		}
		filteredRoutes = append(filteredRoutes, route)
	}
	return &oneArmState{member: member, bridgeName: bridgeName, addrs: append([]netlink.Addr(nil), filteredAddrs...), routes: append([]netlink.Route(nil), filteredRoutes...)}, nil
}

func (s *oneArmState) migrate() error {
	if s == nil || s.member == nil {
		return errors.New("netapply: one-arm state is unavailable")
	}
	member, err := netlink.LinkByName(s.member.Attrs().Name)
	if err != nil {
		return fmt.Errorf("netapply: refresh one-arm member: %w", err)
	}
	s.member = member
	if member.Attrs().MasterIndex <= 0 {
		return errors.New("netapply: one-arm member is not attached to a bridge")
	}
	bridge, err := netlink.LinkByName(s.bridgeName)
	if err != nil {
		return fmt.Errorf("netapply: inspect one-arm bridge %s: %w", s.bridgeName, err)
	}
	s.bridge = bridge
	s.moved = true
	for _, original := range s.addrs {
		addr := cloneNetlinkAddr(original)
		if err := netlink.AddrReplace(bridge, &addr); err != nil {
			return fmt.Errorf("netapply: move address %s to %s: %w", original.String(), s.bridgeName, err)
		}
		if err := netlink.AddrDel(s.member, &original); err != nil {
			return fmt.Errorf("netapply: remove address %s from %s: %w", original.String(), s.member.Attrs().Name, err)
		}
	}
	for _, original := range s.routes {
		route := cloneNetlinkRoute(original)
		route.LinkIndex = bridge.Attrs().Index
		if route.ILinkIndex == s.member.Attrs().Index {
			route.ILinkIndex = bridge.Attrs().Index
		}
		if err := netlink.RouteReplace(&route); err != nil {
			return fmt.Errorf("netapply: move route to %s: %w", s.bridgeName, err)
		}
	}
	return nil
}

func (s *oneArmState) rollback() error {
	if s == nil || !s.moved || s.member == nil || s.bridge == nil {
		return nil
	}
	var firstErr error
	for _, original := range s.addrs {
		addr := cloneNetlinkAddr(original)
		if err := netlink.AddrReplace(s.member, &addr); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("netapply: restore address %s: %w", original.String(), err)
		}
		bridgeAddr := cloneNetlinkAddr(original)
		if err := netlink.AddrDel(s.bridge, &bridgeAddr); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("netapply: remove bridge address %s: %w", original.String(), err)
		}
	}
	for _, original := range s.routes {
		route := cloneNetlinkRoute(original)
		if err := netlink.RouteReplace(&route); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("netapply: restore one-arm route: %w", err)
		}
	}
	s.moved = false
	return firstErr
}

func cloneNetlinkAddr(input netlink.Addr) netlink.Addr {
	out := input
	if input.IPNet != nil {
		out.IPNet = &net.IPNet{IP: append(net.IP(nil), input.IPNet.IP...), Mask: append(net.IPMask(nil), input.IPNet.Mask...)}
	}
	return out
}

func cloneNetlinkRoute(input netlink.Route) netlink.Route {
	out := input
	out.Src = append(net.IP(nil), input.Src...)
	out.Gw = append(net.IP(nil), input.Gw...)
	if input.Dst != nil {
		out.Dst = &net.IPNet{IP: append(net.IP(nil), input.Dst.IP...), Mask: append(net.IPMask(nil), input.Dst.Mask...)}
	}
	out.MultiPath = append([]*netlink.NexthopInfo(nil), input.MultiPath...)
	return out
}
