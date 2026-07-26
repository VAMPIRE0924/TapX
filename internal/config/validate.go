package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"tapx/internal/model"
)

type Problem struct {
	Object  string
	ID      string
	Field   string
	Message string
}

type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		parts = append(parts, fmt.Sprintf("%s[%s].%s: %s", p.Object, p.ID, p.Field, p.Message))
	}
	return strings.Join(parts, "; ")
}

func ValidateForSave(cfg RuntimeConfig) error {
	v := &validator{cfg: cfg}
	v.index()
	v.validateDevices()
	v.validateVKeys()
	v.validateRoutes()
	v.validateXrayProfiles()
	v.validateSettings()
	v.validateListeners()
	v.validateConnectors()
	v.validateClients()
	v.validateAddressLimits()
	v.validateConnectorRouteBindings()
	return v.err()
}

func ValidateForApply(cfg RuntimeConfig) error {
	v := &validator{cfg: cfg, apply: true}
	v.index()
	v.validateDevices()
	v.validateVKeys()
	v.validateRoutes()
	v.validateXrayProfiles()
	v.validateSettings()
	v.validateListeners()
	v.validateConnectors()
	v.validateClients()
	v.validateAddressLimits()
	v.validateConnectorRouteBindings()
	v.enabledReferences()
	return v.err()
}

func (v *validator) validateConnectorRouteBindings() {
	idx := runtimeIndex(v.cfg)
	for _, connector := range v.cfg.Connectors {
		if !connector.Enabled {
			continue
		}
		resolved := idx.binding(connector.Binding).DeviceID
		for _, route := range v.cfg.Routes {
			if !route.Enabled || route.ConnectorID != connector.ID || route.DeviceID == "" {
				continue
			}
			action := normalizeRouteAction(route.Action)
			if action != model.RouteActionBindDevice && action != model.RouteActionAllow {
				continue
			}
			if resolved == "" {
				resolved = route.DeviceID
				continue
			}
			if resolved != route.DeviceID {
				v.add("Connector", connector.ID, "Binding.DeviceID", fmt.Sprintf("conflicts with Route[%s].DeviceID", route.ID))
			}
		}
	}
}

type validator struct {
	cfg        RuntimeConfig
	apply      bool
	problems   []Problem
	devices    map[string]model.Device
	listeners  map[string]model.Listener
	connectors map[string]model.Connector
	clients    map[string]model.Client
	routes     map[string]model.Route
	vkeys      map[string]model.VKey
	addresses  map[string]model.AddressLimit
	xray       map[string]model.XrayProfile
	settings   map[string]model.Settings
}

func (v *validator) add(object, id, field, message string) {
	v.problems = append(v.problems, Problem{
		Object:  object,
		ID:      id,
		Field:   field,
		Message: message,
	})
}

func (v *validator) err() error {
	if len(v.problems) == 0 {
		return nil
	}
	return &ValidationError{Problems: v.problems}
}

func (v *validator) index() {
	v.devices = make(map[string]model.Device, len(v.cfg.Devices))
	v.listeners = make(map[string]model.Listener, len(v.cfg.Listeners))
	v.connectors = make(map[string]model.Connector, len(v.cfg.Connectors))
	v.clients = make(map[string]model.Client, len(v.cfg.Clients))
	v.routes = make(map[string]model.Route, len(v.cfg.Routes))
	v.vkeys = make(map[string]model.VKey, len(v.cfg.VKeys))
	v.addresses = make(map[string]model.AddressLimit, len(v.cfg.Addresses))
	v.xray = make(map[string]model.XrayProfile, len(v.cfg.XrayProfiles))
	v.settings = make(map[string]model.Settings, len(v.cfg.Settings))

	for _, item := range v.cfg.Devices {
		v.putDevice(item)
	}
	for _, item := range v.cfg.Listeners {
		v.putListener(item)
	}
	for _, item := range v.cfg.Connectors {
		v.putConnector(item)
	}
	for _, item := range v.cfg.Clients {
		v.putClient(item)
	}
	for _, item := range v.cfg.Routes {
		v.putRoute(item)
	}
	for _, item := range v.cfg.VKeys {
		v.putVKey(item)
	}
	for _, item := range v.cfg.Addresses {
		v.putAddress(item)
	}
	for _, item := range v.cfg.XrayProfiles {
		v.putXrayProfile(item)
	}
	for _, item := range v.cfg.Settings {
		v.putSettings(item)
	}
}

func (v *validator) putDevice(item model.Device) {
	if item.ID == "" {
		v.add("Device", "", "ID", "is required")
		return
	}
	if _, ok := v.devices[item.ID]; ok {
		v.add("Device", item.ID, "ID", "is duplicated")
		return
	}
	v.devices[item.ID] = item
}

func (v *validator) putListener(item model.Listener) {
	if item.ID == "" {
		v.add("Listener", "", "ID", "is required")
		return
	}
	if _, ok := v.listeners[item.ID]; ok {
		v.add("Listener", item.ID, "ID", "is duplicated")
		return
	}
	v.listeners[item.ID] = item
}

func (v *validator) putConnector(item model.Connector) {
	if item.ID == "" {
		v.add("Connector", "", "ID", "is required")
		return
	}
	if _, ok := v.connectors[item.ID]; ok {
		v.add("Connector", item.ID, "ID", "is duplicated")
		return
	}
	v.connectors[item.ID] = item
}

func (v *validator) putClient(item model.Client) {
	if item.ID == "" {
		v.add("Client", "", "ID", "is required")
		return
	}
	if _, ok := v.clients[item.ID]; ok {
		v.add("Client", item.ID, "ID", "is duplicated")
		return
	}
	v.clients[item.ID] = item
}

func (v *validator) putRoute(item model.Route) {
	if item.ID == "" {
		v.add("Route", "", "ID", "is required")
		return
	}
	if _, ok := v.routes[item.ID]; ok {
		v.add("Route", item.ID, "ID", "is duplicated")
		return
	}
	v.routes[item.ID] = item
}

func (v *validator) putVKey(item model.VKey) {
	if item.ID == "" {
		v.add("VKey", "", "ID", "is required")
		return
	}
	if _, ok := v.vkeys[item.ID]; ok {
		v.add("VKey", item.ID, "ID", "is duplicated")
		return
	}
	v.vkeys[item.ID] = item
}

func (v *validator) putAddress(item model.AddressLimit) {
	if item.ID == "" {
		v.add("AddressLimit", "", "ID", "is required")
		return
	}
	if _, ok := v.addresses[item.ID]; ok {
		v.add("AddressLimit", item.ID, "ID", "is duplicated")
		return
	}
	v.addresses[item.ID] = item
}

func (v *validator) putXrayProfile(item model.XrayProfile) {
	if item.ID == "" {
		v.add("XrayProfile", "", "ID", "is required")
		return
	}
	if _, ok := v.xray[item.ID]; ok {
		v.add("XrayProfile", item.ID, "ID", "is duplicated")
		return
	}
	v.xray[item.ID] = item
}

func (v *validator) putSettings(item model.Settings) {
	if item.ID == "" {
		v.add("Settings", "", "ID", "is required")
		return
	}
	if _, ok := v.settings[item.ID]; ok {
		v.add("Settings", item.ID, "ID", "is duplicated")
		return
	}
	v.settings[item.ID] = item
}

func (v *validator) validateDevices() {
	for _, item := range v.cfg.Devices {
		switch item.Type {
		case model.DeviceTAP, model.DeviceTUN:
		default:
			v.add("Device", item.ID, "Type", "must be tap or tun")
		}
		if item.IfName == "" {
			v.add("Device", item.ID, "IfName", "is required")
		}
		if item.MTU != 0 && (item.MTU < 576 || item.MTU > 65535) {
			v.add("Device", item.ID, "MTU", "must be between 576 and 65535")
		}
		if item.MSSClamp != 0 && (item.MSSClamp < 536 || item.MSSClamp > 65535) {
			v.add("Device", item.ID, "MSSClamp", "must be between 536 and 65535")
		}
		if item.LinkAutoOptimize && item.MSSClamp != 0 {
			v.add("Device", item.ID, "MSSClamp", "must be zero when automatic link optimization is enabled")
		}
		if item.LinkAutoOptimize && item.MTU == 0 {
			v.add("Device", item.ID, "MTU", "is required when automatic link optimization is enabled")
		}
		if item.Bridge != nil && item.Type != model.DeviceTAP {
			v.add("Device", item.ID, "Bridge", "is only valid for tap devices")
		}
		if item.Bridge != nil && item.Bridge.Enabled {
			if strings.TrimSpace(item.Bridge.Name) == "" {
				v.add("Device", item.ID, "Bridge.Name", "is required when bridge is enabled")
			}
			if item.Bridge.MTU != 0 && (item.Bridge.MTU < 576 || item.Bridge.MTU > 65535) {
				v.add("Device", item.ID, "Bridge.MTU", "must be between 576 and 65535")
			}
		}
		for i, route := range item.Routes {
			v.deviceRoute(item.ID, i, route)
		}
		v.deviceNetworkAccess(item)
	}
}

func (v *validator) deviceNetworkAccess(item model.Device) {
	role := item.AccessRole
	if role == "" {
		role = model.AccessRoleClient
	}
	if role != model.AccessRoleClient && role != model.AccessRoleServer {
		v.add("Device", item.ID, "AccessRole", "must be client or server")
	}
	if item.Type == model.DeviceTAP {
		mode := item.TapMode
		if mode == "" {
			mode = model.TapModeStandalone
		}
		switch mode {
		case model.TapModeStandalone, model.TapModeTransparent, model.TapModeOneArm, model.TapModeSharedIP:
		default:
			v.add("Device", item.ID, "TapMode", "must be standalone, transparent, one-arm, or shared-ip")
		}
		if mode == model.TapModeOneArm {
			if item.Bridge == nil || strings.TrimSpace(item.Bridge.IfName) == "" {
				v.add("Device", item.ID, "Bridge.IfName", "is required for one-arm mode")
			}
			if item.OneArmRollbackSeconds < 15 || item.OneArmRollbackSeconds > 3600 {
				v.add("Device", item.ID, "OneArmRollbackSeconds", "must be between 15 and 3600")
			}
		} else if item.OneArmRollbackSeconds != 0 {
			v.add("Device", item.ID, "OneArmRollbackSeconds", "is only valid for one-arm mode")
		}
		v.validateTapDHCP(item, role, mode)
		v.validateSharedIP(item, role, mode)
		if item.TUNDHCP != nil {
			v.add("Device", item.ID, "TUNDHCP", "is only valid for tun devices")
		}
		return
	}

	if item.TapMode != "" && item.TapMode != model.TapModeStandalone {
		v.add("Device", item.ID, "TapMode", "is only valid for tap devices")
	}
	if item.DHCP != nil {
		v.add("Device", item.ID, "DHCP", "is only valid for tap devices")
	}
	if item.SharedIP != nil {
		v.add("Device", item.ID, "SharedIP", "is only valid for tap devices")
	}
	if item.OneArmRollbackSeconds != 0 {
		v.add("Device", item.ID, "OneArmRollbackSeconds", "is only valid for tap devices")
	}
	v.validateTUNDHCP(item, role)
}

func (v *validator) validateTapDHCP(item model.Device, role model.AccessRole, tapMode model.TapMode) {
	if item.DHCP == nil {
		return
	}
	cfg := item.DHCP
	switch cfg.Mode {
	case "", model.DHCPModeOff, model.DHCPModePassthrough, model.DHCPModeServer, model.DHCPModeMirror:
	default:
		v.add("Device", item.ID, "DHCP.Mode", "must be off, passthrough, server, or mirror")
	}
	if cfg.Mode == model.DHCPModeServer {
		if role != model.AccessRoleServer {
			v.add("Device", item.ID, "DHCP.Mode", "server mode requires a server device")
		}
		v.validateDHCPv4Pool(item.ID, "DHCP", cfg.IPv4CIDR, cfg.PoolStart, cfg.PoolEnd)
		v.validatePoolReservedAddresses(item.ID, "DHCP", cfg.PoolStart, cfg.PoolEnd, cfg.IPv4CIDR, cfg.Gateway)
		if cfg.LeaseSeconds < 60 || cfg.LeaseSeconds > 31536000 {
			v.add("Device", item.ID, "DHCP.LeaseSeconds", "must be between 60 and 31536000")
		}
	}
	if cfg.Mode == model.DHCPModeMirror && tapMode != model.TapModeSharedIP {
		v.add("Device", item.ID, "DHCP.Mode", "mirror mode requires shared-ip mode")
	}
	for i, value := range cfg.DNS {
		v.anyIPAddress("Device", item.ID, fmt.Sprintf("DHCP.DNS[%d]", i), value)
	}
	for i, lease := range cfg.StaticLeases {
		if strings.TrimSpace(lease.MAC) == "" || strings.TrimSpace(lease.Address) == "" {
			v.add("Device", item.ID, fmt.Sprintf("DHCP.StaticLeases[%d]", i), "MAC and address are required")
			continue
		}
		if _, err := net.ParseMAC(lease.MAC); err != nil {
			v.add("Device", item.ID, fmt.Sprintf("DHCP.StaticLeases[%d].MAC", i), "must be a valid MAC address")
		}
		v.anyIPAddress("Device", item.ID, fmt.Sprintf("DHCP.StaticLeases[%d].Address", i), lease.Address)
	}
}

func (v *validator) validateSharedIP(item model.Device, role model.AccessRole, tapMode model.TapMode) {
	if item.SharedIP == nil {
		return
	}
	if tapMode != model.TapModeSharedIP {
		v.add("Device", item.ID, "SharedIP", "requires shared-ip mode")
	}
	cfg := item.SharedIP
	expected := model.SharedIPRoleAccess
	if role == model.AccessRoleServer {
		expected = model.SharedIPRoleService
	}
	if cfg.Role != "" && cfg.Role != expected {
		v.add("Device", item.ID, "SharedIP.Role", fmt.Sprintf("must be %s for this access role", expected))
	}
	if expected == model.SharedIPRoleService && strings.TrimSpace(cfg.UplinkInterface) == "" {
		v.add("Device", item.ID, "SharedIP.UplinkInterface", "is required for the service role")
	}
	if cfg.AddressSource != "" && cfg.AddressSource != "auto" && cfg.AddressSource != "manual" {
		v.add("Device", item.ID, "SharedIP.AddressSource", "must be auto or manual")
	}
	if cfg.AddressSource == "manual" {
		v.ipPrefix("Device", item.ID, "SharedIP.IPv4CIDR", cfg.IPv4CIDR, true)
		v.anyIPAddress("Device", item.ID, "SharedIP.Gateway", cfg.Gateway)
	}
	switch cfg.FirewallBackend {
	case "", model.FirewallAuto, model.FirewallNFTables, model.FirewallIPTables:
	default:
		v.add("Device", item.ID, "SharedIP.FirewallBackend", "must be auto, nftables, or iptables")
	}
	v.validatePortRanges(item.ID, "SharedIP.ReservedTCPPorts", cfg.ReservedTCPPorts)
	v.validatePortRanges(item.ID, "SharedIP.ReservedUDPPorts", cfg.ReservedUDPPorts)
	if cfg.ClientMAC != "" {
		if _, err := net.ParseMAC(cfg.ClientMAC); err != nil {
			v.add("Device", item.ID, "SharedIP.ClientMAC", "must be a valid MAC address")
		}
	}
}

func (v *validator) validateTUNDHCP(item model.Device, role model.AccessRole) {
	if item.TUNDHCP == nil {
		return
	}
	cfg := item.TUNDHCP
	switch cfg.Mode {
	case "", model.TUNDHCPModeOff, model.TUNDHCPModeClient, model.TUNDHCPModeServer, model.TUNDHCPModeManual:
	default:
		v.add("Device", item.ID, "TUNDHCP.Mode", "must be off, client, server, or manual")
	}
	if role == model.AccessRoleClient && cfg.Mode == model.TUNDHCPModeServer {
		v.add("Device", item.ID, "TUNDHCP.Mode", "server mode requires a server device")
	}
	if role == model.AccessRoleServer && cfg.Mode == model.TUNDHCPModeClient {
		v.add("Device", item.ID, "TUNDHCP.Mode", "client mode requires a client device")
	}
	if cfg.RelayEnabled && role != model.AccessRoleServer {
		v.add("Device", item.ID, "TUNDHCP.RelayEnabled", "DHCP relay requires a server device")
	}
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = "ipv4"
	}
	if protocol != "ipv4" && protocol != "ipv6" && protocol != "dual" {
		v.add("Device", item.ID, "TUNDHCP.Protocol", "must be ipv4, ipv6, or dual")
	}
	if cfg.Mode == model.TUNDHCPModeManual || cfg.Mode == model.TUNDHCPModeServer {
		if protocol != "ipv6" {
			v.ipPrefix("Device", item.ID, "TUNDHCP.IPv4CIDR", cfg.IPv4CIDR, true)
		}
		if protocol != "ipv4" {
			v.ipPrefix("Device", item.ID, "TUNDHCP.IPv6CIDR", cfg.IPv6CIDR, false)
		}
	}
	if cfg.Mode == model.TUNDHCPModeServer {
		if protocol != "ipv6" {
			v.validateDHCPv4Pool(item.ID, "TUNDHCP", cfg.IPv4CIDR, cfg.PoolStart, cfg.PoolEnd)
			v.validatePoolReservedAddresses(item.ID, "TUNDHCP", cfg.PoolStart, cfg.PoolEnd, cfg.IPv4CIDR, cfg.Gateway, cfg.OfferedGateway)
		}
		if protocol != "ipv4" {
			v.validateAddressRange(item.ID, "TUNDHCP", cfg.IPv6PoolStart, cfg.IPv6PoolEnd, false)
			v.validatePoolReservedAddresses(item.ID, "TUNDHCP", cfg.IPv6PoolStart, cfg.IPv6PoolEnd, cfg.IPv6CIDR, cfg.Gateway, cfg.OfferedGateway)
		}
	}
	if cfg.RelayEnabled {
		if len(cfg.RelayDownstreamInterfaces) == 0 || len(cfg.RelayServers) == 0 {
			v.add("Device", item.ID, "TUNDHCP.Relay", "downstream interfaces and relay servers are required")
		}
		for i, server := range cfg.RelayServers {
			v.anyIPAddress("Device", item.ID, fmt.Sprintf("TUNDHCP.RelayServers[%d]", i), server)
		}
		if cfg.MaxHops < 1 || cfg.MaxHops > 16 {
			v.add("Device", item.ID, "TUNDHCP.MaxHops", "must be between 1 and 16")
		}
	}
}

func (v *validator) validateDHCPv4Pool(id, field, cidr, start, end string) {
	v.ipPrefix("Device", id, field+".IPv4CIDR", cidr, true)
	v.validateAddressRange(id, field, start, end, true)
	if prefix, err := netip.ParsePrefix(cidr); err == nil {
		for name, value := range map[string]string{"PoolStart": start, "PoolEnd": end} {
			if addr, err := netip.ParseAddr(value); err == nil && !prefix.Contains(addr) {
				v.add("Device", id, field+"."+name, "must be inside the interface subnet")
			}
		}
	}
}

func (v *validator) validateAddressRange(id, field, start, end string, ipv4 bool) {
	startAddr, startErr := netip.ParseAddr(strings.TrimSpace(start))
	endAddr, endErr := netip.ParseAddr(strings.TrimSpace(end))
	if startErr != nil || startAddr.Is4() != ipv4 {
		v.add("Device", id, field+".PoolStart", "must be a valid address of the expected family")
	}
	if endErr != nil || endAddr.Is4() != ipv4 {
		v.add("Device", id, field+".PoolEnd", "must be a valid address of the expected family")
	}
	if startErr == nil && endErr == nil && startAddr.Compare(endAddr) > 0 {
		v.add("Device", id, field+".PoolEnd", "must not be lower than the pool start")
	}
}

func (v *validator) validatePoolReservedAddresses(id, field, start, end, interfaceCIDR string, values ...string) {
	startAddr, startErr := netip.ParseAddr(strings.TrimSpace(start))
	endAddr, endErr := netip.ParseAddr(strings.TrimSpace(end))
	if startErr != nil || endErr != nil || startAddr.BitLen() != endAddr.BitLen() {
		return
	}
	reserved := make(map[netip.Addr]string)
	if prefix, err := netip.ParsePrefix(strings.TrimSpace(interfaceCIDR)); err == nil {
		reserved[prefix.Addr().Unmap()] = "interface address"
	}
	for _, value := range values {
		if address, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
			reserved[address.Unmap()] = "gateway"
		}
	}
	for address, label := range reserved {
		if address.BitLen() == startAddr.BitLen() &&
			startAddr.Compare(address) <= 0 && address.Compare(endAddr) <= 0 {
			v.add("Device", id, field+".Pool", "must not include the "+label+" "+address.String())
		}
	}
}

func (v *validator) anyIPAddress(object, id, field, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(object, id, field, "is required")
		return
	}
	if _, err := netip.ParseAddr(strings.TrimSpace(value)); err != nil {
		v.add(object, id, field, "must be a valid IP address")
	}
}

func (v *validator) validatePortRanges(id, field string, values []string) {
	for i, value := range values {
		parts := strings.Split(strings.TrimSpace(value), "-")
		if len(parts) < 1 || len(parts) > 2 {
			v.add("Device", id, fmt.Sprintf("%s[%d]", field, i), "must be a port or port range")
			continue
		}
		first, err1 := strconv.Atoi(parts[0])
		last := first
		var err2 error
		if len(parts) == 2 {
			last, err2 = strconv.Atoi(parts[1])
		}
		if err1 != nil || err2 != nil || first < 1 || last > 65535 || first > last {
			v.add("Device", id, fmt.Sprintf("%s[%d]", field, i), "must be between 1 and 65535")
		}
	}
}

func (v *validator) validateVKeys() {
	for _, item := range v.cfg.VKeys {
		if item.Enabled && item.Value == "" {
			v.add("VKey", item.ID, "Value", "is required when enabled")
		}
		if len([]byte(item.Value)) > 1024 {
			v.add("VKey", item.ID, "Value", "must be 1024 bytes or less")
		}
	}
}

func (v *validator) validateXrayProfiles() {
	for _, item := range v.cfg.XrayProfiles {
		switch item.Runtime {
		case "", model.XrayEmbedded, model.XrayExternal:
		default:
			v.add("XrayProfile", item.ID, "Runtime", "must be empty, embedded, or external")
		}
		v.xrayProtocol("XrayProfile", item.ID, "InboundProtocol", item.InboundProtocol)
		v.xrayProtocol("XrayProfile", item.ID, "OutboundProtocol", item.OutboundProtocol)
		for field, value := range map[string]string{
			"InboundSettingsJSON":  item.InboundSettingsJSON,
			"OutboundSettingsJSON": item.OutboundSettingsJSON,
			"StreamSettingsJSON":   item.StreamSettingsJSON,
			"SniffingJSON":         item.SniffingJSON,
			"MuxJSON":              item.MuxJSON,
			"SockoptJSON":          item.SockoptJSON,
			"FallbacksJSON":        item.FallbacksJSON,
			"RoutingJSON":          item.RoutingJSON,
			"DNSJSON":              item.DNSJSON,
			"PolicyJSON":           item.PolicyJSON,
			"AdvancedJSON":         item.AdvancedJSON,
		} {
			v.jsonObject("XrayProfile", item.ID, field, value)
		}
		if v.xrayProfileUsesAutomaticLink(item.ID) && xrayDisablesQUICPathMTU(item) {
			v.add("XrayProfile", item.ID, "StreamSettingsJSON", "cannot disable QUIC path MTU discovery while a bound device enables automatic link optimization")
		}
		if xrayUsesRealityWithFinalMask(item) {
			v.add("XrayProfile", item.ID, "StreamSettingsJSON", "Final Mask cannot be combined with REALITY")
		}
	}
}

func xrayUsesRealityWithFinalMask(profile model.XrayProfile) bool {
	var stream map[string]any
	if strings.TrimSpace(profile.StreamSettingsJSON) == "" || json.Unmarshal([]byte(profile.StreamSettingsJSON), &stream) != nil {
		return false
	}
	security, _ := stream["security"].(string)
	if !strings.EqualFold(strings.TrimSpace(security), "reality") {
		return false
	}
	finalMask, ok := stream["finalmask"].(map[string]any)
	return ok && len(finalMask) > 0
}

func (v *validator) xrayProfileUsesAutomaticLink(profileID string) bool {
	uses := func(enabled bool, transport model.Transport, xrayProfileID string, binding model.Binding) bool {
		if !enabled || transport != model.TransportXray || xrayProfileID != profileID {
			return false
		}
		deviceID := binding.DeviceID
		if deviceID == "" && binding.RouteID != "" {
			if route, ok := v.routes[binding.RouteID]; ok {
				deviceID = route.DeviceID
			}
		}
		device, ok := v.devices[deviceID]
		return ok && device.Enabled && device.LinkAutoOptimize
	}
	for _, item := range v.cfg.Listeners {
		if uses(item.Enabled, item.Transport, item.XrayProfileID, item.Binding) {
			return true
		}
	}
	for _, item := range v.cfg.Connectors {
		if uses(item.Enabled, item.Transport, item.XrayProfileID, item.Binding) {
			return true
		}
	}
	return false
}

func xrayDisablesQUICPathMTU(profile model.XrayProfile) bool {
	var stream map[string]any
	if strings.TrimSpace(profile.StreamSettingsJSON) == "" || json.Unmarshal([]byte(profile.StreamSettingsJSON), &stream) != nil {
		return false
	}
	network := strings.ToLower(strings.TrimSpace(profile.Network))
	if network == "" {
		network, _ = stream["network"].(string)
		network = strings.ToLower(strings.TrimSpace(network))
	}
	if network != "hysteria" && network != "xhttp" && network != "splithttp" {
		return false
	}
	finalMask, _ := stream["finalmask"].(map[string]any)
	quicParams, _ := finalMask["quicParams"].(map[string]any)
	disabled, _ := quicParams["disablePathMTUDiscovery"].(bool)
	return disabled
}

func (v *validator) validateSettings() {
	for _, item := range v.cfg.Settings {
		panelName := strings.TrimSpace(item.PanelName)
		if len([]rune(panelName)) > 64 {
			v.add("Settings", item.ID, "PanelName", "must not exceed 64 characters")
		}
		if strings.ContainsAny(panelName, "\r\n\t") {
			v.add("Settings", item.ID, "PanelName", "must not contain control characters")
		}
		switch item.LogLevel {
		case "", "debug", "info", "warn", "error":
		default:
			v.add("Settings", item.ID, "LogLevel", "must be empty, debug, info, warn, or error")
		}
		if target := strings.TrimSpace(item.OpenWrtBuildTarget); target != "" {
			if len(target) > 128 || strings.ContainsAny(target, "\x00\r\n\t ") {
				v.add("Settings", item.ID, "OpenWrtBuildTarget", "must be a platform target without whitespace or control characters")
			}
		}
		if listen := strings.TrimSpace(item.PanelListen); listen != "" {
			host, portText, err := net.SplitHostPort(listen)
			if err != nil {
				v.add("Settings", item.ID, "PanelListen", "must be an IP and port, for example :2053 or 127.0.0.1:2053")
			} else {
				if host != "" && net.ParseIP(host) == nil {
					v.add("Settings", item.ID, "PanelListen", "host must be an IP address")
				}
				port, portErr := strconv.ParseUint(portText, 10, 16)
				if portErr != nil || port == 0 {
					v.add("Settings", item.ID, "PanelListen", "port must be between 1 and 65535")
				}
			}
		}
		certFile := strings.TrimSpace(item.PanelCertFile)
		keyFile := strings.TrimSpace(item.PanelKeyFile)
		if (certFile == "") != (keyFile == "") {
			v.add("Settings", item.ID, "PanelCertFile", "certificate and key must be configured together")
		}
		for field, value := range map[string]string{"PanelCertFile": certFile, "PanelKeyFile": keyFile} {
			if value != "" && !strings.HasPrefix(value, "/") && !(len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')) {
				v.add("Settings", item.ID, field, "must be an absolute path")
			}
		}
		if item.PanelHTTPS && (strings.TrimSpace(item.PanelCertFile) == "" || strings.TrimSpace(item.PanelKeyFile) == "") {
			v.add("Settings", item.ID, "PanelHTTPS", "requires PanelCertFile and PanelKeyFile")
		}
		if item.PanelDomain != "" && strings.ContainsAny(item.PanelDomain, "/:?#@\r\n\t ") {
			v.add("Settings", item.ID, "PanelDomain", "must be a host name without scheme, port, path, or whitespace")
		}
		if item.PanelBasePath != "" && (item.PanelBasePath[0] != '/' || item.PanelBasePath[len(item.PanelBasePath)-1] != '/' || strings.ContainsAny(item.PanelBasePath, "?#\r\n\t ")) {
			v.add("Settings", item.ID, "PanelBasePath", "must start and end with '/' and contain no query, fragment, or whitespace")
		}
		if item.Timezone != "" {
			if _, err := time.LoadLocation(item.Timezone); err != nil {
				v.add("Settings", item.ID, "Timezone", "must be a valid IANA timezone")
			}
		}
		if outbound := strings.TrimSpace(item.PanelOutbound); outbound != "" && outbound != "direct" {
			connector, ok := v.connectors[outbound]
			if !ok || !connector.Enabled {
				v.add("Settings", item.ID, "PanelOutbound", "must reference an enabled connector or direct")
			} else if connector.Transport != model.TransportXray {
				v.add("Settings", item.ID, "PanelOutbound", "must reference an embedded xray connector")
			} else if profile, ok := v.xray[connector.XrayProfileID]; !ok || !profile.Enabled || normalizeXrayRuntime(profile.Runtime) != model.XrayEmbedded {
				v.add("Settings", item.ID, "PanelOutbound", "must reference an embedded xray connector")
			}
		}
		if item.PanelAuthEnabled {
			if strings.TrimSpace(item.AdminUsername) == "" {
				v.add("Settings", item.ID, "AdminUsername", "is required when panel auth is enabled")
			}
			if strings.TrimSpace(item.AdminPasswordHash) == "" {
				v.add("Settings", item.ID, "AdminPasswordHash", "is required when panel auth is enabled")
			} else {
				v.panelPasswordHash("Settings", item.ID, "AdminPasswordHash", item.AdminPasswordHash)
			}
		}
		if strings.ContainsRune(item.ExternalXrayPath, 0) {
			v.add("Settings", item.ID, "ExternalXrayPath", "must not contain NUL")
		}
		for field, value := range map[string]string{
			"ExternalXrayConfigFile": item.ExternalXrayConfigFile,
			"ExternalXrayWorkDir":    item.ExternalXrayWorkDir,
			"ExternalXrayArgs":       item.ExternalXrayArgs,
		} {
			if strings.ContainsRune(value, 0) {
				v.add("Settings", item.ID, field, "must not contain NUL")
			}
		}
		v.positive("StatsIntervalSecond", "Settings", item.ID, item.StatsIntervalSecond)
		v.positive("SessionTTLSecond", "Settings", item.ID, item.SessionTTLSecond)
		v.jsonObject("Settings", item.ID, "AdvancedJSON", item.AdvancedJSON)
	}
}

func (v *validator) validateRoutes() {
	for _, item := range v.cfg.Routes {
		if item.Priority < 0 {
			v.add("Route", item.ID, "Priority", "must be greater than or equal to 0")
		}
		switch item.Action {
		case "", model.RouteActionBindDevice, model.RouteActionAllow, model.RouteActionDrop:
		default:
			v.add("Route", item.ID, "Action", "must be empty, bind-device, allow, or drop")
		}
		hasTrafficSelector := item.VKeyID != "" || item.ListenerID != "" || item.ConnectorID != "" || item.ClientID != "" || v.routeHasBindingReference(item.ID)
		if !hasTrafficSelector {
			v.add("Route", item.ID, "Match", "must select a listener, connector, client, or vKey; device and address limit are outputs, not traffic selectors")
		}
		if (item.Action == "" || item.Action == model.RouteActionBindDevice) && item.DeviceID == "" {
			v.add("Route", item.ID, "DeviceID", "is required for bind-device action")
		}
		if item.Action == model.RouteActionDrop && item.ListenerID == "" && item.ClientID == "" && item.VKeyID == "" && !v.routeHasIngressBindingReference(item.ID) {
			v.add("Route", item.ID, "Match", "drop requires a listener, client, or vKey selector; a connector alone has no inbound traffic to reject")
		}
		v.bindingRefs("Route", item.ID, model.Binding{
			VKeyID:    item.VKeyID,
			ClientID:  item.ClientID,
			DeviceID:  item.DeviceID,
			AddressID: item.AddressID,
		}, "")
		if item.ListenerID != "" {
			ref(v, "Route", item.ID, "ListenerID", item.ListenerID, v.listeners)
		}
		if item.ConnectorID != "" {
			ref(v, "Route", item.ID, "ConnectorID", item.ConnectorID, v.connectors)
		}
	}
}

func (v *validator) routeHasBindingReference(routeID string) bool {
	for _, item := range v.cfg.Listeners {
		if item.Binding.RouteID == routeID {
			return true
		}
	}
	for _, item := range v.cfg.Connectors {
		if item.Binding.RouteID == routeID {
			return true
		}
	}
	for _, item := range v.cfg.Clients {
		if item.Binding.RouteID == routeID {
			return true
		}
	}
	return false
}

func (v *validator) routeHasIngressBindingReference(routeID string) bool {
	for _, item := range v.cfg.Listeners {
		if item.Binding.RouteID == routeID {
			return true
		}
	}
	for _, item := range v.cfg.Clients {
		if item.Binding.RouteID == routeID {
			return true
		}
	}
	return false
}

func (v *validator) validateListeners() {
	for _, item := range v.cfg.Listeners {
		if item.ExpiresAt < 0 {
			v.add("Listener", item.ID, "ExpiresAt", "must be greater than or equal to 0")
		}
		v.trafficReset("Listener", item.ID, item.TrafficReset)
		switch item.ShareAddressStrategy {
		case "", "listen":
		case "custom":
			if strings.TrimSpace(item.ShareAddress) == "" {
				v.add("Listener", item.ID, "ShareAddress", "is required when ShareAddressStrategy is custom")
			} else if strings.ContainsAny(item.ShareAddress, "/?#@\r\n\t ") {
				v.add("Listener", item.ID, "ShareAddress", "must be a host or IP without scheme, path, or port")
			}
		default:
			v.add("Listener", item.ID, "ShareAddressStrategy", "must be empty, listen, or custom")
		}
		v.transport("Listener", item.ID, item.Transport)
		if item.Transport != model.TransportXray && item.BindPort == 0 {
			v.add("Listener", item.ID, "BindPort", "is required for raw tcp/udp")
		}
		if item.Transport == model.TransportXray && item.Binding.VKeyID != "" {
			v.add("Listener", item.ID, "Binding.VKeyID", "vKey is only valid for raw tcp/udp")
		}
		v.xrayProfileRef("Listener", item.ID, item.Transport, item.XrayProfileID)
		v.rawUDP("Listener", item.ID, item.Transport, item.RawUDP)
		v.rawTCP("Listener", item.ID, item.Transport, item.RawTCP)
		v.bindingRefs("Listener", item.ID, item.Binding, item.Transport)
		v.endpointClientPolicy("Listener", item.ID, item.Binding)
	}
}

func (v *validator) validateConnectors() {
	for _, item := range v.cfg.Connectors {
		v.transport("Connector", item.ID, item.Transport)
		if item.Transport != model.TransportXray {
			if item.Remote == "" {
				v.add("Connector", item.ID, "Remote", "is required for raw tcp/udp")
			}
			if item.Port == 0 {
				v.add("Connector", item.ID, "Port", "is required for raw tcp/udp")
			}
		}
		if item.Transport == model.TransportXray && item.Binding.VKeyID != "" {
			v.add("Connector", item.ID, "Binding.VKeyID", "vKey is only valid for raw tcp/udp")
		}
		v.xrayProfileRef("Connector", item.ID, item.Transport, item.XrayProfileID)
		v.rawUDP("Connector", item.ID, item.Transport, item.RawUDP)
		v.rawTCP("Connector", item.ID, item.Transport, item.RawTCP)
		v.bindingRefs("Connector", item.ID, item.Binding, item.Transport)
		v.endpointClientPolicy("Connector", item.ID, item.Binding)
	}
}

func (v *validator) validateClients() {
	for _, item := range v.cfg.Clients {
		const maxUserRateLimit = uint64(1_000_000_000_000_000)
		if item.UploadRateLimit > maxUserRateLimit {
			v.add("Client", item.ID, "UploadRateLimit", "must not exceed 1 Pbps")
		}
		if item.DownloadRateLimit > maxUserRateLimit {
			v.add("Client", item.ID, "DownloadRateLimit", "must not exceed 1 Pbps")
		}
		v.trafficReset("Client", item.ID, item.TrafficReset)
		if item.ListenerID != "" {
			ref(v, "Client", item.ID, "ListenerID", item.ListenerID, v.listeners)
		}
		seenListeners := map[string]bool{}
		for index, listenerID := range item.ListenerIDs {
			listenerID = strings.TrimSpace(listenerID)
			if listenerID == "" || seenListeners[listenerID] {
				continue
			}
			seenListeners[listenerID] = true
			ref(v, "Client", item.ID, fmt.Sprintf("ListenerIDs[%d]", index), listenerID, v.listeners)
		}
		seenDevices := map[string]bool{}
		for index, deviceID := range item.AllowedDeviceIDs {
			deviceID = strings.TrimSpace(deviceID)
			if deviceID == "" || seenDevices[deviceID] {
				continue
			}
			seenDevices[deviceID] = true
			ref(v, "Client", item.ID, fmt.Sprintf("AllowedDeviceIDs[%d]", index), deviceID, v.devices)
		}
		v.clientCredentials(item)
		if item.AddressID != "" {
			ref(v, "Client", item.ID, "AddressID", item.AddressID, v.addresses)
		}
		v.bindingRefs("Client", item.ID, item.Binding, "")
		clientBinding := runtimeIndex(v.cfg).bindingBase(item.Binding)
		if item.AddressID != "" && clientBinding.AddressID != "" && item.AddressID != clientBinding.AddressID {
			v.add("Client", item.ID, "AddressID", "conflicts with client binding address limit")
		}
		if clientBinding.DeviceID != "" && !clientAllowsDevice(item, clientBinding.DeviceID) {
			v.add("Client", item.ID, "Binding.DeviceID", "is outside AllowedDeviceIDs")
		}
	}
}

func (v *validator) endpointClientPolicy(object, id string, binding model.Binding) {
	idx := runtimeIndex(v.cfg)
	base := idx.bindingBase(binding)
	if base.ClientID == "" {
		return
	}
	client, ok := v.clients[base.ClientID]
	if !ok {
		return
	}
	clientBinding := idx.bindingBase(client.Binding)
	clientAddressID := first(client.AddressID, clientBinding.AddressID)
	v.noResolvedBindingConflict(object, id, "Binding.VKeyID", base.VKeyValue, clientBinding.VKeyValue)
	v.noResolvedBindingConflict(object, id, "Binding.DeviceID", base.DeviceID, clientBinding.DeviceID)
	v.noResolvedBindingConflict(object, id, "Binding.ConnectorID", base.ConnectorID, clientBinding.ConnectorID)
	v.noResolvedBindingConflict(object, id, "Binding.AddressID", base.AddressID, clientAddressID)

	resolvedDeviceID := first(base.DeviceID, clientBinding.DeviceID)
	if resolvedDeviceID != "" && !clientAllowsDevice(client, resolvedDeviceID) {
		v.add(object, id, "Binding.DeviceID", fmt.Sprintf("is not allowed by Client[%s].AllowedDeviceIDs", client.ID))
	}
	if object == "Listener" && !clientAllowsListener(client, id) {
		v.add(object, id, "Binding.ClientID", fmt.Sprintf("Client[%s] is not assigned to this listener", client.ID))
	}
}

func (v *validator) noResolvedBindingConflict(object, id, field, endpointValue, clientValue string) {
	if endpointValue != "" && clientValue != "" && endpointValue != clientValue {
		v.add(object, id, field, "conflicts with referenced client binding")
	}
}

func clientAllowsDevice(client model.Client, deviceID string) bool {
	hasRestriction := false
	for _, allowed := range client.AllowedDeviceIDs {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		hasRestriction = true
		if allowed == deviceID {
			return true
		}
	}
	return !hasRestriction
}

func clientAllowsListener(client model.Client, listenerID string) bool {
	hasRestriction := false
	if assigned := strings.TrimSpace(client.ListenerID); assigned != "" {
		hasRestriction = true
		if assigned == listenerID {
			return true
		}
	}
	for _, assigned := range client.ListenerIDs {
		assigned = strings.TrimSpace(assigned)
		if assigned == "" {
			continue
		}
		hasRestriction = true
		if assigned == listenerID {
			return true
		}
	}
	return !hasRestriction
}

func (v *validator) trafficReset(kind, id, value string) {
	switch value {
	case "", "never", "hourly", "daily", "weekly", "monthly":
	default:
		v.add(kind, id, "TrafficReset", "must be empty, never, hourly, daily, weekly, or monthly")
	}
}

type addressOwner struct {
	id    string
	field string
}

func (v *validator) clientCredentials(item model.Client) {
	uuid := strings.TrimSpace(item.UUID)
	if uuid != "" && !looksLikeUUID(uuid) {
		v.add("Client", item.ID, "UUID", "must be a UUID")
	}
}

func (v *validator) validateAddressLimits() {
	seen := map[string]addressOwner{}
	for _, item := range v.cfg.Addresses {
		if item.DeviceID != "" {
			ref(v, "AddressLimit", item.ID, "DeviceID", item.DeviceID, v.devices)
		}
		if item.ClientID != "" {
			ref(v, "AddressLimit", item.ID, "ClientID", item.ClientID, v.clients)
		}
		device, hasDevice := v.devices[item.DeviceID]
		if hasDevice && device.Type == model.DeviceTUN && len(item.MACs) > 0 {
			v.add("AddressLimit", item.ID, "MACs", "MAC limits are only valid for tap devices")
		}
		for _, mac := range item.MACs {
			if _, err := net.ParseMAC(mac); err != nil {
				v.add("AddressLimit", item.ID, "MACs", fmt.Sprintf("%q is invalid", mac))
			}
			v.uniqueAddress(seen, "mac:"+strings.ToLower(mac), item.ID, "MACs")
		}
		for _, cidr := range item.IPv4CIDRs {
			v.addressPrefix("AddressLimit", item.ID, "IPv4CIDRs", cidr, true)
			v.uniqueAddress(seen, "ip4:"+cidr, item.ID, "IPv4CIDRs")
		}
		for _, cidr := range item.IPv6CIDRs {
			v.addressPrefix("AddressLimit", item.ID, "IPv6CIDRs", cidr, false)
			v.uniqueAddress(seen, "ip6:"+cidr, item.ID, "IPv6CIDRs")
		}
		v.ipAddress("AddressLimit", item.ID, "IPv4Gateway", item.IPv4Gateway, true)
		v.ipAddress("AddressLimit", item.ID, "IPv6Gateway", item.IPv6Gateway, false)
		for _, dns := range item.DNS {
			if dns == "" {
				continue
			}
			if _, err := netip.ParseAddr(dns); err != nil {
				v.add("AddressLimit", item.ID, "DNS", fmt.Sprintf("%q is invalid", dns))
			}
		}
		for _, route := range item.Routes {
			if route == "" {
				continue
			}
			if _, err := netip.ParsePrefix(route); err != nil {
				v.add("AddressLimit", item.ID, "Routes", fmt.Sprintf("%q is invalid", route))
			}
		}
	}
}

func (v *validator) uniqueAddress(seen map[string]addressOwner, key, id, field string) {
	if key == "" {
		return
	}
	if first, ok := seen[key]; ok {
		v.add("AddressLimit", id, field, fmt.Sprintf("conflicts with AddressLimit[%s].%s", first.id, first.field))
		return
	}
	seen[key] = addressOwner{id: id, field: field}
}

func (v *validator) enabledReferences() {
	for _, item := range v.cfg.Routes {
		if !item.Enabled {
			continue
		}
		enabledRef(v, "Route", item.ID, "VKeyID", item.VKeyID, v.vkeys)
		enabledRef(v, "Route", item.ID, "ListenerID", item.ListenerID, v.listeners)
		enabledRef(v, "Route", item.ID, "ConnectorID", item.ConnectorID, v.connectors)
		enabledRef(v, "Route", item.ID, "ClientID", item.ClientID, v.clients)
		enabledRef(v, "Route", item.ID, "DeviceID", item.DeviceID, v.devices)
		enabledRef(v, "Route", item.ID, "AddressID", item.AddressID, v.addresses)
	}
	for _, item := range v.cfg.Listeners {
		if item.Enabled {
			v.enabledBinding("Listener", item.ID, item.Binding)
			if item.Transport == model.TransportXray {
				if item.XrayProfileID == "" {
					v.add("Listener", item.ID, "XrayProfileID", "is required for xray transport")
				}
				enabledRef(v, "Listener", item.ID, "XrayProfileID", item.XrayProfileID, v.xray)
			}
		}
	}
	for _, item := range v.cfg.Connectors {
		if item.Enabled {
			v.enabledBinding("Connector", item.ID, item.Binding)
			if item.Transport == model.TransportXray {
				if item.XrayProfileID == "" {
					v.add("Connector", item.ID, "XrayProfileID", "is required for xray transport")
				}
				enabledRef(v, "Connector", item.ID, "XrayProfileID", item.XrayProfileID, v.xray)
			}
		}
	}
	for _, item := range v.cfg.Clients {
		if item.Enabled {
			v.enabledBinding("Client", item.ID, item.Binding)
		}
	}
	v.enabledXrayRuntimeRequirements()
}

func (v *validator) enabledXrayRuntimeRequirements() {
	externalNeeded := false
	for _, item := range v.cfg.Listeners {
		if !item.Enabled || item.Transport != model.TransportXray {
			continue
		}
		profile, ok := v.xray[item.XrayProfileID]
		if !ok || !profile.Enabled {
			continue
		}
		if normalizeXrayRuntime(profile.Runtime) == model.XrayExternal {
			externalNeeded = true
		}
		if item.BindPort == 0 {
			v.add("Listener", item.ID, "BindPort", "is required for xray runtime")
		}
		if strings.TrimSpace(profile.InboundProtocol) == "" {
			v.add("XrayProfile", profile.ID, "InboundProtocol", "is required for xray listeners")
		}
	}
	for _, item := range v.cfg.Connectors {
		if !item.Enabled || item.Transport != model.TransportXray {
			continue
		}
		profile, ok := v.xray[item.XrayProfileID]
		if !ok || !profile.Enabled {
			continue
		}
		if normalizeXrayRuntime(profile.Runtime) == model.XrayExternal {
			externalNeeded = true
		}
		if strings.TrimSpace(profile.OutboundProtocol) == "" {
			v.add("XrayProfile", profile.ID, "OutboundProtocol", "is required for xray connectors")
		}
	}
	if !externalNeeded {
		return
	}
	settings, ok := v.firstEnabledSettings()
	if !ok {
		v.add("Settings", "", "ExternalXrayPath", "is required for external xray runtime")
		return
	}
	if strings.TrimSpace(settings.ExternalXrayPath) == "" {
		v.add("Settings", settings.ID, "ExternalXrayPath", "is required for external xray runtime")
	}
}

func (v *validator) firstEnabledSettings() (model.Settings, bool) {
	for _, item := range v.cfg.Settings {
		if item.Enabled {
			return item, true
		}
	}
	return model.Settings{}, false
}

func (v *validator) enabledBinding(object, id string, b model.Binding) {
	enabledRef(v, object, id, "Binding.VKeyID", b.VKeyID, v.vkeys)
	enabledRef(v, object, id, "Binding.ClientID", b.ClientID, v.clients)
	enabledRef(v, object, id, "Binding.RouteID", b.RouteID, v.routes)
	enabledRef(v, object, id, "Binding.DeviceID", b.DeviceID, v.devices)
	enabledRef(v, object, id, "Binding.ConnectorID", b.ConnectorID, v.connectors)
	enabledRef(v, object, id, "Binding.AddressID", b.AddressID, v.addresses)
}

func (v *validator) bindingRefs(object, id string, b model.Binding, transport model.Transport) {
	if b.VKeyID != "" {
		ref(v, object, id, "Binding.VKeyID", b.VKeyID, v.vkeys)
		if transport == model.TransportXray {
			v.add(object, id, "Binding.VKeyID", "vKey is only valid for raw tcp/udp")
		}
	}
	if b.ClientID != "" {
		ref(v, object, id, "Binding.ClientID", b.ClientID, v.clients)
	}
	if b.RouteID != "" {
		ref(v, object, id, "Binding.RouteID", b.RouteID, v.routes)
		if route, ok := v.routes[b.RouteID]; ok {
			v.noBindingConflict(object, id, "Binding.VKeyID", b.VKeyID, route.VKeyID)
			v.noBindingConflict(object, id, "Binding.ClientID", b.ClientID, route.ClientID)
			v.noBindingConflict(object, id, "Binding.DeviceID", b.DeviceID, route.DeviceID)
			v.noBindingConflict(object, id, "Binding.ConnectorID", b.ConnectorID, route.ConnectorID)
			v.noBindingConflict(object, id, "Binding.AddressID", b.AddressID, route.AddressID)
		}
	}
	if b.DeviceID != "" {
		ref(v, object, id, "Binding.DeviceID", b.DeviceID, v.devices)
	}
	if b.ConnectorID != "" {
		ref(v, object, id, "Binding.ConnectorID", b.ConnectorID, v.connectors)
	}
	if b.AddressID != "" {
		ref(v, object, id, "Binding.AddressID", b.AddressID, v.addresses)
	}
}

func (v *validator) noBindingConflict(object, id, field, direct, routed string) {
	if direct != "" && routed != "" && direct != routed {
		v.add(object, id, field, "conflicts with referenced route")
	}
}

func looksLikeUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, ch := range value {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
				return false
			}
		}
	}
	return true
}

func (v *validator) transport(object, id string, transport model.Transport) {
	switch transport {
	case model.TransportUDP, model.TransportTCP, model.TransportXray:
	default:
		v.add(object, id, "Transport", "must be udp, tcp, or xray")
	}
}

func (v *validator) xrayProfileRef(object, id string, transport model.Transport, profileID string) {
	if profileID == "" {
		return
	}
	if transport != model.TransportXray {
		v.add(object, id, "XrayProfileID", "is only valid for xray transport")
		return
	}
	ref(v, object, id, "XrayProfileID", profileID, v.xray)
}

func (v *validator) rawUDP(object, id string, transport model.Transport, settings model.RawUDPSettings) {
	if transport != model.TransportUDP {
		return
	}
	v.positive("RawUDP.KeepAliveSecond", object, id, settings.KeepAliveSecond)
	v.positive("RawUDP.Workers", object, id, settings.Workers)
	if settings.Workers > 1 {
		v.add(object, id, "RawUDP.Workers", "must be 0 (automatic) or 1 because one UDP endpoint owns one device queue and socket")
	}
	v.positive("RawUDP.QueueSize", object, id, settings.QueueSize)
	v.positive("RawUDP.ConnectTimeout", object, id, settings.ConnectTimeout)
	v.positive("RawUDP.IdleTimeout", object, id, settings.IdleTimeout)
	v.rawDTLS(object, id, settings.DTLS)
}

func (v *validator) rawTCP(object, id string, transport model.Transport, settings model.RawTCPSettings) {
	if transport != model.TransportTCP && transport != model.TransportXray {
		return
	}
	switch settings.LengthMode {
	case "", model.TCPLength16, model.TCPLength32:
	default:
		v.add(object, id, "RawTCP.LengthMode", "must be empty, uint16, or uint32")
	}
	if transport != model.TransportTCP {
		return
	}
	v.positive("RawTCP.KeepAliveSecond", object, id, settings.KeepAliveSecond)
	v.positive("RawTCP.ConnectTimeout", object, id, settings.ConnectTimeout)
	v.positive("RawTCP.ReconnectSecond", object, id, settings.ReconnectSecond)
	v.positive("RawTCP.Workers", object, id, settings.Workers)
	if settings.Workers > 1 {
		v.add(object, id, "RawTCP.Workers", "must be 0 (automatic) or 1 because a framed TCP stream has one ordered reader")
	}
	v.positive("RawTCP.QueueSize", object, id, settings.QueueSize)
	v.positive("RawTCP.IdleTimeout", object, id, settings.IdleTimeout)
	v.rawTLS(object, id, settings.TLS)
}

func (v *validator) rawTLS(object, id string, settings model.RawTLSSettings) {
	v.validateRawSecurity("RawTCP.TLS", object, id, settings.Enabled, settings.CertFile, settings.KeyFile, settings.ServerName, settings.MinVersion, settings.MaxVersion)
	if object == "Connector" && (strings.TrimSpace(settings.CertFile) != "" || strings.TrimSpace(settings.KeyFile) != "") {
		v.add(object, id, "RawTCP.TLS.CertFile", "client certificates are not part of the connector TLS contract")
	}
	if object == "Listener" && settings.AllowInsecure {
		v.add(object, id, "RawTCP.TLS.AllowInsecure", "is only valid on a connector")
	}
	if !settings.Enabled {
		return
	}
	if object == "Listener" {
		if strings.TrimSpace(settings.CertFile) == "" {
			v.add(object, id, "RawTCP.TLS.CertFile", "is required when TLS is enabled on a listener")
		}
		if strings.TrimSpace(settings.KeyFile) == "" {
			v.add(object, id, "RawTCP.TLS.KeyFile", "is required when TLS is enabled on a listener")
		}
	}
}

func (v *validator) rawDTLS(object, id string, settings model.RawDTLSSettings) {
	v.validateRawSecurity("RawUDP.DTLS", object, id, settings.Enabled, settings.CertFile, settings.KeyFile, settings.ServerName, settings.MinVersion, settings.MaxVersion)
	minRank := v.rawTLSVersionRank(object, id, "RawUDP.DTLS.MinVersion", settings.MinVersion)
	maxRank := v.rawTLSVersionRank(object, id, "RawUDP.DTLS.MaxVersion", settings.MaxVersion)
	if minRank > 12 || (maxRank > 0 && maxRank < 12) {
		v.add(object, id, "RawUDP.DTLS.MinVersion", "the current DTLS transport uses DTLS 1.2, which must be inside the selected range")
	}
	if object == "Connector" && (strings.TrimSpace(settings.CertFile) != "" || strings.TrimSpace(settings.KeyFile) != "") {
		v.add(object, id, "RawUDP.DTLS.CertFile", "client certificates are not part of the connector DTLS contract")
	}
	if object == "Listener" && settings.AllowInsecure {
		v.add(object, id, "RawUDP.DTLS.AllowInsecure", "is only valid on a connector")
	}
	v.positive("RawUDP.DTLS.MTU", object, id, settings.MTU)
	v.positive("RawUDP.DTLS.ReplayWindow", object, id, settings.ReplayWindow)
	if !settings.Enabled {
		return
	}
	if object == "Listener" {
		if strings.TrimSpace(settings.CertFile) == "" {
			v.add(object, id, "RawUDP.DTLS.CertFile", "is required when DTLS is enabled on a listener")
		}
		if strings.TrimSpace(settings.KeyFile) == "" {
			v.add(object, id, "RawUDP.DTLS.KeyFile", "is required when DTLS is enabled on a listener")
		}
	}
}

func (v *validator) validateRawSecurity(prefix, object, id string, enabled bool, certFile, keyFile, serverName, minVersion, maxVersion string) {
	for _, item := range []struct {
		field string
		value string
	}{
		{prefix + ".CertFile", certFile},
		{prefix + ".KeyFile", keyFile},
		{prefix + ".ServerName", serverName},
	} {
		if strings.ContainsRune(item.value, 0) {
			v.add(object, id, item.field, "must not contain NUL bytes")
		}
	}
	if strings.TrimSpace(certFile) == "" && strings.TrimSpace(keyFile) != "" {
		v.add(object, id, prefix+".CertFile", "is required when key file is set")
	}
	if strings.TrimSpace(keyFile) == "" && strings.TrimSpace(certFile) != "" && enabled {
		v.add(object, id, prefix+".KeyFile", "is required when cert file is set")
	}
	minRank := v.rawTLSVersionRank(object, id, prefix+".MinVersion", minVersion)
	maxRank := v.rawTLSVersionRank(object, id, prefix+".MaxVersion", maxVersion)
	if minRank > 0 && maxRank > 0 && minRank > maxRank {
		v.add(object, id, prefix+".MaxVersion", "must be greater than or equal to MinVersion")
	}
}

func (v *validator) rawTLSVersionRank(object, id, field, value string) int {
	switch strings.TrimSpace(value) {
	case "":
		return 0
	case "1.0", "tls1.0", "TLS1.0":
		return 10
	case "1.1", "tls1.1", "TLS1.1":
		return 11
	case "1.2", "tls1.2", "TLS1.2":
		return 12
	case "1.3", "tls1.3", "TLS1.3":
		return 13
	default:
		v.add(object, id, field, "must be empty, 1.0, 1.1, 1.2, or 1.3")
		return -1
	}
}

func (v *validator) positive(field, object, id string, value int) {
	if value < 0 {
		v.add(object, id, field, "must be zero or positive")
	}
}

func (v *validator) deviceRoute(deviceID string, index int, route model.DeviceRoute) {
	field := fmt.Sprintf("Routes[%d]", index)
	if !route.Enabled {
		return
	}
	dstFamily := 0
	destination := strings.TrimSpace(route.Destination)
	if destination == "" {
		v.add("Device", deviceID, field+".Destination", "is required when route is enabled")
	} else if destination != "default" {
		prefix, err := netip.ParsePrefix(destination)
		if err != nil {
			v.add("Device", deviceID, field+".Destination", fmt.Sprintf("%q is invalid", route.Destination))
		} else if prefix.Addr().Is4() {
			dstFamily = 4
		} else {
			dstFamily = 6
		}
	}
	gatewayFamily := v.optionalAddrFamily("Device", deviceID, field+".Gateway", route.Gateway)
	sourceFamily := v.optionalAddrFamily("Device", deviceID, field+".Source", route.Source)
	if route.Metric < 0 {
		v.add("Device", deviceID, field+".Metric", "must be zero or positive")
	}
	if strings.ContainsAny(route.IfName, " \t\r\n") {
		v.add("Device", deviceID, field+".IfName", "must not contain whitespace")
	}
	if strings.ContainsAny(route.Table, " \t\r\n") {
		v.add("Device", deviceID, field+".Table", "must not contain whitespace")
	}
	if dstFamily != 0 && gatewayFamily != 0 && dstFamily != gatewayFamily {
		v.add("Device", deviceID, field+".Gateway", "must match destination IP family")
	}
	if dstFamily != 0 && sourceFamily != 0 && dstFamily != sourceFamily {
		v.add("Device", deviceID, field+".Source", "must match destination IP family")
	}
	if gatewayFamily != 0 && sourceFamily != 0 && gatewayFamily != sourceFamily {
		v.add("Device", deviceID, field+".Source", "must match gateway IP family")
	}
}

func (v *validator) optionalAddrFamily(object, id, field, value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		v.add(object, id, field, fmt.Sprintf("%q is invalid", value))
		return 0
	}
	if addr.Is4() {
		return 4
	}
	return 6
}

func ref[T any](v *validator, object, id, field, target string, index map[string]T) {
	if target == "" {
		return
	}
	if _, ok := index[target]; !ok {
		v.add(object, id, field, fmt.Sprintf("references missing %q", target))
	}
}

func enabledRef[T interface{ IsEnabled() bool }](v *validator, object, id, field, target string, index map[string]T) {
	if target == "" {
		return
	}
	item, ok := index[target]
	if !ok {
		return
	}
	if !item.IsEnabled() {
		v.add(object, id, field, fmt.Sprintf("references disabled %q", target))
	}
}

func (v *validator) ipPrefix(object, id, field, value string, ipv4 bool) {
	if value == "" {
		return
	}
	v.addressPrefix(object, id, field, value, ipv4)
}

func (v *validator) ipAddress(object, id, field, value string, ipv4 bool) {
	if value == "" {
		return
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		v.add(object, id, field, fmt.Sprintf("%q is invalid", value))
		return
	}
	if ipv4 && !addr.Is4() {
		v.add(object, id, field, fmt.Sprintf("%q is not IPv4", value))
	}
	if !ipv4 && !addr.Is6() {
		v.add(object, id, field, fmt.Sprintf("%q is not IPv6", value))
	}
}

func (v *validator) addressPrefix(object, id, field, value string, ipv4 bool) {
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		v.add(object, id, field, fmt.Sprintf("%q is invalid", value))
		return
	}
	if ipv4 && !prefix.Addr().Is4() {
		v.add(object, id, field, fmt.Sprintf("%q is not IPv4", value))
	}
	if !ipv4 && !prefix.Addr().Is6() {
		v.add(object, id, field, fmt.Sprintf("%q is not IPv6", value))
	}
}

func (v *validator) jsonObject(object, id, field, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if !json.Valid([]byte(value)) {
		v.add(object, id, field, "must be valid JSON")
		return
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		v.add(object, id, field, "must be valid JSON")
		return
	}
	switch decoded.(type) {
	case map[string]any, []any:
	default:
		v.add(object, id, field, "must be a JSON object or array")
	}
}

func (v *validator) xrayProtocol(object, id, field, value string) {
	if value == "" {
		return
	}
	if strings.ContainsAny(value, " \t\r\n") {
		v.add(object, id, field, "must not contain whitespace")
	}
}

func (v *validator) panelPasswordHash(object, id, field, value string) {
	parts := strings.Split(value, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		v.add(object, id, field, "must be pbkdf2-sha256$iterations$salt$hash")
		return
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 {
		v.add(object, id, field, "iterations must be at least 10000")
		return
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 8 {
		v.add(object, id, field, "salt must be unpadded base64 with at least 8 bytes")
		return
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(hash) < 16 {
		v.add(object, id, field, "hash must be unpadded base64 with at least 16 bytes")
	}
}

func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}
