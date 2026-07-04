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
	v.enabledReferences()
	return v.err()
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
		v.ipPrefix("Device", item.ID, "IPv4CIDR", item.IPv4CIDR, true)
		v.ipPrefix("Device", item.ID, "IPv6CIDR", item.IPv6CIDR, false)
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
		v.deviceDNS(item.ID, item.DNS)
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
	}
}

func (v *validator) validateSettings() {
	for _, item := range v.cfg.Settings {
		switch item.LogLevel {
		case "", "debug", "info", "warn", "error":
		default:
			v.add("Settings", item.ID, "LogLevel", "must be empty, debug, info, warn, or error")
		}
		if item.OpenWrtBuildTarget != "" && item.OpenWrtBuildTarget != "x86-64" {
			v.add("Settings", item.ID, "OpenWrtBuildTarget", "currently must be x86-64")
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
		v.positive("StatsIntervalSecond", "Settings", item.ID, item.StatsIntervalSecond)
		v.positive("SessionTTLSecond", "Settings", item.ID, item.SessionTTLSecond)
		v.jsonObject("Settings", item.ID, "AdvancedJSON", item.AdvancedJSON)
	}
}

func (v *validator) validateRoutes() {
	for _, item := range v.cfg.Routes {
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

func (v *validator) validateListeners() {
	for _, item := range v.cfg.Listeners {
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
	}
}

func (v *validator) validateClients() {
	for _, item := range v.cfg.Clients {
		if item.ListenerID != "" {
			ref(v, "Client", item.ID, "ListenerID", item.ListenerID, v.listeners)
		}
		v.clientCredential(item)
		if item.AddressID != "" {
			ref(v, "Client", item.ID, "AddressID", item.AddressID, v.addresses)
		}
		v.bindingRefs("Client", item.ID, item.Binding, "")
	}
}

func (v *validator) clientCredential(item model.Client) {
	credentialType := strings.TrimSpace(item.CredentialType)
	credentialValue := strings.TrimSpace(item.CredentialValue)
	switch credentialType {
	case "", "uuid", "password", "vkey":
	default:
		v.add("Client", item.ID, "CredentialType", "must be uuid, password, vkey, or empty")
	}
	if credentialType == "" {
		return
	}
	if credentialValue == "" {
		v.add("Client", item.ID, "CredentialValue", "is required when CredentialType is set")
		return
	}
	if strings.ContainsAny(credentialValue, "\r\n\t ") {
		v.add("Client", item.ID, "CredentialValue", "must not contain whitespace")
	}
	if credentialType == "uuid" && !looksLikeUUID(credentialValue) {
		v.add("Client", item.ID, "CredentialValue", "must be a UUID for uuid credentials")
	}
}

type addressOwner struct {
	id    string
	field string
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
	switch settings.PeerMode {
	case "", model.UDPPeerAny, model.UDPPeerFixed, model.UDPPeerLearn:
	default:
		v.add(object, id, "RawUDP.PeerMode", "must be empty, any, fixed, or learn")
	}
	if settings.PeerMode == model.UDPPeerFixed && settings.FixedPeer == "" {
		v.add(object, id, "RawUDP.FixedPeer", "is required when peer mode is fixed")
	}
	if settings.FixedPeer != "" {
		if _, err := netip.ParseAddrPort(settings.FixedPeer); err != nil {
			v.add(object, id, "RawUDP.FixedPeer", "must be host:port with an IP address")
		}
	}
	if settings.BindAddress != "" {
		if _, err := netip.ParseAddr(settings.BindAddress); err != nil {
			v.add(object, id, "RawUDP.BindAddress", "must be an IP address")
		}
	}
	v.positive("RawUDP.ReceiveBuffer", object, id, settings.ReceiveBuffer)
	v.positive("RawUDP.SendBuffer", object, id, settings.SendBuffer)
	v.positive("RawUDP.KeepAliveSecond", object, id, settings.KeepAliveSecond)
	v.positive("RawUDP.Workers", object, id, settings.Workers)
	v.positive("RawUDP.QueueSize", object, id, settings.QueueSize)
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
	if settings.BindAddress != "" {
		if _, err := netip.ParseAddr(settings.BindAddress); err != nil {
			v.add(object, id, "RawTCP.BindAddress", "must be an IP address")
		}
	}
	v.positive("RawTCP.ReceiveBuffer", object, id, settings.ReceiveBuffer)
	v.positive("RawTCP.SendBuffer", object, id, settings.SendBuffer)
	v.positive("RawTCP.KeepAliveSecond", object, id, settings.KeepAliveSecond)
	v.positive("RawTCP.ConnectTimeout", object, id, settings.ConnectTimeout)
	v.positive("RawTCP.ReconnectSecond", object, id, settings.ReconnectSecond)
	v.positive("RawTCP.Workers", object, id, settings.Workers)
	v.positive("RawTCP.ReadBuffer", object, id, settings.ReadBuffer)
	v.positive("RawTCP.WriteBuffer", object, id, settings.WriteBuffer)
	v.rawTLS(object, id, settings.TLS)
}

func (v *validator) rawTLS(object, id string, settings model.RawTLSSettings) {
	v.validateRawSecurity("RawTCP.TLS", object, id, settings.Enabled, settings.CertFile, settings.KeyFile, settings.CAFile, settings.ServerName, settings.ALPN, settings.MinVersion, settings.MaxVersion)
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
	v.validateRawSecurity("RawUDP.DTLS", object, id, settings.Enabled, settings.CertFile, settings.KeyFile, settings.CAFile, settings.ServerName, settings.ALPN, settings.MinVersion, settings.MaxVersion)
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

func (v *validator) validateRawSecurity(prefix, object, id string, enabled bool, certFile, keyFile, caFile, serverName string, alpn []string, minVersion, maxVersion string) {
	for _, item := range []struct {
		field string
		value string
	}{
		{prefix + ".CertFile", certFile},
		{prefix + ".KeyFile", keyFile},
		{prefix + ".CAFile", caFile},
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
	for i, proto := range alpn {
		field := fmt.Sprintf("%s.ALPN[%d]", prefix, i)
		if proto == "" {
			v.add(object, id, field, "must not be empty")
			continue
		}
		if strings.ContainsAny(proto, "\x00 \t\r\n") {
			v.add(object, id, field, "must not contain whitespace or NUL bytes")
		}
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

func (v *validator) deviceDNS(deviceID string, dns *model.DNSConfig) {
	if dns == nil || !dns.Enabled {
		return
	}
	if len(dns.Nameservers) == 0 {
		v.add("Device", deviceID, "DNS.Nameservers", "is required when DNS is enabled")
	}
	for _, nameserver := range dns.Nameservers {
		if _, err := netip.ParseAddr(nameserver); err != nil {
			v.add("Device", deviceID, "DNS.Nameservers", fmt.Sprintf("%q is invalid", nameserver))
		}
	}
	for _, domain := range dns.SearchDomains {
		if strings.TrimSpace(domain) == "" || strings.ContainsAny(domain, " \t\r\n") {
			v.add("Device", deviceID, "DNS.SearchDomains", fmt.Sprintf("%q is invalid", domain))
		}
	}
	for _, option := range dns.Options {
		if strings.TrimSpace(option) == "" || strings.ContainsAny(option, " \t\r\n") {
			v.add("Device", deviceID, "DNS.Options", fmt.Sprintf("%q is invalid", option))
		}
	}
	if strings.ContainsRune(dns.OutputPath, 0) {
		v.add("Device", deviceID, "DNS.OutputPath", "must not contain NUL")
	}
	if dns.OutputPath != "" && !strings.HasPrefix(dns.OutputPath, "/") {
		v.add("Device", deviceID, "DNS.OutputPath", "must be an absolute Linux path")
	}
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
