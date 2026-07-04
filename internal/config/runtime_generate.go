package config

import "tapx/internal/model"

type GeneratedRuntime struct {
	Devices      []RuntimeDevice
	Listeners    []RuntimeEndpoint
	Connectors   []RuntimeEndpoint
	Routes       []RuntimeRoute
	XrayProfiles []RuntimeXrayProfile
	Settings     []RuntimeSettings
	UDPPipes     []RuntimeUDPPipe
	TCPPipes     []RuntimeTCPPipe
	XrayPipes    []RuntimeXrayPipe
}

type RuntimeDevice struct {
	ID       string
	Type     model.DeviceType
	IfName   string
	MTU      int
	MSSClamp int
	IPv4CIDR string
	IPv6CIDR string
	Bridge   RuntimeBridge
	Routes   []RuntimeDeviceRoute
	DNS      RuntimeDNS
}

type RuntimeBridge struct {
	Enabled bool
	Name    string
	IfName  string
	MTU     int
}

type RuntimeDeviceRoute struct {
	Enabled     bool
	Destination string
	Gateway     string
	Source      string
	IfName      string
	Metric      int
	Table       string
}

type RuntimeDNS struct {
	Enabled       bool
	Nameservers   []string
	SearchDomains []string
	Options       []string
	OutputPath    string
}

type RuntimeEndpoint struct {
	ID            string
	Transport     model.Transport
	BindHost      string
	BindPort      uint16
	Remote        string
	Port          uint16
	XrayProfileID string
	RawUDP        model.RawUDPSettings
	RawTCP        model.RawTCPSettings
	Binding       RuntimeBinding
}

type RuntimeRoute struct {
	ID          string
	ListenerID  string
	DeviceID    string
	ConnectorID string
	ClientID    string
	Binding     RuntimeBinding
}

type RuntimeUDPPipe struct {
	EndpointID    string
	EndpointKind  string
	RouteID       string
	DeviceID      string
	FrameKind     model.DeviceType
	BindHost      string
	BindPort      uint16
	Remote        string
	Port          uint16
	PeerMode      model.UDPPeerMode
	FixedPeer     string
	BindInterface string
	BindAddress   string
	ReceiveBuffer int
	SendBuffer    int
	ReuseAddr     bool
	ReusePort     bool
	MaxFrameSize  int
	DTLS          model.RawDTLSSettings
	AddressGuard  RuntimeAddressGuard
	Binding       RuntimeBinding
}

type RuntimeTCPPipe struct {
	EndpointID      string
	EndpointKind    string
	RouteID         string
	DeviceID        string
	FrameKind       model.DeviceType
	BindHost        string
	BindPort        uint16
	Remote          string
	Port            uint16
	LengthMode      model.TCPLengthMode
	MaxFrameSize    int
	BindInterface   string
	BindAddress     string
	ReceiveBuffer   int
	SendBuffer      int
	NoDelay         bool
	KeepAliveSecond int
	FastOpen        bool
	ConnectTimeout  int
	AddressGuard    RuntimeAddressGuard
	TLS             model.RawTLSSettings
	Binding         RuntimeBinding
}

type RuntimeXrayPipe struct {
	EndpointID    string
	EndpointKind  string
	RouteID       string
	DeviceID      string
	FrameKind     model.DeviceType
	XrayProfileID string
	Remote        string
	Port          uint16
	LengthMode    model.TCPLengthMode
	MaxFrameSize  int
	AddressGuard  RuntimeAddressGuard
	Binding       RuntimeBinding
}

type RuntimeAddressGuard struct {
	IPv4CIDRs []string
	IPv6CIDRs []string
	MACs      []string
}

type RuntimeBinding struct {
	VKeyValue   string
	ClientID    string
	RouteID     string
	DeviceID    string
	ConnectorID string
	AddressID   string
}

type RuntimeXrayProfile struct {
	ID                   string
	Runtime              model.XrayRuntime
	InboundProtocol      string
	InboundSettingsJSON  string
	OutboundProtocol     string
	OutboundSettingsJSON string
	Network              string
	Security             string
	StreamSettingsJSON   string
	SniffingJSON         string
	MuxJSON              string
	SockoptJSON          string
	FallbacksJSON        string
	RoutingJSON          string
	DNSJSON              string
	PolicyJSON           string
	AdvancedJSON         string
}

type RuntimeSettings struct {
	ID                  string
	PanelListen         string
	PanelHTTPS          bool
	PanelCertFile       string
	PanelKeyFile        string
	PanelAuthEnabled    bool
	AdminUsername       string
	SessionTTLSecond    int
	ExternalXrayPath    string
	LogLevel            string
	StatsIntervalSecond int
	BackupDir           string
	DataDir             string
	OpenWrtBuildTarget  string
	AdvancedJSON        string
}

func GenerateRuntime(cfg RuntimeConfig) (*GeneratedRuntime, error) {
	if err := ValidateForApply(cfg); err != nil {
		return nil, err
	}

	index := runtimeIndex(cfg)
	out := &GeneratedRuntime{}
	for _, item := range cfg.Devices {
		if !item.Enabled {
			continue
		}
		out.Devices = append(out.Devices, RuntimeDevice{
			ID:       item.ID,
			Type:     item.Type,
			IfName:   item.IfName,
			MTU:      item.MTU,
			MSSClamp: item.MSSClamp,
			IPv4CIDR: item.IPv4CIDR,
			IPv6CIDR: item.IPv6CIDR,
			Bridge:   runtimeBridge(item.Bridge),
			Routes:   runtimeDeviceRoutes(item.Routes),
			DNS:      runtimeDNS(item.DNS),
		})
	}
	for _, item := range cfg.Routes {
		if !item.Enabled {
			continue
		}
		out.Routes = append(out.Routes, RuntimeRoute{
			ID:          item.ID,
			ListenerID:  item.ListenerID,
			DeviceID:    item.DeviceID,
			ConnectorID: item.ConnectorID,
			ClientID:    item.ClientID,
			Binding: RuntimeBinding{
				VKeyValue:   index.vkeyValue(item.VKeyID),
				ClientID:    item.ClientID,
				DeviceID:    item.DeviceID,
				ConnectorID: item.ConnectorID,
				AddressID:   item.AddressID,
			},
		})
	}
	for _, item := range cfg.XrayProfiles {
		if !item.Enabled {
			continue
		}
		out.XrayProfiles = append(out.XrayProfiles, RuntimeXrayProfile{
			ID:                   item.ID,
			Runtime:              normalizeXrayRuntime(item.Runtime),
			InboundProtocol:      item.InboundProtocol,
			InboundSettingsJSON:  item.InboundSettingsJSON,
			OutboundProtocol:     item.OutboundProtocol,
			OutboundSettingsJSON: item.OutboundSettingsJSON,
			Network:              item.Network,
			Security:             item.Security,
			StreamSettingsJSON:   item.StreamSettingsJSON,
			SniffingJSON:         item.SniffingJSON,
			MuxJSON:              item.MuxJSON,
			SockoptJSON:          item.SockoptJSON,
			FallbacksJSON:        item.FallbacksJSON,
			RoutingJSON:          item.RoutingJSON,
			DNSJSON:              item.DNSJSON,
			PolicyJSON:           item.PolicyJSON,
			AdvancedJSON:         item.AdvancedJSON,
		})
	}
	for _, item := range cfg.Settings {
		if !item.Enabled {
			continue
		}
		out.Settings = append(out.Settings, RuntimeSettings{
			ID:                  item.ID,
			PanelListen:         item.PanelListen,
			PanelHTTPS:          item.PanelHTTPS,
			PanelCertFile:       item.PanelCertFile,
			PanelKeyFile:        item.PanelKeyFile,
			PanelAuthEnabled:    item.PanelAuthEnabled,
			AdminUsername:       item.AdminUsername,
			SessionTTLSecond:    item.SessionTTLSecond,
			ExternalXrayPath:    item.ExternalXrayPath,
			LogLevel:            item.LogLevel,
			StatsIntervalSecond: item.StatsIntervalSecond,
			BackupDir:           item.BackupDir,
			DataDir:             item.DataDir,
			OpenWrtBuildTarget:  item.OpenWrtBuildTarget,
			AdvancedJSON:        item.AdvancedJSON,
		})
	}
	for _, item := range cfg.Listeners {
		if !item.Enabled {
			continue
		}
		out.Listeners = append(out.Listeners, RuntimeEndpoint{
			ID:            item.ID,
			Transport:     item.Transport,
			BindHost:      item.BindHost,
			BindPort:      item.BindPort,
			XrayProfileID: item.XrayProfileID,
			RawUDP:        item.RawUDP,
			RawTCP:        item.RawTCP,
			Binding:       index.binding(item.Binding),
		})
		if item.Transport == model.TransportUDP {
			if pipe, ok := index.udpPipeFromListener(item); ok {
				out.UDPPipes = append(out.UDPPipes, pipe)
			}
		} else if item.Transport == model.TransportTCP {
			if pipe, ok := index.tcpPipeFromListener(item); ok {
				out.TCPPipes = append(out.TCPPipes, pipe)
			}
		} else if item.Transport == model.TransportXray {
			if pipe, ok := index.xrayPipeFromListener(item); ok {
				out.XrayPipes = append(out.XrayPipes, pipe)
			}
		}
	}
	for _, item := range cfg.Connectors {
		if !item.Enabled {
			continue
		}
		out.Connectors = append(out.Connectors, RuntimeEndpoint{
			ID:            item.ID,
			Transport:     item.Transport,
			Remote:        item.Remote,
			Port:          item.Port,
			XrayProfileID: item.XrayProfileID,
			RawUDP:        item.RawUDP,
			RawTCP:        item.RawTCP,
			Binding:       index.binding(item.Binding),
		})
		if item.Transport == model.TransportUDP {
			if pipe, ok := index.udpPipeFromConnector(item); ok {
				out.UDPPipes = append(out.UDPPipes, pipe)
			}
		} else if item.Transport == model.TransportTCP {
			if pipe, ok := index.tcpPipeFromConnector(item); ok {
				out.TCPPipes = append(out.TCPPipes, pipe)
			}
		} else if item.Transport == model.TransportXray {
			if pipe, ok := index.xrayPipeFromConnector(item); ok {
				out.XrayPipes = append(out.XrayPipes, pipe)
			}
		}
	}
	return out, nil
}

type generatorIndex struct {
	devices      map[string]model.Device
	routes       map[string]model.Route
	vkeys        map[string]model.VKey
	addresses    map[string]model.AddressLimit
	xrayProfiles map[string]model.XrayProfile
}

func runtimeIndex(cfg RuntimeConfig) generatorIndex {
	idx := generatorIndex{
		devices:      make(map[string]model.Device, len(cfg.Devices)),
		routes:       make(map[string]model.Route, len(cfg.Routes)),
		vkeys:        make(map[string]model.VKey, len(cfg.VKeys)),
		addresses:    make(map[string]model.AddressLimit, len(cfg.Addresses)),
		xrayProfiles: make(map[string]model.XrayProfile, len(cfg.XrayProfiles)),
	}
	for _, item := range cfg.Devices {
		idx.devices[item.ID] = item
	}
	for _, item := range cfg.Routes {
		idx.routes[item.ID] = item
	}
	for _, item := range cfg.VKeys {
		idx.vkeys[item.ID] = item
	}
	for _, item := range cfg.Addresses {
		idx.addresses[item.ID] = item
	}
	for _, item := range cfg.XrayProfiles {
		idx.xrayProfiles[item.ID] = item
	}
	return idx
}

func (idx generatorIndex) udpPipeFromListener(input model.Listener) (RuntimeUDPPipe, bool) {
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeUDPPipe{}, false
	}
	return RuntimeUDPPipe{
		EndpointID:    input.ID,
		EndpointKind:  "listener",
		RouteID:       binding.RouteID,
		DeviceID:      device.ID,
		FrameKind:     device.Type,
		BindHost:      input.BindHost,
		BindPort:      input.BindPort,
		PeerMode:      normalizeUDPPeerMode(input.RawUDP.PeerMode),
		FixedPeer:     input.RawUDP.FixedPeer,
		BindInterface: input.RawUDP.BindInterface,
		BindAddress:   input.RawUDP.BindAddress,
		ReceiveBuffer: input.RawUDP.ReceiveBuffer,
		SendBuffer:    input.RawUDP.SendBuffer,
		ReuseAddr:     input.RawUDP.ReuseAddr,
		ReusePort:     input.RawUDP.ReusePort,
		MaxFrameSize:  maxFrameSize(device),
		DTLS:          input.RawUDP.DTLS,
		AddressGuard:  idx.addressGuard(binding),
		Binding:       binding,
	}, true
}

func (idx generatorIndex) udpPipeFromConnector(input model.Connector) (RuntimeUDPPipe, bool) {
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeUDPPipe{}, false
	}
	return RuntimeUDPPipe{
		EndpointID:    input.ID,
		EndpointKind:  "connector",
		RouteID:       binding.RouteID,
		DeviceID:      device.ID,
		FrameKind:     device.Type,
		Remote:        input.Remote,
		Port:          input.Port,
		PeerMode:      normalizeUDPPeerMode(input.RawUDP.PeerMode),
		FixedPeer:     input.RawUDP.FixedPeer,
		BindInterface: input.RawUDP.BindInterface,
		BindAddress:   input.RawUDP.BindAddress,
		ReceiveBuffer: input.RawUDP.ReceiveBuffer,
		SendBuffer:    input.RawUDP.SendBuffer,
		ReuseAddr:     input.RawUDP.ReuseAddr,
		ReusePort:     input.RawUDP.ReusePort,
		MaxFrameSize:  maxFrameSize(device),
		DTLS:          input.RawUDP.DTLS,
		AddressGuard:  idx.addressGuard(binding),
		Binding:       binding,
	}, true
}

func (idx generatorIndex) tcpPipeFromListener(input model.Listener) (RuntimeTCPPipe, bool) {
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeTCPPipe{}, false
	}
	return RuntimeTCPPipe{
		EndpointID:      input.ID,
		EndpointKind:    "listener",
		RouteID:         binding.RouteID,
		DeviceID:        device.ID,
		FrameKind:       device.Type,
		BindHost:        input.BindHost,
		BindPort:        input.BindPort,
		LengthMode:      normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:    maxFrameSize(device),
		BindInterface:   input.RawTCP.BindInterface,
		BindAddress:     input.RawTCP.BindAddress,
		ReceiveBuffer:   input.RawTCP.ReceiveBuffer,
		SendBuffer:      input.RawTCP.SendBuffer,
		NoDelay:         input.RawTCP.NoDelay,
		KeepAliveSecond: input.RawTCP.KeepAliveSecond,
		FastOpen:        input.RawTCP.FastOpen,
		ConnectTimeout:  input.RawTCP.ConnectTimeout,
		AddressGuard:    idx.addressGuard(binding),
		TLS:             input.RawTCP.TLS,
		Binding:         binding,
	}, true
}

func (idx generatorIndex) tcpPipeFromConnector(input model.Connector) (RuntimeTCPPipe, bool) {
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeTCPPipe{}, false
	}
	return RuntimeTCPPipe{
		EndpointID:      input.ID,
		EndpointKind:    "connector",
		RouteID:         binding.RouteID,
		DeviceID:        device.ID,
		FrameKind:       device.Type,
		Remote:          input.Remote,
		Port:            input.Port,
		LengthMode:      normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:    maxFrameSize(device),
		BindInterface:   input.RawTCP.BindInterface,
		BindAddress:     input.RawTCP.BindAddress,
		ReceiveBuffer:   input.RawTCP.ReceiveBuffer,
		SendBuffer:      input.RawTCP.SendBuffer,
		NoDelay:         input.RawTCP.NoDelay,
		KeepAliveSecond: input.RawTCP.KeepAliveSecond,
		FastOpen:        input.RawTCP.FastOpen,
		ConnectTimeout:  input.RawTCP.ConnectTimeout,
		AddressGuard:    idx.addressGuard(binding),
		TLS:             input.RawTCP.TLS,
		Binding:         binding,
	}, true
}

func (idx generatorIndex) xrayPipeFromListener(input model.Listener) (RuntimeXrayPipe, bool) {
	if !idx.hasEmbeddedXrayProfile(input.XrayProfileID) {
		return RuntimeXrayPipe{}, false
	}
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeXrayPipe{}, false
	}
	return RuntimeXrayPipe{
		EndpointID:    input.ID,
		EndpointKind:  "listener",
		RouteID:       binding.RouteID,
		DeviceID:      device.ID,
		FrameKind:     device.Type,
		XrayProfileID: input.XrayProfileID,
		LengthMode:    normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:  maxFrameSize(device),
		AddressGuard:  idx.addressGuard(binding),
		Binding:       binding,
	}, true
}

func (idx generatorIndex) xrayPipeFromConnector(input model.Connector) (RuntimeXrayPipe, bool) {
	if !idx.hasEmbeddedXrayProfile(input.XrayProfileID) {
		return RuntimeXrayPipe{}, false
	}
	binding := idx.binding(input.Binding)
	device, ok := idx.deviceForBinding(binding)
	if !ok {
		return RuntimeXrayPipe{}, false
	}
	return RuntimeXrayPipe{
		EndpointID:    input.ID,
		EndpointKind:  "connector",
		RouteID:       binding.RouteID,
		DeviceID:      device.ID,
		FrameKind:     device.Type,
		XrayProfileID: input.XrayProfileID,
		Remote:        input.Remote,
		Port:          input.Port,
		LengthMode:    normalizeTCPLengthMode(input.RawTCP.LengthMode),
		MaxFrameSize:  maxFrameSize(device),
		AddressGuard:  idx.addressGuard(binding),
		Binding:       binding,
	}, true
}

func (idx generatorIndex) hasEmbeddedXrayProfile(id string) bool {
	profile, ok := idx.xrayProfiles[id]
	return ok && profile.Enabled && normalizeXrayRuntime(profile.Runtime) == model.XrayEmbedded
}

func (idx generatorIndex) deviceForBinding(binding RuntimeBinding) (model.Device, bool) {
	if binding.DeviceID == "" {
		return model.Device{}, false
	}
	device, ok := idx.devices[binding.DeviceID]
	if !ok || !device.Enabled {
		return model.Device{}, false
	}
	return device, true
}

func (idx generatorIndex) binding(input model.Binding) RuntimeBinding {
	out := RuntimeBinding{
		VKeyValue:   idx.vkeyValue(input.VKeyID),
		ClientID:    input.ClientID,
		RouteID:     input.RouteID,
		DeviceID:    input.DeviceID,
		ConnectorID: input.ConnectorID,
		AddressID:   input.AddressID,
	}
	if input.RouteID == "" {
		return out
	}
	route, ok := idx.routes[input.RouteID]
	if !ok {
		return out
	}
	out.VKeyValue = first(out.VKeyValue, idx.vkeyValue(route.VKeyID))
	out.ClientID = first(out.ClientID, route.ClientID)
	out.DeviceID = first(out.DeviceID, route.DeviceID)
	out.ConnectorID = first(out.ConnectorID, route.ConnectorID)
	out.AddressID = first(out.AddressID, route.AddressID)
	return out
}

func (idx generatorIndex) addressGuard(binding RuntimeBinding) RuntimeAddressGuard {
	if binding.AddressID == "" {
		return RuntimeAddressGuard{}
	}
	address, ok := idx.addresses[binding.AddressID]
	if !ok || !address.Enabled {
		return RuntimeAddressGuard{}
	}
	return RuntimeAddressGuard{
		IPv4CIDRs: append([]string(nil), address.IPv4CIDRs...),
		IPv6CIDRs: append([]string(nil), address.IPv6CIDRs...),
		MACs:      append([]string(nil), address.MACs...),
	}
}

func normalizeUDPPeerMode(mode model.UDPPeerMode) model.UDPPeerMode {
	if mode == "" {
		return model.UDPPeerAny
	}
	return mode
}

func normalizeTCPLengthMode(mode model.TCPLengthMode) model.TCPLengthMode {
	if mode == "" {
		return model.TCPLength16
	}
	return mode
}

func normalizeXrayRuntime(runtime model.XrayRuntime) model.XrayRuntime {
	if runtime == "" {
		return model.XrayEmbedded
	}
	return runtime
}

func runtimeBridge(input *model.BridgeConfig) RuntimeBridge {
	if input == nil {
		return RuntimeBridge{}
	}
	return RuntimeBridge{
		Enabled: input.Enabled,
		Name:    input.Name,
		IfName:  input.IfName,
		MTU:     input.MTU,
	}
}

func runtimeDeviceRoutes(input []model.DeviceRoute) []RuntimeDeviceRoute {
	if len(input) == 0 {
		return nil
	}
	out := make([]RuntimeDeviceRoute, 0, len(input))
	for _, item := range input {
		if !item.Enabled {
			continue
		}
		out = append(out, RuntimeDeviceRoute{
			Enabled:     item.Enabled,
			Destination: item.Destination,
			Gateway:     item.Gateway,
			Source:      item.Source,
			IfName:      item.IfName,
			Metric:      item.Metric,
			Table:       item.Table,
		})
	}
	return out
}

func runtimeDNS(input *model.DNSConfig) RuntimeDNS {
	if input == nil {
		return RuntimeDNS{}
	}
	return RuntimeDNS{
		Enabled:       input.Enabled,
		Nameservers:   append([]string(nil), input.Nameservers...),
		SearchDomains: append([]string(nil), input.SearchDomains...),
		Options:       append([]string(nil), input.Options...),
		OutputPath:    input.OutputPath,
	}
}

func maxFrameSize(device model.Device) int {
	if device.MTU > 0 {
		return device.MTU
	}
	return 65535
}

func (idx generatorIndex) vkeyValue(id string) string {
	if id == "" {
		return ""
	}
	item, ok := idx.vkeys[id]
	if !ok || !item.Enabled {
		return ""
	}
	return item.Value
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
