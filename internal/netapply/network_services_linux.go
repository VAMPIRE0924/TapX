//go:build linux

package netapply

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tapx/internal/model"
)

type managedService struct {
	cmd  *exec.Cmd
	done chan error
	once sync.Once
}

func startManagedService(name string, args ...string) (*managedService, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("netapply: start %s: %w", name, err)
	}
	service := &managedService{cmd: cmd, done: make(chan error, 1)}
	go func() { service.done <- cmd.Wait() }()
	select {
	case err := <-service.done:
		if err == nil {
			err = errors.New("process exited before becoming ready")
		}
		return nil, fmt.Errorf("netapply: start %s: %w", name, err)
	case <-time.After(100 * time.Millisecond):
		return service, nil
	}
}

func (s *managedService) stop() error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	var result error
	s.once.Do(func() {
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
			result = err
		}
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && result == nil {
				result = err
			}
			select {
			case <-s.done:
			case <-time.After(time.Second):
			}
		}
	})
	return result
}

type serviceSpec struct {
	name  string
	args  []string
	files []generatedServiceFile
}

type generatedServiceFile struct {
	path string
	mode os.FileMode
	data []byte
}

func (h *appliedDevice) applyNetworkAccess(cfg DeviceConfig) error {
	if cfg.Type == model.DeviceTAP && cfg.DHCP.Mode == model.DHCPModeServer {
		serviceIf := h.ifName
		if cfg.Bridge.Enabled {
			serviceIf = cfg.Bridge.Name
		}
		if cfg.DHCP.IPv4CIDR != "" {
			if err := h.addAddress(serviceIf, cfg.DHCP.IPv4CIDR); err != nil {
				return err
			}
		}
		spec, err := dnsmasqSpec(serviceIf, cfg.DHCP)
		if err != nil {
			return err
		}
		if err := h.startService(spec); err != nil {
			return err
		}
	}
	if cfg.Type == model.DeviceTUN {
		if err := h.applyTUNAddressService(cfg); err != nil {
			return err
		}
	}
	if cfg.Type == model.DeviceTAP && cfg.TapMode == model.TapModeSharedIP {
		if err := h.applySharedIP(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (h *appliedDevice) applyTUNAddressService(cfg DeviceConfig) error {
	tun := cfg.TUNDHCP
	switch tun.Mode {
	case "", model.TUNDHCPModeOff:
	case model.TUNDHCPModeClient:
		// TUN is an L3 interface. TapX obtains its lease over the selected
		// transport control stream after the data endpoint is authenticated.
	case model.TUNDHCPModeManual, model.TUNDHCPModeServer:
		if err := h.ApplyAddressLease(AddressLease{
			IPv4CIDR:          tun.IPv4CIDR,
			IPv6CIDR:          tun.IPv6CIDR,
			Gateway:           tun.Gateway,
			DNS:               append([]string(nil), tun.DNS...),
			AllowDefaultRoute: cfg.AllowDefaultRoute,
		}); err != nil {
			return err
		}
		// Server pool allocation and delivery use TapX control streams rather
		// than DHCP broadcasts on the TUN interface.
	default:
		return fmt.Errorf("netapply: unsupported TUN address mode %q", tun.Mode)
	}
	if tun.RelayEnabled {
		specs, err := relaySpecs(h.ifName, tun)
		if err != nil {
			return err
		}
		for _, spec := range specs {
			if err := h.startService(spec); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *appliedDevice) addAddress(ifName, cidr string) error {
	if strings.TrimSpace(cidr) == "" {
		return nil
	}
	if _, err := netip.ParsePrefix(cidr); err != nil {
		return fmt.Errorf("netapply: invalid CIDR %q: %w", cidr, err)
	}
	if err := h.runner("ip", "addr", "add", cidr, "dev", ifName); err != nil {
		return err
	}
	h.addrs = append(h.addrs, appliedAddress{ifName: ifName, cidr: cidr})
	return nil
}

func (h *appliedDevice) startService(spec serviceSpec) error {
	h.serviceMu.Lock()
	defer h.serviceMu.Unlock()
	_, err := h.startServiceLocked(spec)
	return err
}

func (h *appliedDevice) startServiceLocked(spec serviceSpec) (int, error) {
	for _, file := range spec.files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return -1, err
		}
		if err := os.WriteFile(file.path, file.data, file.mode); err != nil {
			return -1, fmt.Errorf("netapply: write %s: %w", file.path, err)
		}
		h.generatedFiles = append(h.generatedFiles, file.path)
	}
	service, err := startManagedService(spec.name, spec.args...)
	if err != nil {
		return -1, err
	}
	h.services = append(h.services, service)
	return len(h.services) - 1, nil
}

func (h *appliedDevice) replaceService(index int, spec serviceSpec) error {
	h.serviceMu.Lock()
	defer h.serviceMu.Unlock()
	if index < 0 || index >= len(h.services) {
		return errors.New("netapply: managed service index is invalid")
	}
	if err := h.services[index].stop(); err != nil {
		return err
	}
	for _, file := range spec.files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file.path, file.data, file.mode); err != nil {
			return fmt.Errorf("netapply: write %s: %w", file.path, err)
		}
	}
	service, err := startManagedService(spec.name, spec.args...)
	if err != nil {
		return err
	}
	h.services[index] = service
	return nil
}

func dnsmasqSpec(ifName string, cfg model.DHCPConfig) (serviceSpec, error) {
	prefix, err := netip.ParsePrefix(cfg.IPv4CIDR)
	if err != nil {
		return serviceSpec{}, fmt.Errorf("netapply: DHCP interface CIDR: %w", err)
	}
	start, err := netip.ParseAddr(cfg.PoolStart)
	if err != nil || !prefix.Contains(start) {
		return serviceSpec{}, errors.New("netapply: DHCP pool start is outside the interface subnet")
	}
	end, err := netip.ParseAddr(cfg.PoolEnd)
	if err != nil || !prefix.Contains(end) || start.Compare(end) > 0 {
		return serviceSpec{}, errors.New("netapply: DHCP pool end is invalid")
	}
	lease := cfg.LeaseSeconds
	if lease <= 0 {
		lease = 86400
	}
	var b strings.Builder
	b.WriteString("port=0\nbind-dynamic\ninterface=" + ifName + "\nexcept-interface=lo\n")
	b.WriteString(fmt.Sprintf("dhcp-range=%s,%s,%s,%ds\n", start, end, netmask(prefix.Bits()), lease))
	if cfg.Authoritative {
		b.WriteString("dhcp-authoritative\n")
	}
	if !cfg.ConflictDetection {
		b.WriteString("no-ping\n")
	}
	if gateway := strings.TrimSpace(cfg.Gateway); gateway != "" {
		b.WriteString("dhcp-option=option:router," + gateway + "\n")
	}
	if len(cfg.DNS) > 0 {
		b.WriteString("dhcp-option=option:dns-server," + strings.Join(cfg.DNS, ",") + "\n")
	}
	for _, lease := range cfg.StaticLeases {
		line := "dhcp-host=" + strings.TrimSpace(lease.MAC) + "," + strings.TrimSpace(lease.Address)
		if lease.Name != "" {
			line += "," + strings.TrimSpace(lease.Name)
		}
		b.WriteString(line + "\n")
	}
	path := filepath.Join("/run/tapx", "dnsmasq-"+safeName(ifName)+".conf")
	return serviceSpec{name: "dnsmasq", args: []string{"--keep-in-foreground", "--conf-file=" + path}, files: []generatedServiceFile{{path: path, mode: 0o600, data: []byte(b.String())}}}, nil
}

func relaySpecs(ifName string, cfg model.TUNDHCPConfig) ([]serviceSpec, error) {
	protocol := cfg.RelayProtocol
	if protocol == "" {
		protocol = "ipv4"
	}
	downstreams := append([]string(nil), cfg.RelayDownstreamInterfaces...)
	if len(downstreams) == 0 {
		return nil, errors.New("netapply: DHCP relay requires an explicit downstream LAN or bridge interface")
	}
	if len(cfg.RelayServers) == 0 {
		return nil, errors.New("netapply: DHCP relay servers are required")
	}
	var out []serviceSpec
	if protocol == "ipv4" || protocol == "dual" {
		args := []string{"-4", "-d", "-q", "--no-pid"}
		for _, downstream := range downstreams {
			args = append(args, "-id", downstream)
		}
		args = append(args, "-iu", ifName)
		if cfg.MaxHops > 0 {
			args = append(args, "-c", strconv.Itoa(cfg.MaxHops))
		}
		serverCount := 0
		for _, server := range cfg.RelayServers {
			address, err := netip.ParseAddr(strings.TrimSpace(server))
			if err == nil && address.Is4() {
				args = append(args, address.String())
				serverCount++
			}
		}
		if serverCount == 0 {
			return nil, errors.New("netapply: DHCPv4 relay requires an IPv4 server")
		}
		out = append(out, serviceSpec{name: "dhcrelay", args: args})
	}
	if protocol == "ipv6" || protocol == "dual" {
		args := []string{"-6", "-d", "-q", "--no-pid"}
		for _, downstream := range downstreams {
			args = append(args, "-l", downstream)
		}
		serverCount := 0
		for _, server := range cfg.RelayServers {
			address, err := netip.ParseAddr(strings.TrimSpace(server))
			if err == nil && address.Is6() {
				args = append(args, "-u", address.String()+"%"+ifName)
				serverCount++
			}
		}
		if serverCount == 0 {
			return nil, errors.New("netapply: DHCPv6 relay requires an IPv6 server")
		}
		out = append(out, serviceSpec{name: "dhcrelay", args: args})
	}
	return out, nil
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "tapx"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func netmask(bits int) string {
	if bits < 0 || bits > 32 {
		return "255.255.255.0"
	}
	mask := uint32(0)
	if bits > 0 {
		mask = ^uint32(0) << (32 - bits)
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask))
}
