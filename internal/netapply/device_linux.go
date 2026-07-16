//go:build linux

package netapply

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"tapx/internal/model"
)

type commandRunner func(name string, args ...string) error

var runCommand commandRunner = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, string(out))
	}
	return nil
}

type appliedDevice struct {
	mu                 sync.Mutex
	runner             commandRunner
	ifName             string
	addrs              []string
	bridgeName         string
	bridgeCreated      bool
	tapEnslaved        bool
	memberIfName       string
	bridgeControlRules []firewallRule
	routes             [][]string
	mssRules           []firewallRule
	dns                dnsRollback
}

type firewallRule struct {
	command string
	args    []string
}

type dnsRollback struct {
	path    string
	existed bool
	data    []byte
	mode    os.FileMode
}

func ApplyDevice(cfg DeviceConfig) (Handle, error) {
	return applyDevice(cfg, runCommand)
}

func applyDevice(cfg DeviceConfig, runner commandRunner) (Handle, error) {
	if cfg.IfName == "" {
		return nil, errors.New("netapply: interface name is required")
	}
	if runner == nil {
		runner = runCommand
	}
	handle := &appliedDevice{runner: runner, ifName: cfg.IfName}
	if cfg.MTU > 0 {
		if err := runner("ip", "link", "set", "dev", cfg.IfName, "mtu", fmt.Sprint(cfg.MTU)); err != nil {
			return nil, err
		}
	}
	for _, cidr := range []string{cfg.IPv4CIDR, cfg.IPv6CIDR} {
		if cidr == "" {
			continue
		}
		if _, err := netip.ParsePrefix(cidr); err != nil {
			_ = handle.Rollback()
			return nil, fmt.Errorf("netapply: invalid CIDR %q: %w", cidr, err)
		}
		if err := runner("ip", "addr", "add", cidr, "dev", cfg.IfName); err != nil {
			_ = handle.Rollback()
			return nil, err
		}
		handle.addrs = append(handle.addrs, cidr)
	}
	if cfg.MTU > 0 || cfg.MSSClamp > 0 || cfg.LinkAutoOptimize || cfg.IPv4CIDR != "" || cfg.IPv6CIDR != "" || hasEnabledRoutes(cfg.Routes) {
		if err := runner("ip", "link", "set", "dev", cfg.IfName, "up"); err != nil {
			_ = handle.Rollback()
			return nil, err
		}
	}
	if cfg.Bridge.Enabled {
		if cfg.Type != model.DeviceTAP {
			_ = handle.Rollback()
			return nil, errors.New("netapply: bridge is only valid for tap devices")
		}
		if err := handle.applyBridge(cfg.Bridge); err != nil {
			_ = handle.Rollback()
			return nil, err
		}
	}
	if err := handle.applyRoutes(cfg.Routes); err != nil {
		_ = handle.Rollback()
		return nil, err
	}
	if err := handle.applyMSSClamp(cfg.MSSClamp, cfg.LinkAutoOptimize); err != nil {
		_ = handle.Rollback()
		return nil, err
	}
	if err := handle.applyDNS(cfg.DNS); err != nil {
		_ = handle.Rollback()
		return nil, err
	}
	return handle, nil
}

func (h *appliedDevice) Rollback() error {
	if h == nil || h.runner == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var firstErr error
	if h.dns.path != "" {
		if err := h.rollbackDNS(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for i := len(h.mssRules) - 1; i >= 0; i-- {
		rule := h.mssRules[i]
		if err := h.runner(rule.command, deleteRuleArgs(rule.args)...); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	h.mssRules = nil
	for i := len(h.routes) - 1; i >= 0; i-- {
		args := append([]string{"route", "del"}, h.routes[i]...)
		if err := h.runner("ip", args...); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	h.routes = nil
	for i := len(h.bridgeControlRules) - 1; i >= 0; i-- {
		rule := h.bridgeControlRules[i]
		if err := h.runner(rule.command, rule.args...); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	h.bridgeControlRules = nil
	if h.tapEnslaved {
		if err := h.runner("ip", "link", "set", "dev", h.ifName, "nomaster"); err != nil && firstErr == nil {
			firstErr = err
		}
		h.tapEnslaved = false
	}
	if h.memberIfName != "" {
		if err := h.runner("ip", "link", "set", "dev", h.memberIfName, "nomaster"); err != nil && firstErr == nil {
			firstErr = err
		}
		h.memberIfName = ""
	}
	if h.bridgeCreated && h.bridgeName != "" {
		if err := h.runner("ip", "link", "delete", h.bridgeName, "type", "bridge"); err != nil && firstErr == nil {
			firstErr = err
		}
		h.bridgeCreated = false
		h.bridgeName = ""
	}
	for i := len(h.addrs) - 1; i >= 0; i-- {
		if err := h.runner("ip", "addr", "del", h.addrs[i], "dev", h.ifName); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	h.addrs = nil
	return firstErr
}

func (h *appliedDevice) applyBridge(cfg BridgeConfig) error {
	if cfg.Name == "" {
		return errors.New("netapply: bridge name is required")
	}
	h.bridgeName = cfg.Name
	if err := h.runner("ip", "link", "show", "dev", cfg.Name); err != nil {
		if err := h.runner("ip", "link", "add", "name", cfg.Name, "type", "bridge"); err != nil {
			return err
		}
		h.bridgeCreated = true
	}
	if cfg.MTU > 0 {
		if err := h.runner("ip", "link", "set", "dev", cfg.Name, "mtu", fmt.Sprint(cfg.MTU)); err != nil {
			return err
		}
	}
	// Linux bridges consume IEEE 802.1D link-local multicast by default.
	// Bits 3-15 can be forwarded natively; bits 0-2 need the tc bypass below.
	if err := h.runner("ip", "link", "set", "dev", cfg.Name, "type", "bridge", "group_fwd_mask", "65528"); err != nil {
		return err
	}
	if err := h.runner("ip", "link", "set", "dev", cfg.Name, "up"); err != nil {
		return err
	}
	if err := h.runner("ip", "link", "set", "dev", h.ifName, "master", cfg.Name); err != nil {
		return err
	}
	h.tapEnslaved = true
	if cfg.IfName != "" {
		if err := h.runner("ip", "link", "set", "dev", cfg.IfName, "master", cfg.Name); err != nil {
			return err
		}
		h.memberIfName = cfg.IfName
		if err := h.runner("ip", "link", "set", "dev", cfg.IfName, "up"); err != nil {
			return err
		}
		if err := h.applyRestrictedBridgeControls(h.ifName, cfg.IfName); err != nil {
			return err
		}
	}
	return nil
}

func (h *appliedDevice) applyRestrictedBridgeControls(tapIfName, memberIfName string) error {
	const preference = "62000"
	restricted := []struct {
		handle string
		mac    string
	}{
		{handle: "0x54580001", mac: "01:80:c2:00:00:00"}, // STP
		{handle: "0x54580002", mac: "01:80:c2:00:00:01"}, // MAC control / PAUSE
		{handle: "0x54580003", mac: "01:80:c2:00:00:02"}, // LACP
	}

	for _, direction := range []struct {
		source string
		target string
	}{
		{source: tapIfName, target: memberIfName},
		{source: memberIfName, target: tapIfName},
	} {
		// replace preserves an existing clsact qdisc and its unrelated filters.
		if err := h.runner("tc", "qdisc", "replace", "dev", direction.source, "clsact"); err != nil {
			return err
		}
		for _, control := range restricted {
			args := []string{
				"filter", "replace", "dev", direction.source, "ingress",
				"protocol", "all", "pref", preference, "handle", control.handle,
				"flower", "skip_hw", "dst_mac", control.mac,
				"action", "mirred", "egress", "redirect", "dev", direction.target,
			}
			if err := h.runner("tc", args...); err != nil {
				return err
			}
			h.bridgeControlRules = append(h.bridgeControlRules, firewallRule{
				command: "tc",
				args: []string{
					"filter", "delete", "dev", direction.source, "ingress",
					"protocol", "all", "pref", preference, "handle", control.handle, "flower",
				},
			})
		}
	}
	return nil
}

func (h *appliedDevice) applyDNS(cfg DNSConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if len(cfg.Nameservers) == 0 {
		return errors.New("netapply: DNS nameservers are required")
	}
	outputPath := strings.TrimSpace(cfg.OutputPath)
	if outputPath == "" {
		outputPath = defaultDNSPath(h.ifName)
	}
	if !filepath.IsAbs(outputPath) {
		return errors.New("netapply: DNS output path must be absolute")
	}
	content, err := dnsContent(cfg)
	if err != nil {
		return err
	}

	rollback := dnsRollback{path: outputPath, mode: 0o644}
	if stat, err := os.Stat(outputPath); err == nil {
		rollback.existed = true
		rollback.mode = stat.Mode().Perm()
		data, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("netapply: read existing DNS file %s: %w", outputPath, err)
		}
		rollback.data = data
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("netapply: stat DNS file %s: %w", outputPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("netapply: create DNS dir %s: %w", filepath.Dir(outputPath), err)
	}
	if err := os.WriteFile(outputPath, content, rollback.mode); err != nil {
		return fmt.Errorf("netapply: write DNS file %s: %w", outputPath, err)
	}
	h.dns = rollback
	return nil
}

func (h *appliedDevice) rollbackDNS() error {
	path := h.dns.path
	defer func() { h.dns = dnsRollback{} }()
	if h.dns.existed {
		if err := os.WriteFile(path, h.dns.data, h.dns.mode); err != nil {
			return fmt.Errorf("netapply: restore DNS file %s: %w", path, err)
		}
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("netapply: remove DNS file %s: %w", path, err)
	}
	return nil
}

func defaultDNSPath(ifName string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "\x00", "_").Replace(ifName)
	if safe == "" {
		safe = "tapx"
	}
	return filepath.Join("/run/tapx/resolv", safe+".conf")
}

func dnsContent(cfg DNSConfig) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# generated by tapx\n")
	for _, nameserver := range cfg.Nameservers {
		addr, err := netip.ParseAddr(strings.TrimSpace(nameserver))
		if err != nil {
			return nil, fmt.Errorf("netapply: invalid DNS nameserver %q: %w", nameserver, err)
		}
		b.WriteString("nameserver ")
		b.WriteString(addr.String())
		b.WriteByte('\n')
	}
	if len(cfg.SearchDomains) > 0 {
		b.WriteString("search")
		for _, domain := range cfg.SearchDomains {
			domain = strings.TrimSpace(domain)
			if domain == "" || strings.ContainsAny(domain, " \t\r\n") {
				return nil, fmt.Errorf("netapply: invalid DNS search domain %q", domain)
			}
			b.WriteByte(' ')
			b.WriteString(domain)
		}
		b.WriteByte('\n')
	}
	if len(cfg.Options) > 0 {
		b.WriteString("options")
		for _, option := range cfg.Options {
			option = strings.TrimSpace(option)
			if option == "" || strings.ContainsAny(option, " \t\r\n") {
				return nil, fmt.Errorf("netapply: invalid DNS option %q", option)
			}
			b.WriteByte(' ')
			b.WriteString(option)
		}
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

func (h *appliedDevice) applyMSSClamp(mss int, automatic bool) error {
	if !automatic && mss == 0 {
		return nil
	}
	if !automatic && (mss < 536 || mss > 65535) {
		return errors.New("netapply: MSS clamp must be between 536 and 65535")
	}
	rules := mssClampRules(h.ifName, mss, automatic)
	for _, rule := range rules {
		if err := h.runner(rule.command, rule.args...); err != nil {
			return err
		}
		h.mssRules = append(h.mssRules, rule)
	}
	return nil
}

func (h *appliedDevice) SetMSSClamp(ipv4MSS, ipv6MSS int) error {
	if h == nil || h.runner == nil {
		return errors.New("netapply: device handle is unavailable")
	}
	if ipv4MSS <= 0 || ipv4MSS > 65535 || ipv6MSS <= 0 || ipv6MSS > 65535 {
		return errors.New("netapply: discovered MSS values must be between 1 and 65535")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.replaceMSSRules(fixedMSSClampRules(h.ifName, ipv4MSS, ipv6MSS))
}

func (h *appliedDevice) replaceMSSRules(rules []firewallRule) error {
	old := append([]firewallRule(nil), h.mssRules...)
	for i := len(old) - 1; i >= 0; i-- {
		if err := h.runner(old[i].command, deleteRuleArgs(old[i].args)...); err != nil {
			for j := i + 1; j < len(old); j++ {
				_ = h.runner(old[j].command, old[j].args...)
			}
			return fmt.Errorf("netapply: remove previous MSS clamp: %w", err)
		}
	}
	added := 0
	for _, rule := range rules {
		if err := h.runner(rule.command, rule.args...); err != nil {
			for i := added - 1; i >= 0; i-- {
				_ = h.runner(rules[i].command, deleteRuleArgs(rules[i].args)...)
			}
			for _, previous := range old {
				_ = h.runner(previous.command, previous.args...)
			}
			return fmt.Errorf("netapply: install discovered MSS clamp: %w", err)
		}
		added++
	}
	h.mssRules = rules
	return nil
}

func mssClampRules(ifName string, mss int, automatic bool) []firewallRule {
	out := make([]firewallRule, 0, 4)
	modeArgs := []string{"--set-mss", fmt.Sprint(mss)}
	if automatic {
		modeArgs = []string{"--clamp-mss-to-pmtu"}
	}
	for _, command := range []string{"iptables", "ip6tables"} {
		for _, chain := range []string{"FORWARD", "OUTPUT"} {
			out = append(out, firewallRule{
				command: command,
				args: []string{
					"-t", "mangle",
					"-A", chain,
					"-o", ifName,
					"-p", "tcp",
					"--tcp-flags", "SYN,RST", "SYN",
					"-j", "TCPMSS",
				},
			})
			out[len(out)-1].args = append(out[len(out)-1].args, modeArgs...)
		}
	}
	return out
}

func fixedMSSClampRules(ifName string, ipv4MSS, ipv6MSS int) []firewallRule {
	out := make([]firewallRule, 0, 4)
	for _, family := range []struct {
		command string
		mss     int
	}{{"iptables", ipv4MSS}, {"ip6tables", ipv6MSS}} {
		for _, chain := range []string{"FORWARD", "OUTPUT"} {
			out = append(out, firewallRule{
				command: family.command,
				args: []string{
					"-t", "mangle", "-A", chain, "-o", ifName, "-p", "tcp",
					"--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", fmt.Sprint(family.mss),
				},
			})
		}
	}
	return out
}

func deleteRuleArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out)-1; i++ {
		if out[i] == "-A" {
			out[i] = "-D"
			break
		}
	}
	return out
}

func (h *appliedDevice) applyRoutes(routes []RouteConfig) error {
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		args, err := routeArgs(route, h.ifName)
		if err != nil {
			return err
		}
		if err := h.runner("ip", append([]string{"route", "add"}, args...)...); err != nil {
			return err
		}
		h.routes = append(h.routes, args)
	}
	return nil
}

func routeArgs(route RouteConfig, defaultIfName string) ([]string, error) {
	destination := strings.TrimSpace(route.Destination)
	if destination == "" {
		return nil, errors.New("netapply: route destination is required")
	}
	if destination != "default" {
		if _, err := netip.ParsePrefix(destination); err != nil {
			return nil, fmt.Errorf("netapply: invalid route destination %q: %w", route.Destination, err)
		}
	}
	ifName := strings.TrimSpace(route.IfName)
	if ifName == "" {
		ifName = defaultIfName
	}
	if ifName == "" {
		return nil, errors.New("netapply: route interface is required")
	}

	args := []string{destination}
	if gateway := strings.TrimSpace(route.Gateway); gateway != "" {
		if _, err := netip.ParseAddr(gateway); err != nil {
			return nil, fmt.Errorf("netapply: invalid route gateway %q: %w", route.Gateway, err)
		}
		args = append(args, "via", gateway)
	}
	args = append(args, "dev", ifName)
	if source := strings.TrimSpace(route.Source); source != "" {
		if _, err := netip.ParseAddr(source); err != nil {
			return nil, fmt.Errorf("netapply: invalid route source %q: %w", route.Source, err)
		}
		args = append(args, "src", source)
	}
	if route.Metric > 0 {
		args = append(args, "metric", fmt.Sprint(route.Metric))
	}
	if table := strings.TrimSpace(route.Table); table != "" {
		if strings.ContainsAny(table, " \t\r\n") {
			return nil, errors.New("netapply: route table must not contain whitespace")
		}
		args = append(args, "table", table)
	}
	return args, nil
}
