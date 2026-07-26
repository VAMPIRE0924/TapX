//go:build linux

package netapply

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"tapx/internal/model"
)

type sysctlRollback struct {
	path  string
	value string
}

func (h *appliedDevice) applySharedIP(cfg DeviceConfig) error {
	shared := cfg.SharedIP
	role := shared.Role
	if role == "" {
		if cfg.AccessRole == model.AccessRoleServer {
			role = model.SharedIPRoleService
		} else {
			role = model.SharedIPRoleAccess
		}
	}
	if role == model.SharedIPRoleAccess {
		return nil
	}
	if role != model.SharedIPRoleService {
		return fmt.Errorf("netapply: unsupported shared-IP role %q", role)
	}
	uplink := strings.TrimSpace(shared.UplinkInterface)
	if uplink == "" {
		return errors.New("netapply: shared-IP uplink interface is required")
	}
	address, gateway, err := sharedIPv4Network(shared, uplink)
	if err != nil {
		return err
	}
	if err := h.setSysctl("/proc/sys/net/ipv4/ip_forward", "1\n"); err != nil {
		return err
	}
	for _, name := range []string{"all", uplink, h.ifName} {
		path := filepath.Join("/proc/sys/net/ipv4/conf", name, "rp_filter")
		if err := h.setSysctl(path, "0\n"); err != nil {
			return err
		}
	}
	for _, name := range []string{uplink, h.ifName} {
		path := filepath.Join("/proc/sys/net/ipv4/conf", name, "proxy_arp")
		if err := h.setSysctl(path, "1\n"); err != nil {
			return err
		}
	}
	// The TAP endpoint and the upstream gateway are on the same IPv4 prefix.
	// PVLAN proxy ARP lets the host answer gateway ARP on behalf of the remote
	// endpoint without bridging the only physical interface into the TAP.
	if err := h.setSysctl(filepath.Join("/proc/sys/net/ipv4/conf", h.ifName, "proxy_arp_pvlan"), "1\n"); err != nil {
		return err
	}
	backend := shared.FirewallBackend
	if backend == "" || backend == model.FirewallAuto {
		if _, err := os.Stat("/usr/sbin/nft"); err == nil {
			backend = model.FirewallNFTables
		} else if _, err := os.Stat("/sbin/nft"); err == nil {
			backend = model.FirewallNFTables
		} else {
			backend = model.FirewallIPTables
		}
	}
	tcpPorts := mergePortRanges(shared.ReservedTCPPorts)
	udpPorts := mergePortRanges(shared.ReservedUDPPorts)
	if shared.HostPortPriority {
		tcpPorts = mergePortRanges(tcpPorts, listeningPorts("tcp"))
		udpPorts = mergePortRanges(udpPorts, listeningPorts("udp"))
	}
	switch backend {
	case model.FirewallNFTables:
		err = h.applySharedIPNFT(uplink, tcpPorts, udpPorts)
	case model.FirewallIPTables:
		err = h.applySharedIPTables(uplink, tcpPorts, udpPorts)
	default:
		err = fmt.Errorf("netapply: unsupported firewall backend %q", backend)
	}
	if err != nil {
		return err
	}

	dhcp := model.DHCPConfig{
		Mode: model.DHCPModeServer, IPv4CIDR: address.String(), PoolStart: address.Addr().String(), PoolEnd: address.Addr().String(),
		Gateway: gateway, DNS: append([]string(nil), shared.DNS...), LeaseSeconds: 300,
		Authoritative: true, ConflictDetection: false,
	}
	if shared.ClientMAC != "" {
		dhcp.StaticLeases = []model.DHCPStaticLease{{Name: "tapx-shared", MAC: shared.ClientMAC, Address: address.Addr().String()}}
	}
	spec, err := dnsmasqSpec(h.ifName, dhcp)
	if err != nil {
		return err
	}
	h.serviceMu.Lock()
	serviceIndex, err := h.startServiceLocked(spec)
	h.serviceMu.Unlock()
	if err != nil {
		return err
	}
	if shared.TrackAddressChanges && shared.AddressSource != "manual" {
		if err := h.startSharedIPMonitor(shared, uplink, serviceIndex, address, gateway); err != nil {
			return err
		}
	}
	return nil
}

func sharedIPv4Network(cfg model.SharedIPConfig, uplink string) (netip.Prefix, string, error) {
	if cfg.AddressSource == "manual" {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(cfg.IPv4CIDR))
		if err != nil || !prefix.Addr().Is4() {
			return netip.Prefix{}, "", errors.New("netapply: shared-IP manual IPv4 CIDR is invalid")
		}
		return prefix, strings.TrimSpace(cfg.Gateway), nil
	}
	iface, err := net.InterfaceByName(uplink)
	if err != nil {
		return netip.Prefix{}, "", fmt.Errorf("netapply: inspect shared-IP uplink %s: %w", uplink, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return netip.Prefix{}, "", fmt.Errorf("netapply: inspect shared-IP addresses: %w", err)
	}
	for _, addr := range addrs {
		prefix, err := netip.ParsePrefix(addr.String())
		if err == nil && prefix.Addr().Is4() && !prefix.Addr().IsLoopback() {
			gateway := strings.TrimSpace(cfg.Gateway)
			if gateway == "" {
				gateway = defaultIPv4Gateway(iface.Index)
			}
			return prefix, gateway, nil
		}
	}
	return netip.Prefix{}, "", fmt.Errorf("netapply: uplink %s has no usable IPv4 address", uplink)
}

func defaultIPv4Gateway(linkIndex int) string {
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{LinkIndex: linkIndex}, netlink.RT_FILTER_OIF)
	if err != nil {
		return ""
	}
	for _, route := range routes {
		if route.Dst == nil && route.Gw != nil && route.Gw.To4() != nil {
			return route.Gw.String()
		}
	}
	return ""
}

func (h *appliedDevice) startSharedIPMonitor(cfg model.SharedIPConfig, uplink string, serviceIndex int, current netip.Prefix, gateway string) error {
	ctx, cancel := context.WithCancel(context.Background())
	addressUpdates := make(chan netlink.AddrUpdate, 8)
	routeUpdates := make(chan netlink.RouteUpdate, 8)
	addrDone := make(chan struct{})
	routeDone := make(chan struct{})
	if err := netlink.AddrSubscribe(addressUpdates, addrDone); err != nil {
		cancel()
		return fmt.Errorf("netapply: subscribe shared-IP addresses: %w", err)
	}
	if err := netlink.RouteSubscribe(routeUpdates, routeDone); err != nil {
		close(addrDone)
		cancel()
		return fmt.Errorf("netapply: subscribe shared-IP routes: %w", err)
	}
	h.monitorCancel = cancel
	h.monitorDone = make(chan struct{})
	go func() {
		defer close(h.monitorDone)
		defer close(addrDone)
		defer close(routeDone)
		lastPrefix, lastGateway := current, gateway
		var debounce *time.Timer
		var debounceC <-chan time.Time
		queue := func() {
			if debounce == nil {
				debounce = time.NewTimer(250 * time.Millisecond)
			} else {
				if !debounce.Stop() {
					select {
					case <-debounce.C:
					default:
					}
				}
				debounce.Reset(250 * time.Millisecond)
			}
			debounceC = debounce.C
		}
		for {
			select {
			case <-ctx.Done():
				if debounce != nil {
					debounce.Stop()
				}
				return
			case <-addressUpdates:
				queue()
			case <-routeUpdates:
				queue()
			case <-debounceC:
				debounceC = nil
				prefix, nextGateway, err := sharedIPv4Network(cfg, uplink)
				if err != nil || prefix == lastPrefix && nextGateway == lastGateway {
					continue
				}
				dhcp := model.DHCPConfig{
					Mode: model.DHCPModeServer, IPv4CIDR: prefix.String(), PoolStart: prefix.Addr().String(), PoolEnd: prefix.Addr().String(),
					Gateway: nextGateway, DNS: append([]string(nil), cfg.DNS...), LeaseSeconds: 300,
					Authoritative: true, ConflictDetection: false,
				}
				if cfg.ClientMAC != "" {
					dhcp.StaticLeases = []model.DHCPStaticLease{{Name: "tapx-shared", MAC: cfg.ClientMAC, Address: prefix.Addr().String()}}
				}
				spec, specErr := dnsmasqSpec(h.ifName, dhcp)
				if specErr == nil && h.replaceService(serviceIndex, spec) == nil {
					lastPrefix, lastGateway = prefix, nextGateway
				}
			}
		}
	}()
	return nil
}

func (h *appliedDevice) applySharedIPNFT(uplink string, tcpPorts, udpPorts []string) error {
	table := "tapx_" + safeName(h.ifName)
	if len(table) > 28 {
		table = table[:28]
	}
	mark, routeTable, priority := sharedRouteIDs(h.ifName)
	commands := [][]string{
		{"add", "table", "inet", table},
		{"add", "chain", "inet", table, "prerouting", "{ type filter hook prerouting priority mangle; policy accept; }"},
		{"add", "chain", "inet", table, "forward", "{ type filter hook forward priority filter; policy accept; }"},
		{"add", "rule", "inet", table, "forward", "iifname", h.ifName, "oifname", uplink, "accept"},
		{"add", "rule", "inet", table, "forward", "iifname", uplink, "oifname", h.ifName, "ct", "state", "established,related", "accept"},
	}
	if len(tcpPorts) > 0 {
		commands = append(commands,
			[]string{"add", "set", "inet", table, "reserved_tcp", "{ type inet_service; flags interval; }"},
			[]string{"add", "element", "inet", table, "reserved_tcp", "{ " + strings.Join(tcpPorts, ", ") + " }"},
			[]string{"add", "rule", "inet", table, "prerouting", "iifname", uplink, "fib", "daddr", "type", "local", "tcp", "dport", "@reserved_tcp", "accept"},
		)
	}
	if len(udpPorts) > 0 {
		commands = append(commands,
			[]string{"add", "set", "inet", table, "reserved_udp", "{ type inet_service; flags interval; }"},
			[]string{"add", "element", "inet", table, "reserved_udp", "{ " + strings.Join(udpPorts, ", ") + " }"},
			[]string{"add", "rule", "inet", table, "prerouting", "iifname", uplink, "fib", "daddr", "type", "local", "udp", "dport", "@reserved_udp", "accept"},
		)
	}
	commands = append(commands,
		[]string{"add", "rule", "inet", table, "prerouting", "iifname", uplink, "fib", "daddr", "type", "local", "meta", "l4proto", "tcp", "meta", "mark", "set", mark},
		[]string{"add", "rule", "inet", table, "prerouting", "iifname", uplink, "fib", "daddr", "type", "local", "meta", "l4proto", "udp", "meta", "mark", "set", mark},
	)
	for _, args := range commands {
		if err := h.runner("nft", args...); err != nil {
			return err
		}
	}
	h.rollbackCommands = append(h.rollbackCommands, firewallRule{command: "nft", args: []string{"delete", "table", "inet", table}})
	if err := h.installSharedPolicyRoute(mark, routeTable, priority); err != nil {
		return err
	}
	return nil
}

func (h *appliedDevice) applySharedIPTables(uplink string, tcpPorts, udpPorts []string) error {
	chain := "TAPX_" + strings.ToUpper(safeName(h.ifName))
	if len(chain) > 28 {
		chain = chain[:28]
	}
	mark, routeTable, priority := sharedRouteIDs(h.ifName)
	if err := h.runner("iptables", "-t", "mangle", "-N", chain); err != nil {
		return err
	}
	h.rollbackCommands = append(h.rollbackCommands, firewallRule{command: "iptables", args: []string{"-t", "mangle", "-X", chain}})
	h.rollbackCommands = append(h.rollbackCommands, firewallRule{command: "iptables", args: []string{"-t", "mangle", "-F", chain}})
	if err := h.runner("iptables", "-t", "mangle", "-A", "PREROUTING", "-i", uplink, "-m", "addrtype", "--dst-type", "LOCAL", "-j", chain); err != nil {
		return err
	}
	h.rollbackCommands = append(h.rollbackCommands, firewallRule{command: "iptables", args: []string{"-t", "mangle", "-D", "PREROUTING", "-i", uplink, "-m", "addrtype", "--dst-type", "LOCAL", "-j", chain}})
	for _, group := range splitPortGroups(iptablesPortRanges(tcpPorts), 15) {
		if err := h.runner("iptables", "-t", "mangle", "-A", chain, "-p", "tcp", "-m", "multiport", "--dports", strings.Join(group, ","), "-j", "RETURN"); err != nil {
			return err
		}
	}
	for _, group := range splitPortGroups(iptablesPortRanges(udpPorts), 15) {
		if err := h.runner("iptables", "-t", "mangle", "-A", chain, "-p", "udp", "-m", "multiport", "--dports", strings.Join(group, ","), "-j", "RETURN"); err != nil {
			return err
		}
	}
	for _, protocol := range []string{"tcp", "udp"} {
		if err := h.runner("iptables", "-t", "mangle", "-A", chain, "-p", protocol, "-j", "MARK", "--set-mark", mark); err != nil {
			return err
		}
	}
	if err := h.installSharedPolicyRoute(mark, routeTable, priority); err != nil {
		return err
	}
	return nil
}

func iptablesPortRanges(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.ReplaceAll(value, "-", ":"))
	}
	return out
}

func (h *appliedDevice) installSharedPolicyRoute(mark, table, priority string) error {
	if err := h.runner("ip", "rule", "add", "priority", priority, "fwmark", mark, "lookup", table); err != nil {
		return err
	}
	h.rollbackCommands = append(h.rollbackCommands, firewallRule{command: "ip", args: []string{"rule", "del", "priority", priority, "fwmark", mark, "lookup", table}})
	if err := h.runner("ip", "route", "add", "default", "dev", h.ifName, "table", table); err != nil {
		return err
	}
	h.rollbackCommands = append(h.rollbackCommands, firewallRule{command: "ip", args: []string{"route", "del", "default", "dev", h.ifName, "table", table}})
	return nil
}

func (h *appliedDevice) setSysctl(path, value string) error {
	previous, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("netapply: read sysctl %s: %w", path, err)
	}
	if string(previous) == value {
		return nil
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("netapply: write sysctl %s: %w", path, err)
	}
	h.sysctls = append(h.sysctls, sysctlRollback{path: path, value: string(previous)})
	return nil
}

func sharedRouteIDs(ifName string) (mark, table, priority string) {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(ifName))
	value := hash.Sum32()%1000 + 100
	return fmt.Sprintf("0x%x", value), strconv.Itoa(int(value + 10000)), strconv.Itoa(int(value + 20000))
}

func listeningPorts(protocol string) []string {
	paths := []string{"/proc/net/" + protocol, "/proc/net/" + protocol + "6"}
	seen := map[string]struct{}{}
	for _, path := range paths {
		file, err := os.Open(path)
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
			if len(fields) < 4 || protocol == "tcp" && fields[3] != "0A" {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			port, err := strconv.ParseUint(parts[1], 16, 16)
			if err == nil && port > 0 {
				seen[strconv.Itoa(int(port))] = struct{}{}
			}
		}
		_ = file.Close()
	}
	out := make([]string, 0, len(seen))
	for port := range seen {
		out = append(out, port)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i])
		b, _ := strconv.Atoi(out[j])
		return a < b
	})
	return out
}

func mergePortRanges(groups ...[]string) []string {
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func splitPortGroups(values []string, size int) [][]string {
	var out [][]string
	for len(values) > 0 {
		count := size
		if len(values) < count {
			count = len(values)
		}
		out = append(out, append([]string(nil), values[:count]...))
		values = values[count:]
	}
	return out
}
